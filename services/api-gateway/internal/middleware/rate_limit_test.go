// services/api-gateway/internal/middleware/rate_limit_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimitMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		requests       int
		interval       time.Duration
		expectedStatus []int // Status codes for each request
	}{
		{
			name:           "Under limit",
			requests:       5,
			interval:       10 * time.Millisecond,
			expectedStatus: []int{http.StatusOK, http.StatusOK, http.StatusOK, http.StatusOK, http.StatusOK},
		},
		{
			name:           "At limit",
			requests:       10,
			interval:       10 * time.Millisecond,
			expectedStatus: append(make([]int, 10), http.StatusOK),
		},
		{
			name:           "Over limit",
			requests:       15,
			interval:       10 * time.Millisecond,
			expectedStatus: append(append(make([]int, 10), http.StatusOK), http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests),
		},
		{
			name:           "Burst requests",
			requests:       20,
			interval:       1 * time.Nanosecond, // Almost simultaneous
			expectedStatus: append(append(make([]int, 10), http.StatusOK), http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup rate limiter: 10 requests per 100ms
			rateLimiter := NewRateLimiter(10, 100*time.Millisecond)

			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := RateLimitMiddleware(rateLimiter)(testHandler)

			// Make requests
			statuses := make([]int, tt.requests)
			for i := 0; i < tt.requests; i++ {
				req := httptest.NewRequest("GET", "/test", nil)
				req.RemoteAddr = "192.168.1.1:12345" // Same IP for all requests

				rr := httptest.NewRecorder()
				wrappedHandler.ServeHTTP(rr, req)
				statuses[i] = rr.Code

				time.Sleep(tt.interval)
			}

			// Check status codes
			assert.Equal(t, tt.expectedStatus, statuses)
		})
	}
}

func TestRateLimitMiddleware_DifferentIPs(t *testing.T) {
	rateLimiter := NewRateLimiter(5, 100*time.Millisecond)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := RateLimitMiddleware(rateLimiter)(testHandler)

	// Make requests from different IPs
	ips := []string{
		"192.168.1.1",
		"192.168.1.2",
		"192.168.1.3",
		"192.168.1.4",
		"192.168.1.5",
		"192.168.1.6", // 6th IP
	}

	statuses := make([]int, len(ips))
	for i, ip := range ips {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = ip + ":12345"

		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)
		statuses[i] = rr.Code
	}

	// All should be OK since different IPs have separate counters
	for _, status := range statuses {
		assert.Equal(t, http.StatusOK, status)
	}
}

func TestRateLimitMiddleware_ConcurrentRequests(t *testing.T) {
	rateLimiter := NewRateLimiter(50, time.Second)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := RateLimitMiddleware(rateLimiter)(testHandler)

	// Concurrent requests
	concurrency := 10
	requestsPerGoroutine := 20

	var wg sync.WaitGroup
	statusChan := make(chan int, concurrency*requestsPerGoroutine)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				req := httptest.NewRequest("GET", "/test", nil)
				req.RemoteAddr = "192.168.1.1:12345"

				rr := httptest.NewRecorder()
				wrappedHandler.ServeHTTP(rr, req)
				statusChan <- rr.Code
			}
		}()
	}

	wg.Wait()
	close(statusChan)

	// Count status codes
	okCount := 0
	tooManyCount := 0
	for status := range statusChan {
		if status == http.StatusOK {
			okCount++
		} else {
			tooManyCount++
		}
	}

	// Should have exactly 50 OK, rest rate limited
	assert.Equal(t, 50, okCount, "Should have exactly 50 successful requests")
	assert.Equal(t, 150, tooManyCount, "Should have 150 rate-limited requests")
}
