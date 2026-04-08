package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLimiter(t *testing.T, limit int, window time.Duration) (*Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { _ = rdb.Close() })

	return New(rdb, limit, window), mr
}

func TestAllow_UnderLimit(t *testing.T) {
	limiter, _ := setupLimiter(t, 5, 1*time.Second)
	ctx := context.Background()

	allowed, err := limiter.Allow(ctx, "sms")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestAllow_OverLimit(t *testing.T) {
	limiter, _ := setupLimiter(t, 5, 1*time.Second)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, "sms")
		require.NoError(t, err)
		assert.True(t, allowed, "request %d should be allowed", i+1)
	}

	allowed, err := limiter.Allow(ctx, "sms")
	require.NoError(t, err)
	assert.False(t, allowed, "6th request should be rejected")
}

func TestAllow_ChannelsAreIsolated(t *testing.T) {
	limiter, _ := setupLimiter(t, 2, 1*time.Second)
	ctx := context.Background()

	// Fill up SMS channel
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(ctx, "sms")
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// SMS should be blocked
	allowed, err := limiter.Allow(ctx, "sms")
	require.NoError(t, err)
	assert.False(t, allowed, "sms should be rate limited")

	// Email should still be allowed
	allowed, err = limiter.Allow(ctx, "email")
	require.NoError(t, err)
	assert.True(t, allowed, "email should not be affected by sms limit")
}

func TestAllow_WindowExpiry(t *testing.T) {
	// Use a short window so we can wait for real time to pass,
	// testing the sliding window ZREMRANGEBYSCORE prune (not just TTL expiry).
	limiter, _ := setupLimiter(t, 2, 150*time.Millisecond)
	ctx := context.Background()

	// Fill up the limit
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(ctx, "push")
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// Should be blocked
	allowed, err := limiter.Allow(ctx, "push")
	require.NoError(t, err)
	assert.False(t, allowed)

	// Wait for real time to pass beyond the window
	time.Sleep(200 * time.Millisecond)

	// Should be allowed again — old entries pruned by ZREMRANGEBYSCORE
	allowed, err = limiter.Allow(ctx, "push")
	require.NoError(t, err)
	assert.True(t, allowed, "should be allowed after window expiry")
}
