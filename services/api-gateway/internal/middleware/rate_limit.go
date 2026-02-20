// services/api-gateway/internal/middleware/rate_limit.go
package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/metrics"
)

type RateLimiter struct {
	requests map[string][]time.Time
	mu       sync.RWMutex
	limit    int // requests per interval
	interval time.Duration
}

type RateLimitConfig struct {
	Limit    int
	Interval time.Duration
}

func NewRateLimiter(limit int, interval time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		interval: interval,
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.interval)

	// Clean up old requests
	if timestamps, exists := rl.requests[ip]; exists {
		var valid []time.Time
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				valid = append(valid, ts)
			}
		}
		rl.requests[ip] = valid
	}

	// Check limit
	if len(rl.requests[ip]) >= rl.limit {
		return false
	}

	// Add new request
	rl.requests[ip] = append(rl.requests[ip], now)
	return true
}

func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.interval)

	for ip, timestamps := range rl.requests {
		var valid []time.Time
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				valid = append(valid, ts)
			}
		}
		if len(valid) == 0 {
			delete(rl.requests, ip)
		} else {
			rl.requests[ip] = valid
		}
	}
}

func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	// Start cleanup goroutine
	go func() {
		ticker := time.NewTicker(limiter.interval)
		defer ticker.Stop()
		for range ticker.C {
			limiter.Cleanup()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = forwarded
			}

			if !limiter.Allow(ip) {
				logger.Warn().Str("ip", ip).Msg("Rate limit exceeded")
				metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "429").Inc()

				w.Header().Set("X-RateLimit-Limit", string(rune(limiter.limit)))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", string(rune(int(limiter.interval.Seconds()))))

				http.Error(w, "Too many requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func PerClientRateLimiter(limit int, interval time.Duration) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(limit, interval)
	return RateLimitMiddleware(limiter)
}

func PerRouteRateLimiter(limits map[string]*RateLimitConfig) func(http.Handler) http.Handler {
	limiters := make(map[string]*RateLimiter)
	for route, config := range limits {
		limiters[route] = NewRateLimiter(config.Limit, config.Interval)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := r.URL.Path
			if limiter, exists := limiters[route]; exists {
				ip := r.RemoteAddr
				if !limiter.Allow(ip) {
					http.Error(w, "Too many requests", http.StatusTooManyRequests)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
