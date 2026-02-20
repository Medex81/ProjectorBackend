// pkg/common/middleware/metrics.go
package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create response writer wrapper
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Process request
		next.ServeHTTP(rw, r)

		// Record metrics
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(rw.statusCode)

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)

		// Record request size if available
		if r.ContentLength > 0 {
			metrics.HTTPRequestSize.WithLabelValues(r.Method, r.URL.Path).Observe(float64(r.ContentLength))
		}

		// Record response size
		metrics.HTTPResponseSize.WithLabelValues(r.Method, r.URL.Path, status).Observe(float64(rw.size))

		// Record active requests
		metrics.HTTPActiveRequests.WithLabelValues(r.Method).Inc()
		defer metrics.HTTPActiveRequests.WithLabelValues(r.Method).Dec()
	})
}

func MetricsMiddlewareWithBuckets(buckets []float64) func(http.Handler) http.Handler {
	// Create custom duration histogram
	durationHistogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: buckets,
		},
		[]string{"method", "path"},
	)
	prometheus.MustRegister(durationHistogram)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(rw, r)

			duration := time.Since(start).Seconds()
			durationHistogram.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
		})
	}
}

func InstrumentHandler(handler http.Handler, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create instrumented response writer
		rw := &responseWriter{ResponseWriter: w}

		// Record before
		metrics.HTTPInFlightRequests.WithLabelValues(name).Inc()
		defer metrics.HTTPInFlightRequests.WithLabelValues(name).Dec()

		start := time.Now()

		// Handle request
		handler.ServeHTTP(rw, r)

		// Record after
		duration := time.Since(start).Seconds()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, name).Observe(duration)
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, name, strconv.Itoa(rw.statusCode)).Inc()
	})
}

// Prometheus metrics definitions
var (
	HTTPRequestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_size_bytes",
			Help:    "Size of HTTP requests in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path"},
	)

	HTTPResponseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_response_size_bytes",
			Help:    "Size of HTTP responses in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path", "status"},
	)

	HTTPActiveRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_active_requests",
			Help: "Number of active HTTP requests",
		},
		[]string{"method"},
	)

	HTTPInFlightRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_in_flight_requests",
			Help: "Number of in-flight HTTP requests",
		},
		[]string{"handler"},
	)
)

func init() {
	prometheus.MustRegister(HTTPRequestSize)
	prometheus.MustRegister(HTTPResponseSize)
	prometheus.MustRegister(HTTPActiveRequests)
	prometheus.MustRegister(HTTPInFlightRequests)
}
