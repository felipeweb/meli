package observability

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

// Pool metrics:
// 1. Connections taken
// 2. Connections closed
// 3. Connections usetime -- how long is a connection used until it is closed, discarded or returned
// 4. Connections reused
// 4. Connections stale
// 5. Dial errors

const dimensionless = "1"
const unitMilliseconds = "ms"

var (
	MBytesRead                     = stats.Int64("redis/bytes_read", "The number of bytes read from the server", stats.UnitBytes)
	MBytesWritten                  = stats.Int64("redis/bytes_written", "The number of bytes written out to the server", stats.UnitBytes)
	MDialErrors                    = stats.Int64("redis/dial_errors", "The number of dial errors", dimensionless)
	MConnectionsTaken              = stats.Int64("redis/connections_taken", "The number of connections taken", dimensionless)
	MConnectionsClosed             = stats.Int64("redis/connections_closed", "The number of connections closed", dimensionless)
	MConnectionsReturned           = stats.Int64("redis/connections_returned", "The number of connections returned to the pool", dimensionless)
	MConnectionsReused             = stats.Int64("redis/connections_reused", "The number of connections reused", dimensionless)
	MConnectionsNew                = stats.Int64("redis/connections_new", "The number of newly created connections", dimensionless)
	MConnectionUseTimeMilliseconds = stats.Float64("redis/connection_usetime", "The number of milliseconds for which a connection is used", unitMilliseconds)
	MRoundtripLatencyMilliseconds  = stats.Float64("redis/roundtrip_latency", "The time between sending the first byte to the server until the last byte of response is received back", unitMilliseconds)
	MWriteErrors                   = stats.Int64("redis/write_errors", "The number of errors encountered during write routines", dimensionless)
	MWrites                        = stats.Int64("redis/writes", "The number of write invocations", dimensionless)
)

var KeyCommandName, _ = tag.NewKey("cmd")

var defaultMillisecondsDistribution = view.Distribution(
	// [0ms, 0.01ms, 0.05ms, 0.1ms, 0.5ms, 1ms, 1.5ms, 2ms, 2.5ms, 5ms, 10ms, 25ms, 50ms, 100ms, 200ms, 400ms, 600ms, 800ms, 1s, 1.5s, 2.5s, 5s, 10s, 20s, 40s, 100s, 200s, 500s]
	0, 0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.0015, 0.002, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.4, 0.6, 0.8, 1, 1.5, 2.5, 5, 10, 20, 40, 100, 200, 500,
)

var defaultBytesDistribution = view.Distribution(
	// [0, 1KB, 2KB, 4KB, 16KB, 64KB, 256KB,   1MB,     4MB,     16MB,     64MB,     256MB,     1GB,        4GB]
	0, 1024, 2048, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864, 268435456, 1073741824, 4294967296,
)

var Views = []*view.View{
	{
		Name:        "redis/client/connection_usetime",
		Description: "The duration in milliseconds for which a connection is used before being returned to the pool, closed or discarded",

		Aggregation: defaultMillisecondsDistribution,
		Measure:     MConnectionUseTimeMilliseconds,
	},
	{
		Name:        "redis/client/dial_errors",
		Description: "The number of errors encountered after dialling",
		Aggregation: view.Count(),
		Measure:     MDialErrors,
	},
	{
		Name:        "redis/client/bytes_written_cummulative",
		Description: "The number of bytes written out to the server",
		Aggregation: view.Count(),
		Measure:     MBytesWritten,
	},
	{
		Name:        "redis/client/bytes_written_distribution",
		Description: "The number of distribution of bytes written out to the server",
		Aggregation: defaultBytesDistribution,
		Measure:     MBytesWritten,
	},
	{
		Name:        "redis/client/bytes_read_cummulative",
		Description: "The number of bytes read from a response from the server",
		Aggregation: view.Count(),
		Measure:     MBytesRead,
	},
	{
		Name:        "redis/client/bytes_read_distribution",
		Description: "The number of distribution of bytes read from the server",
		Aggregation: defaultBytesDistribution,
		Measure:     MBytesRead,
	},
	{
		Name:        "redis/client/roundtrip_latency",
		Description: "The distribution of milliseconds of the roundtrip latencies",
		Aggregation: defaultMillisecondsDistribution,
		Measure:     MRoundtripLatencyMilliseconds,
		TagKeys:     []tag.Key{KeyCommandName},
	},
	{
		Name:        "redis/client/write_errors",
		Description: "The number of errors encountered during a write routine",
		Aggregation: view.Count(),
		Measure:     MWriteErrors,
		TagKeys:     []tag.Key{KeyCommandName},
	},
	{
		Name:        "redis/client/writes",
		Description: "The number of write invocations",
		Aggregation: view.Count(),
		Measure:     MWrites,
		TagKeys:     []tag.Key{KeyCommandName},
	},
	{
		Name:        "redis/client/connections_taken",
		Description: "The number of connections taken out the pool",
		Aggregation: view.Count(),
		Measure:     MConnectionsTaken,
	},
	{
		Name:        "redis/client/connections_returned",
		Description: "The number of connections returned the connection pool",
		Aggregation: view.Count(),
		Measure:     MConnectionsReturned,
	},
	{
		Name:        "redis/client/connections_reused",
		Description: "The number of connections reused",
		Aggregation: view.Count(),
		Measure:     MConnectionsReused,
	},
	{
		Name:        "redis/client/connections_new",
		Description: "The number of newly created connections",
		Aggregation: view.Count(),
		Measure:     MConnectionsNew,
	},
}
