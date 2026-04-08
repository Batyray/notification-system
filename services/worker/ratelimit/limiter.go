package ratelimit

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

// slidingWindowScript is a Lua script that atomically checks and increments
// a sliding window rate limiter using Redis sorted sets.
//
// KEYS[1] = rate_limit:{channel}
// ARGV[1] = window start (now - window)
// ARGV[2] = now (seconds with millisecond precision)
// ARGV[3] = limit
// ARGV[4] = member (now-random for uniqueness)
// ARGV[5] = TTL in seconds
var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local window_start = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local ttl = tonumber(ARGV[5])

redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)

local count = redis.call('ZCARD', key)

if count < limit then
    redis.call('ZADD', key, now, member)
    redis.call('EXPIRE', key, ttl)
    return 1
else
    return 0
end
`)

// RateLimitedError is returned when a request exceeds the rate limit for a channel.
type RateLimitedError struct {
	Channel string
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("rate limited on channel: %s", e.Channel)
}

// Limiter implements a sliding window rate limiter backed by Redis sorted sets.
type Limiter struct {
	rdb    *redis.Client
	limit  int
	window time.Duration
}

// New creates a new Limiter.
func New(rdb *redis.Client, limit int, window time.Duration) *Limiter {
	return &Limiter{
		rdb:    rdb,
		limit:  limit,
		window: window,
	}
}

// Allow checks whether a request on the given channel is permitted under the
// rate limit. It returns true if the request is allowed, false otherwise.
func (l *Limiter) Allow(ctx context.Context, channel string) (bool, error) {
	now := float64(time.Now().UnixMilli()) / 1000.0
	windowStart := now - l.window.Seconds()
	member := fmt.Sprintf("%f-%d", now, rand.Int63())
	ttl := int(l.window.Seconds()) + 1

	key := fmt.Sprintf("rate_limit:%s", channel)

	result, err := slidingWindowScript.Run(ctx, l.rdb, []string{key}, windowStart, now, l.limit, member, ttl).Int()
	if err != nil {
		return false, fmt.Errorf("rate limiter script error: %w", err)
	}

	return result == 1, nil
}
