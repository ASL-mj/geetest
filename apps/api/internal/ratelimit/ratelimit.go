// Package ratelimit implements the per-CDK admission controls: a Redis token
// bucket for the per-minute rate and TTL-backed leases for concurrency.
// Redis failures fail open (logged) so platform availability survives a
// degraded cache, while every allowance still passes quota admission.
package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/captchaflow/service-platform/api/internal/config"
)

// Limiter is the admission-control surface used by the solve service.
type Limiter interface {
	// AllowRate reports whether the CDK may start another call this minute.
	AllowRate(ctx context.Context, cdkID string) (bool, time.Duration, error)
	// AcquireConc takes a concurrency lease; Release must be called when the
	// request finishes regardless of outcome.
	AcquireConc(ctx context.Context, cdkID, requestID string) (bool, error)
	ReleaseConc(ctx context.Context, cdkID, requestID string) error
}

// tokenBucketLua: refill by elapsed time, consume one token, return 1/0.
var tokenBucketLua = redis.NewScript(`
local key      = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill   = tonumber(ARGV[2])
local now      = tonumber(ARGV[3])
local window   = tonumber(ARGV[4])

local state   = redis.call('HMGET', key, 'tokens', 'ts')
local tokens  = tonumber(state[1])
local ts      = tonumber(state[2])

if tokens == nil or ts == nil then
    tokens = capacity
    ts     = now
end

local elapsed = now - ts
if elapsed < 0 then elapsed = 0 end
tokens = math.min(capacity, tokens + elapsed * refill / window)

local allowed = 0
if tokens >= 1 then
    tokens  = tokens - 1
    allowed = 1
end

redis.call('HSET', key, 'tokens', tokens, 'ts', now)
redis.call('PEXPIRE', key, window * 2)
return allowed
`)

// acquireConcLua: drop expired leases, admit when under capacity, record the
// holder with its expiry so crashed requests self-clean.
var acquireConcLua = redis.NewScript(`
local key    = KEYS[1]
local now    = tonumber(ARGV[1])
local limit  = tonumber(ARGV[2])
local holder = ARGV[3]
local ttl    = tonumber(ARGV[4])

redis.call('ZREMRANGEBYSCORE', key, '-inf', now)
local count = redis.call('ZCARD', key)
if count >= limit then
    return 0
end
redis.call('ZADD', key, now + ttl, holder)
redis.call('PEXPIRE', key, ttl * 2)
return 1
`)

// RedisLimiter implements Limiter against a Redis instance.
type RedisLimiter struct {
	client      *redis.Client
	perMinute   int
	concurrency int
	leaseTTL    time.Duration
	// now is injectable so tests can drive refill and lease expiry
	// deterministically; production always uses time.Now.
	now func() time.Time
}

// New connects to the Redis URL; the limiter fails open when unusable. The
// URL is parsed with redis.ParseURL so credentials and DB numbers actually
// reach the connection — a hand-stripped addr used to silently disable all
// admission control the moment a password was configured.
func New(ctx context.Context, settings config.Settings) *RedisLimiter {
	options, err := redis.ParseURL(settings.RedisURL)
	if err != nil {
		slog.Error("invalid REDIS_URL; admission control fails open", "error", err)
		return NewFromClient(redis.NewClient(&redis.Options{Addr: "localhost:6379"}), settings)
	}
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		slog.Warn("redis unavailable; admission control fails open", "error", err)
	}
	return NewFromClient(client, settings)
}

// NewFromClient builds a limiter over an existing client (tests, clusters).
func NewFromClient(client *redis.Client, settings config.Settings) *RedisLimiter {
	return &RedisLimiter{
		client:      client,
		perMinute:   settings.RateLimitPerMinute,
		concurrency: settings.ConcurrencyLimit,
		leaseTTL:    settings.SolverTotalTimeout + 10*time.Second,
		now:         time.Now,
	}
}

// AllowRate consumes one token from the CDK bucket; retryAfter is the window
// length rendered as the Retry-After header.
func (l *RedisLimiter) AllowRate(ctx context.Context, cdkID string) (bool, time.Duration, error) {
	allowed, err := tokenBucketLua.Run(ctx, l.client,
		[]string{fmt.Sprintf("cdk:rate:%s", cdkID)},
		l.perMinute, l.perMinute, l.now().UnixMilli(), 60000,
	).Int()
	if err != nil {
		slog.Warn("rate limiter failed open", "error", err)
		return true, 0, err
	}
	return allowed == 1, time.Minute, nil
}

// AcquireConc takes one of the CDK's concurrency leases via a sorted set of
// holders scored by expiry; TTL doubles as the crash-recovery backstop.
func (l *RedisLimiter) AcquireConc(ctx context.Context, cdkID, requestID string) (bool, error) {
	admitted, err := acquireConcLua.Run(ctx, l.client,
		[]string{fmt.Sprintf("cdk:conc:%s", cdkID)},
		l.now().UnixMilli(), l.concurrency, requestID, l.leaseTTL.Milliseconds(),
	).Int()
	if err != nil {
		slog.Warn("concurrency limiter failed open", "error", err)
		return true, err
	}
	return admitted == 1, nil
}

// ReleaseConc drops this request's lease; a missing member is fine (TTL
// already fired or release ran twice).
func (l *RedisLimiter) ReleaseConc(ctx context.Context, cdkID, requestID string) error {
	key := fmt.Sprintf("cdk:conc:%s", cdkID)
	if err := l.client.ZRem(ctx, key, requestID).Err(); err != nil {
		slog.Warn("concurrency release failed", "error", err)
		return err
	}
	return nil
}
