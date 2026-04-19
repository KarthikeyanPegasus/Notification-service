package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spidey/notification-service/internal/config"
)

// Client wraps redis.Client with typed helpers.
type Client struct {
	RDB *redis.Client
}

func NewClient(cfg config.RedisConfig) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     20,
		MinIdleConns: 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	return &Client{RDB: rdb}, nil
}

func (c *Client) Close() error {
	return c.RDB.Close()
}

// Set stores a value with an expiry.
func (c *Client) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := marshal(value)
	if err != nil {
		return err
	}
	return c.RDB.Set(ctx, key, data, ttl).Err()
}

// Get retrieves a value and unmarshals into dest.
func (c *Client) Get(ctx context.Context, key string, dest any) error {
	data, err := c.RDB.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}
		return fmt.Errorf("redis get %s: %w", key, err)
	}
	return json.Unmarshal(data, dest)
}

// Exists returns true if the key exists.
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.RDB.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists %s: %w", key, err)
	}
	return n > 0, nil
}

// Del removes keys.
func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.RDB.Del(ctx, keys...).Err()
}

// Incr atomically increments a counter and returns the new value.
func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	return c.RDB.Incr(ctx, key).Result()
}

// Expire sets a TTL on an existing key.
func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.RDB.Expire(ctx, key, ttl).Err()
}

// SetNX sets key=value only if the key does not exist. Returns true if set.
func (c *Client) SetNX(ctx context.Context, key string, value any, ttl time.Duration) (bool, error) {
	data, err := marshal(value)
	if err != nil {
		return false, err
	}
	return c.RDB.SetNX(ctx, key, data, ttl).Result()
}

// SetEX is a convenience wrapper for SETEX (set with expiry).
func (c *Client) SetEX(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := marshal(value)
	if err != nil {
		return err
	}
	return c.RDB.Set(ctx, key, data, ttl).Err()
}

// SAdd adds members to a Redis set.
func (c *Client) SAdd(ctx context.Context, key string, members ...any) error {
	return c.RDB.SAdd(ctx, key, members...).Err()
}

// SMembers returns all members of a set.
func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.RDB.SMembers(ctx, key).Result()
}

// SRem removes a member from a set.
func (c *Client) SRem(ctx context.Context, key string, members ...any) error {
	return c.RDB.SRem(ctx, key, members...).Err()
}

// GetString is a typed convenience for string-valued keys.
func (c *Client) GetString(ctx context.Context, key string) (string, error) {
	val, err := c.RDB.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrCacheMiss
		}
		return "", err
	}
	return val, nil
}

// SetString stores a plain string value with TTL.
func (c *Client) SetString(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.RDB.Set(ctx, key, value, ttl).Err()
}

// ErrCacheMiss signals that a requested key is not present in the cache.
var ErrCacheMiss = errors.New("cache miss")

func marshal(v any) ([]byte, error) {
	switch val := v.(type) {
	case string:
		return []byte(val), nil
	case []byte:
		return val, nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshalling cache value: %w", err)
		}
		return data, nil
	}
}
