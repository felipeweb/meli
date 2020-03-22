package redis

import (
	"errors"
	u "net/url"

	"github.com/go-redis/redis/v7"
	"github.com/rs/xid"
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

var ParseURL = redis.ParseURL

//create redis client
func NewClient(config *Config) (*Client, error) {
	cli := redis.NewClient(&redis.Options{
		Addr:       config.Addr,
		Password:   config.Password,
		DB:         0, //DEFAULT
		MaxRetries: config.MaxRetries,
	})

	client := &Client{cli}
	_, err := cli.Ping().Result()
	if err != nil {
		return nil, err
	}

	return client, nil
}

// find value pair by key
func (client *Client) Find(id string) (string, error) {
	cli := client.cli
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
	cli := client.cli
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

	//check validity of url
	_, err := u.ParseRequestURI(url)
	if err != nil {
		return "", ErrInvalidURL
	}

	cli := client.cli
	//decode value, shorten url
	val := xid.New().String()

	//set key-value to redis client
	err = cli.Set(val, url, 0).Err() //set no expire-time
	if err != nil {
		return "", err
	}
	return val, nil
}
