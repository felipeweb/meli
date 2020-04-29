package redis

import (
	"context"
	"errors"
	"fmt"
	u "net/url"

	"github.com/felipeweb/meli/pkg/redis/instrumentation/redis"
	"github.com/rs/xid"
	"go.opencensus.io/trace"
)

var (
	ErrKeyNotFound = errors.New("redis does not contain key")
	ErrInvalidURL  = errors.New("not a valid url")
)

type Client struct {
	cli *redis.Client
}

//Custom config
type Config struct {
	Addr       string
	Password   string
	DB         int
	MaxRetries int
}

//create redis client
func NewClient(ctx context.Context, redisURL string) (*Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse redis URL: %w", err)
	}
	if opts.MaxRetries < 2 {
		opts.MaxRetries = 3
	}
	cli := redis.NewClient(&redis.Options{
		Addr:       opts.Addr,
		Password:   opts.Password,
		DB:         opts.DB,
		MaxRetries: opts.MaxRetries,
		Context:    ctx,
	})
	client := &Client{cli}
	_, err = cli.Ping().Result()
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (client *Client) Context() context.Context {
	return client.cli.Context()
}

func (client *Client) Close() {
	client.cli.Close()
}

// find value pair by key
func (client *Client) Find(id string) (string, error) {
	ctx, span := trace.StartSpan(client.cli.Context(), "redis.find")
	defer span.End()
	cli := client.cli.WithContext(ctx)
	url, err := cli.Get(id).Result()
	// does not contain key
	if err == redis.Nil {
		return "", ErrKeyNotFound
	}
	if err != nil {
		return "", err
	}
	return url, nil
}

func (client *Client) Del(id string) error {
	ctx, span := trace.StartSpan(client.cli.Context(), "redis.del")
	defer span.End()
	cli := client.cli.WithContext(ctx)
	_, err := client.Find(id)
	if err != nil {
		return err
	}
	_, err = cli.Del(id).Result()
	if err != nil {
		return err
	}
	return nil
}

// set key-value pair
func (client *Client) Set(url string) (string, error) {
	ctx, span := trace.StartSpan(client.cli.Context(), "redis.find")
	defer span.End()
	cli := client.cli.WithContext(ctx)
	//check validity of url
	_, err := u.ParseRequestURI(url)
	if err != nil {
		return "", ErrInvalidURL
	}
	//decode value, shorten url
	val := xid.New().String()

	//set key-value to redis client
	err = cli.Set(val, url, 0).Err() //set no expire-time
	if err != nil {
		return "", err
	}
	return val, nil
}
