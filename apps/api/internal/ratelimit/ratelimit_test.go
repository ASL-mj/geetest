package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/captchaflow/service-platform/api/internal/config"
)

func testLimiter(t *testing.T, perMinute, concurrency int) (*RedisLimiter, *fakeClock) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	settings := config.Settings{
		SolverTotalTimeout: time.Second,
		RateLimitPerMinute: perMinute,
		ConcurrencyLimit:   concurrency,
	}
	limiter := NewFromClient(client, settings)
	clock := &fakeClock{current: time.UnixMilli(1_700_000_000_000)}
	limiter.now = clock.Now
	return limiter, clock
}

// fakeClock is a controllable time source for deterministic bucket refill
// and lease expiry testing.
type fakeClock struct {
	current time.Time
}

func (c *fakeClock) Now() time.Time { return c.current }

func (c *fakeClock) Advance(d time.Duration) { c.current = c.current.Add(d) }

func TestTokenBucketAllowsUpToCapacity(t *testing.T) {
	limiter, _ := testLimiter(t, 3, 4)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		allowed, _, err := limiter.AllowRate(ctx, "cdk-1")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("call %d denied within capacity", i)
		}
	}
	allowed, _, err := limiter.AllowRate(ctx, "cdk-1")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("4th call must be denied at capacity 3")
	}

	// A different CDK has its own bucket.
	allowed, _, err = limiter.AllowRate(ctx, "cdk-2")
	if err != nil || !allowed {
		t.Fatalf("independent bucket expected, got allowed=%v err=%v", allowed, err)
	}
}

func TestTokenBucketRefillsOverTime(t *testing.T) {
	limiter, clock := testLimiter(t, 1, 4)
	ctx := context.Background()

	if allowed, _, _ := limiter.AllowRate(ctx, "cdk-1"); !allowed {
		t.Fatal("first call must pass")
	}
	if allowed, _, _ := limiter.AllowRate(ctx, "cdk-1"); allowed {
		t.Fatal("second call must be denied before refill")
	}
	clock.Advance(70 * time.Second)
	if allowed, _, _ := limiter.AllowRate(ctx, "cdk-1"); !allowed {
		t.Fatal("call must pass after a minute elapsed")
	}
}

func TestConcurrencyLeaseBlocksAndReleases(t *testing.T) {
	limiter, _ := testLimiter(t, 60, 2)
	ctx := context.Background()

	if ok, _ := limiter.AcquireConc(ctx, "cdk-1", "req-1"); !ok {
		t.Fatal("first lease must succeed")
	}
	if ok, _ := limiter.AcquireConc(ctx, "cdk-1", "req-2"); !ok {
		t.Fatal("second lease must succeed")
	}
	if ok, _ := limiter.AcquireConc(ctx, "cdk-1", "req-3"); ok {
		t.Fatal("third lease must be denied at concurrency 2")
	}
	// A different CDK is unaffected.
	if ok, _ := limiter.AcquireConc(ctx, "cdk-2", "req-4"); !ok {
		t.Fatal("independent CDK lease must succeed")
	}

	if err := limiter.ReleaseConc(ctx, "cdk-1", "req-1"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := limiter.AcquireConc(ctx, "cdk-1", "req-5"); !ok {
		t.Fatal("lease must be available after release")
	}
}

func TestConcurrencyLeaseExpiresViaTTL(t *testing.T) {
	limiter, clock := testLimiter(t, 60, 1)
	ctx := context.Background()

	if ok, _ := limiter.AcquireConc(ctx, "cdk-1", "req-1"); !ok {
		t.Fatal("first lease must succeed")
	}
	if ok, _ := limiter.AcquireConc(ctx, "cdk-1", "req-2"); ok {
		t.Fatal("second concurrent lease must be denied")
	}
	// Advance past the lease TTL (total timeout + 10s backstop).
	clock.Advance(20 * time.Second)
	if ok, _ := limiter.AcquireConc(ctx, "cdk-1", "req-2"); !ok {
		t.Fatal("expired lease must free the slot")
	}
}
