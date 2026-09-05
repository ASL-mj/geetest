package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/captchaflow/service-platform/api/internal/crypto"
)

// clientIP resolves the request's source address. With trustedProxyHops = 0
// (the default, direct exposure) it is exactly RemoteAddr; otherwise it
// walks X-Forwarded-For from the right, skipping the trusted proxy hops the
// operator declares, so an allowlist deployed behind nginx keeps working.
func clientIP(r *http.Request, trustedProxyHops int) string {
	if trustedProxyHops > 0 {
		xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
		if xff != "" {
			parts := strings.Split(xff, ",")
			idx := len(parts) - 1 - trustedProxyHops
			if idx < 0 {
				idx = 0
			}
			if candidate := strings.TrimSpace(parts[idx]); candidate != "" {
				return candidate
			}
		}
	}
	host := r.RemoteAddr
	if hostOnly, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = hostOnly
	}
	return host
}

// ipWindowLimiter is a small in-memory fixed-window limiter for the two
// unauthenticated hot spots (admin login, CDK activation). Single-instance
// only by design; a multi-instance deployment should move this to Redis.
type ipWindowLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	counts    map[string]*ipWindow
	nowFunc   func() time.Time
	lastSweep time.Time
}

type ipWindow struct {
	start time.Time
	count int
}

func newIPWindowLimiter(limit int, window time.Duration) *ipWindowLimiter {
	return &ipWindowLimiter{
		limit:     limit,
		window:    window,
		counts:    make(map[string]*ipWindow),
		nowFunc:   time.Now,
		lastSweep: time.Now(),
	}
}

// allow reports whether ip may proceed in the current window.
func (l *ipWindowLimiter) allow(ip string) bool {
	now := l.nowFunc()
	l.mu.Lock()
	defer l.mu.Unlock()
	// Cheap periodic GC so long-tail IPs cannot grow the map forever.
	if now.Sub(l.lastSweep) > 10*time.Minute {
		for key, window := range l.counts {
			if now.Sub(window.start) >= l.window {
				delete(l.counts, key)
			}
		}
		l.lastSweep = now
	}
	window, ok := l.counts[ip]
	if !ok || now.Sub(window.start) >= l.window {
		l.counts[ip] = &ipWindow{start: now, count: 1}
		return true
	}
	window.count++
	return window.count <= l.limit
}

// requireThrottle wraps a handler with the per-IP fixed window. Rejections
// carry 429 with Retry-After set to the remaining window seconds.
func (s *Server) requireThrottle(limiter *ipWindowLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(clientIP(r, s.trustedProxyHops)) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, errorEnvelope{
				Success:   false,
				RequestID: crypto.NewRequestID(),
				Error:     errorBody{Code: "RATE_LIMITED", Message: "Too many attempts; retry later.", Retryable: true},
			})
			return
		}
		next(w, r)
	}
}
