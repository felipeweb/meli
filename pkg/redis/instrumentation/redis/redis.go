package redis

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/felipeweb/meli/pkg/redis/instrumentation/redis/internal"
	"github.com/felipeweb/meli/pkg/redis/instrumentation/redis/internal/observability"
	"github.com/felipeweb/meli/pkg/redis/instrumentation/redis/internal/pool"
	"github.com/felipeweb/meli/pkg/redis/instrumentation/redis/internal/proto"

	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
)

// Nil reply Redis returns when key does not exist.
const Nil = proto.Nil

func init() {
	SetLogger(log.New(os.Stderr, "redis: ", log.LstdFlags|log.Lshortfile))
}

func SetLogger(logger *log.Logger) {
	internal.Logger = logger
}

type baseClient struct {
	opt      *Options
	connPool pool.Pooler

	process           func(Cmder) error
	processPipeline   func([]Cmder) error
	processTxPipeline func([]Cmder) error

	onClose func() error // hook called when client is closed

	ctxFunc func() context.Context // Optional function invoked
}

func (c *baseClient) context() context.Context {
	var ctx context.Context
	if c.ctxFunc != nil {
		ctx = c.ctxFunc()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx
}

func (c *baseClient) init() {
	c.process = c.defaultProcess
	c.processPipeline = c.defaultProcessPipeline
	c.processTxPipeline = c.defaultProcessTxPipeline
}

func (c *baseClient) String() string {
	return fmt.Sprintf("Redis<%s db:%d>", c.getAddr(), c.opt.DB)
}

func (c *baseClient) newConn() (*pool.Conn, error) {
	cn, err := c.connPool.NewConn()
	if err != nil {
		return nil, err
	}

	if !cn.Inited {
		if err := c.initConn(cn); err != nil {
			_ = c.connPool.CloseConn(cn)
			return nil, err
		}
	}

	return cn, nil
}

func (c *baseClient) getConn(ctx context.Context) (*pool.Conn, bool, error) {
	_, span := trace.StartSpan(ctx, "redis.(*baseClient).getConn")
	defer span.End()

	cn, isNew, err := c.connPool.Get()
	if err != nil {
		span.SetStatus(trace.Status{Code: int32(trace.StatusCodeInternal), Message: err.Error()})
		return nil, false, err
	}

	if !cn.Inited {
		span.Annotatef(nil, "Initializing connection")
		if err := c.initConn(cn); err != nil {
			span.SetStatus(trace.Status{Code: int32(trace.StatusCodeInternal), Message: err.Error()})
			_ = c.connPool.Remove(cn)
			return nil, false, err
		}
	}

	if isNew {
		stats.Record(ctx, observability.MConnectionsNew.M(1), observability.MConnectionsTaken.M(1))
	} else {
		stats.Record(ctx, observability.MConnectionsReused.M(1), observability.MConnectionsTaken.M(1))
	}
	return cn, isNew, nil
}

func (c *baseClient) releaseConn(ctx context.Context, cn *pool.Conn, err error) bool {
	if internal.IsBadConn(err, false) {
		_ = c.connPool.Remove(cn)
		return false
	}

	_ = c.connPool.Put(cn)
	stats.Record(ctx, observability.MConnectionsReturned.M(1))
	return true
}

func (c *baseClient) initConn(cn *pool.Conn) error {
	cn.Inited = true

	if c.opt.Password == "" &&
		c.opt.DB == 0 &&
		!c.opt.readOnly &&
		c.opt.OnConnect == nil {
		return nil
	}

	conn := newConn(c.opt, cn)
	_, err := conn.Pipelined(func(pipe Pipeliner) error {
		if c.opt.Password != "" {
			pipe.Auth(c.opt.Password)
		}

		if c.opt.DB > 0 {
			pipe.Select(c.opt.DB)
		}

		if c.opt.readOnly {
			pipe.ReadOnly()
		}

		return nil
	})
	if err != nil {
		return err
	}

	if c.opt.OnConnect != nil {
		return c.opt.OnConnect(conn)
	}
	return nil
}

// WrapProcess wraps function that processes Redis commands.
func (c *baseClient) WrapProcess(fn func(oldProcess func(cmd Cmder) error) func(cmd Cmder) error) {
	c.process = fn(c.process)
}

func (c *baseClient) Process(cmd Cmder) error {
	return c.process(cmd)
}

func (c *baseClient) defaultProcess(cmd Cmder) error {
	ctx, span := trace.StartSpan(c.context(), "redis.(*baseClient)."+cmd.Name())
	ctx, _ = observability.TagKeyValuesIntoContext(ctx, observability.KeyCommandName, cmd.Name())

	startTime := time.Now()
	defer func() {
		span.End()
		// Finally record the roundtrip latency.
		stats.Record(ctx, observability.MRoundtripLatencyMilliseconds.M(time.Since(startTime).Seconds()*1000))
	}()

	for attempt := 0; attempt <= c.opt.MaxRetries; attempt++ {
		span.Annotatef([]trace.Attribute{
			trace.Int64Attribute("attempt", int64(attempt)),
			trace.Int64Attribute("write_timeout_ns", c.opt.WriteTimeout.Nanoseconds()),
		}, "Getting connection")

		if attempt > 0 {
			td := c.retryBackoff(attempt)
			span.Annotatef([]trace.Attribute{
				trace.StringAttribute("sleep_duration", td.String()),
				trace.Int64Attribute("attempt", int64(attempt)),
			}, "Sleeping for exponential backoff")
			time.Sleep(td)
		}

		cn, _, err := c.getConn(ctx)
		if err != nil {
			cmd.setErr(err)
			if internal.IsRetryableError(err, true) {
				span.Annotatef([]trace.Attribute{
					trace.StringAttribute("err", err.Error()),
					trace.Int64Attribute("attempt", int64(attempt)),
				}, "Retryable error so retrying")
				continue
			}
			span.SetStatus(trace.Status{Code: int32(trace.StatusCodeInternal), Message: err.Error()})
			return err
		}

		cn.SetWriteTimeout(c.opt.WriteTimeout)
		_, wSpan := trace.StartSpan(ctx, "redis.writeCmd")
		wErr := writeCmd(ctx, cn, cmd)
		wSpan.End()
		if wErr != nil {
			releaseStart := time.Now()
			span.Annotatef(nil, "Releasing connection")
			reusedConn := c.releaseConn(ctx, cn, wErr)
			status := "removed conn"
			if reusedConn {
				status = "reused conn"
			}
			span.Annotatef([]trace.Attribute{
				trace.StringAttribute("time_spent", time.Since(releaseStart).String()),
				trace.StringAttribute("status", status),
			}, "Released connection")
			cmd.setErr(wErr)
			if internal.IsRetryableError(wErr, true) {
				continue
			}
			span.SetStatus(trace.Status{Code: int32(trace.StatusCodeInternal), Message: err.Error()})
			return err
		}

		_, rSpan := trace.StartSpan(ctx, "redis.(Cmder).readReply")
		cn.SetReadTimeout(c.cmdTimeout(cmd))
		err = cmd.readReply(cn)
		rSpan.End()
		span.Annotatef(nil, "Releasing connection")
		reusedConn := c.releaseConn(ctx, cn, err)
		status := "removed conn"
		if reusedConn {
			status = "reused conn"
		}
		span.Annotatef([]trace.Attribute{
			trace.StringAttribute("status", status),
		}, "Released connection")
		// TODO: (@odeke-em) multiplex on the errors retrieved
		if err != nil {
			span.SetStatus(trace.Status{Code: int32(trace.StatusCodeInternal), Message: err.Error()})
			if internal.IsRetryableError(err, cmd.readTimeout() == nil) {
				continue
			}
		}

		return err
	}

	eErr := cmd.Err()
	if eErr != nil {
		// TODO: (@odeke-em) tally/categorize these errors and increment them
		span.SetStatus(trace.Status{Code: int32(trace.StatusCodeInternal), Message: eErr.Error()})
	}
	return eErr
}

func (c *baseClient) retryBackoff(attempt int) time.Duration {
	return internal.RetryBackoff(attempt, c.opt.MinRetryBackoff, c.opt.MaxRetryBackoff)
}

func (c *baseClient) cmdTimeout(cmd Cmder) time.Duration {
	if timeout := cmd.readTimeout(); timeout != nil {
		return *timeout
	}

	return c.opt.ReadTimeout
}

// Close closes the client, releasing any open resources.
//
// It is rare to Close a Client, as the Client is meant to be
// long-lived and shared between many goroutines.
func (c *baseClient) Close() error {
	var firstErr error
	if c.onClose != nil {
		if err := c.onClose(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := c.connPool.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (c *baseClient) getAddr() string {
	return c.opt.Addr
}

func (c *baseClient) WrapProcessPipeline(
	fn func(oldProcess func([]Cmder) error) func([]Cmder) error,
) {
	c.processPipeline = fn(c.processPipeline)
	c.processTxPipeline = fn(c.processTxPipeline)
}

func (c *baseClient) defaultProcessPipeline(cmds []Cmder) error {
	return c.generalProcessPipeline(cmds, c.pipelineProcessCmds)
}

func (c *baseClient) defaultProcessTxPipeline(cmds []Cmder) error {
	return c.generalProcessPipeline(cmds, c.txPipelineProcessCmds)
}

type pipelineProcessor func(*pool.Conn, []Cmder) (bool, error)

func (c *baseClient) generalProcessPipeline(cmds []Cmder, p pipelineProcessor) error {
	ctx, span := trace.StartSpan(c.context(), "redis.(*baseClient).generalProcessPipeline")
	defer span.End()

	for attempt := 0; attempt <= c.opt.MaxRetries; attempt++ {
		span.Annotatef([]trace.Attribute{
			trace.Int64Attribute("attempt", int64(attempt)),
		}, "Getting connection")

		if attempt > 0 {
			td := c.retryBackoff(attempt)
			span.Annotatef([]trace.Attribute{
				trace.StringAttribute("sleep_duration", td.String()),
				trace.Int64Attribute("attempt", int64(attempt)),
			}, "Sleeping for exponential backoff")
			time.Sleep(td)
		}

		cn, _, err := c.getConn(ctx)
		if err != nil {
			span.SetStatus(trace.Status{Code: int32(trace.StatusCodeInternal), Message: err.Error()})
			setCmdsErr(cmds, err)
			return err
		}

		canRetry, err := p(cn, cmds)

		if err == nil || internal.IsRedisError(err) {
			span.Annotatef(nil, "Putting connection back into the pool")
			_ = c.connPool.Put(cn)
			break
		}
		span.Annotatef(nil, "Removing connection from the pool")
		_ = c.connPool.Remove(cn)

		if !canRetry || !internal.IsRetryableError(err, true) {
			span.Annotatef([]trace.Attribute{
				trace.StringAttribute("err", err.Error()),
				trace.Int64Attribute("attempt", int64(attempt)),
			}, "Not retrying hence exiting")
			break
		}

		// Otherwise, retrying with the next attempt.
		span.Annotatef([]trace.Attribute{
			trace.StringAttribute("err", err.Error()),
			trace.Int64Attribute("attempt", int64(attempt)),
		}, "Retryable error so retrying")
	}
	eErr := firstCmdsErr(cmds)
	if eErr != nil {
		span.SetStatus(trace.Status{Code: int32(trace.StatusCodeInternal), Message: eErr.Error()})
	}
	return eErr
}

func attributeNamesFromCommands(cmds []Cmder) []trace.Attribute {
	if len(cmds) == 0 {
		return nil
	}
	names := namesFromCommands(cmds)
	attributes := make([]trace.Attribute, len(names))
	for i, name := range names {
		attributes[i] = trace.StringAttribute(fmt.Sprintf("%d", i), name)
	}
	return attributes
}

func namesFromCommands(cmds []Cmder) []string {
	names := make([]string, len(cmds))
	for i, cmd := range cmds {
		names[i] = cmd.Name()
	}
	return names
}

func (c *baseClient) pipelineProcessCmds(cn *pool.Conn, cmds []Cmder) (bool, error) {
	ctx, span := trace.StartSpan(c.context(), "redis.(*baseClient).pipelineProcessCmds")
	defer span.End()
	attributes := append(attributeNamesFromCommands(cmds),
		trace.StringAttribute("read_timeout", c.opt.ReadTimeout.String()),
		trace.StringAttribute("write_timeout", c.opt.WriteTimeout.String()),
	)
	span.Annotatef(attributes, "With commands")

	cn.SetWriteTimeout(c.opt.WriteTimeout)
	_, wSpan := trace.StartSpan(ctx, "redis.writeCmd")
	defer wSpan.End()
	if err := writeCmd(ctx, cn, cmds...); err != nil {
		wSpan.SetStatus(trace.Status{Code: int32(trace.StatusCodeInternal), Message: err.Error()})
		setCmdsErr(cmds, err)
		return true, err
	}

	// Set read timeout for all commands.
	cn.SetReadTimeout(c.opt.ReadTimeout)
	_, rSpan := trace.StartSpan(ctx, "redis.pipelineReadCmds")
	err := pipelineReadCmds(cn, cmds)
	if err != nil {
		rSpan.SetStatus(trace.Status{Code: int32(trace.StatusCodeInternal), Message: err.Error()})
	}
	rSpan.End()
	return true, err
}

func pipelineReadCmds(cn *pool.Conn, cmds []Cmder) error {
	for _, cmd := range cmds {
		err := cmd.readReply(cn)
		if err != nil && !internal.IsRedisError(err) {
			return err
		}
	}
	return nil
}

func (c *baseClient) txPipelineProcessCmds(cn *pool.Conn, cmds []Cmder) (bool, error) {
	cn.SetWriteTimeout(c.opt.WriteTimeout)
	if err := txPipelineWriteMulti(c.context(), cn, cmds); err != nil {
		setCmdsErr(cmds, err)
		return true, err
	}

	// Set read timeout for all commands.
	cn.SetReadTimeout(c.opt.ReadTimeout)

	if err := c.txPipelineReadQueued(cn, cmds); err != nil {
		setCmdsErr(cmds, err)
		return false, err
	}

	return false, pipelineReadCmds(cn, cmds)
}

func txPipelineWriteMulti(ctx context.Context, cn *pool.Conn, cmds []Cmder) error {
	multiExec := make([]Cmder, 0, len(cmds)+2)
	multiExec = append(multiExec, NewStatusCmd("MULTI"))
	multiExec = append(multiExec, cmds...)
	multiExec = append(multiExec, NewSliceCmd("EXEC"))
	return writeCmd(ctx, cn, multiExec...)
}

func (c *baseClient) txPipelineReadQueued(cn *pool.Conn, cmds []Cmder) error {
	// Parse queued replies.
	var statusCmd StatusCmd
	if err := statusCmd.readReply(cn); err != nil {
		return err
	}

	for _ = range cmds {
		err := statusCmd.readReply(cn)
		if err != nil && !internal.IsRedisError(err) {
			return err
		}
	}

	// Parse number of replies.
	line, err := cn.Rd.ReadLine()
	if err != nil {
		if err == Nil {
			err = TxFailedErr
		}
		return err
	}

	switch line[0] {
	case proto.ErrorReply:
		return proto.ParseErrorReply(line)
	case proto.ArrayReply:
		// ok
	default:
		err := fmt.Errorf("redis: expected '*', but got line %q", line)
		return err
	}

	return nil
}

//------------------------------------------------------------------------------

// Client is a Redis client representing a pool of zero or more
// underlying connections. It's safe for concurrent use by multiple
// goroutines.
type Client struct {
	baseClient
	cmdable

	ctx context.Context
}

// NewClient returns a client to the Redis Server specified by Options.
func NewClient(opt *Options) *Client {
	opt.init()

	c := Client{
		baseClient: baseClient{
			opt:      opt,
			connPool: newConnPool(opt),
		},
		ctx: opt.Context,
	}
	c.baseClient.ctxFunc = c.Context
	c.baseClient.init()
	c.init()

	return &c
}

func (c *Client) init() {
	c.cmdable.setProcessor(c.Process)
}

func (c *Client) Context() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func (c *Client) WithContext(ctx context.Context) *Client {
	if ctx == nil {
		panic("nil context")
	}
	c2 := c.copy()
	c2.ctx = ctx
	return c2
}

func (c *Client) copy() *Client {
	cp := *c
	cp.init()
	return &cp
}

// Options returns read-only Options that were used to create the client.
func (c *Client) Options() *Options {
	return c.opt
}

type PoolStats pool.Stats

// PoolStats returns connection pool stats.
func (c *Client) PoolStats() *PoolStats {
	stats := c.connPool.Stats()
	return (*PoolStats)(stats)
}

func (c *Client) Pipelined(fn func(Pipeliner) error) ([]Cmder, error) {
	return c.Pipeline().Pipelined(fn)
}

func (c *Client) Pipeline() Pipeliner {
	pipe := Pipeline{
		exec: c.processPipeline,
	}
	pipe.statefulCmdable.setProcessor(pipe.Process)
	return &pipe
}

func (c *Client) TxPipelined(fn func(Pipeliner) error) ([]Cmder, error) {
	return c.TxPipeline().Pipelined(fn)
}

// TxPipeline acts like Pipeline, but wraps queued commands with MULTI/EXEC.
func (c *Client) TxPipeline() Pipeliner {
	pipe := Pipeline{
		exec: c.processTxPipeline,
	}
	pipe.statefulCmdable.setProcessor(pipe.Process)
	return &pipe
}

func (c *Client) pubSub() *PubSub {
	return &PubSub{
		opt: c.opt,

		newConn: func(channels []string) (*pool.Conn, error) {
			return c.newConn()
		},
		closeConn: c.connPool.CloseConn,
	}
}

// Subscribe subscribes the client to the specified channels.
// Channels can be omitted to create empty subscription.
func (c *Client) Subscribe(channels ...string) *PubSub {
	pubsub := c.pubSub()
	if len(channels) > 0 {
		_ = pubsub.Subscribe(channels...)
	}
	return pubsub
}

// PSubscribe subscribes the client to the given patterns.
// Patterns can be omitted to create empty subscription.
func (c *Client) PSubscribe(channels ...string) *PubSub {
	pubsub := c.pubSub()
	if len(channels) > 0 {
		_ = pubsub.PSubscribe(channels...)
	}
	return pubsub
}

//------------------------------------------------------------------------------

// Conn is like Client, but its pool contains single connection.
type Conn struct {
	baseClient
	statefulCmdable
}

func newConn(opt *Options, cn *pool.Conn) *Conn {
	c := Conn{
		baseClient: baseClient{
			opt:      opt,
			connPool: pool.NewSingleConnPool(cn),
		},
	}
	c.baseClient.init()
	c.statefulCmdable.setProcessor(c.Process)
	return &c
}

func (c *Conn) Pipelined(fn func(Pipeliner) error) ([]Cmder, error) {
	return c.Pipeline().Pipelined(fn)
}

func (c *Conn) Pipeline() Pipeliner {
	pipe := Pipeline{
		exec: c.processPipeline,
	}
	pipe.statefulCmdable.setProcessor(pipe.Process)
	return &pipe
}

func (c *Conn) TxPipelined(fn func(Pipeliner) error) ([]Cmder, error) {
	return c.TxPipeline().Pipelined(fn)
}

// TxPipeline acts like Pipeline, but wraps queued commands with MULTI/EXEC.
func (c *Conn) TxPipeline() Pipeliner {
	pipe := Pipeline{
		exec: c.processTxPipeline,
	}
	pipe.statefulCmdable.setProcessor(pipe.Process)
	return &pipe
}
