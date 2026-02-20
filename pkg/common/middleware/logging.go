// pkg/common/middleware/logging.go
package middleware

import (
	"net/http"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/google/uuid"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Create response writer wrapper
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Add request ID to headers
		w.Header().Set("X-Request-ID", requestID)

		// Process request
		next.ServeHTTP(rw, r)

		// Log request details
		logger.Info().
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("query", r.URL.RawQuery).
			Int("status", rw.statusCode).
			Int("size", rw.size).
			Dur("duration", time.Since(start)).
			Str("ip", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Str("referer", r.Referer()).
			Msg("HTTP request")
	})
}

func LoggingMiddlewareWithSkip(paths []string) func(http.Handler) http.Handler {
	skipMap := make(map[string]bool)
	for _, path := range paths {
		skipMap[path] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skipMap[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(rw, r)

			logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", rw.statusCode).
				Dur("duration", time.Since(start)).
				Msg("HTTP request")
		})
	}
}

type LogEntry struct {
	RequestID string        `json:"request_id"`
	Method    string        `json:"method"`
	Path      string        `json:"path"`
	Status    int           `json:"status"`
	Size      int           `json:"size"`
	Duration  time.Duration `json:"duration"`
	IP        string        `json:"ip"`
	UserAgent string        `json:"user_agent"`
	Referer   string        `json:"referer"`
	Timestamp time.Time     `json:"timestamp"`
}

func (e *LogEntry) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"request_id": e.RequestID,
		"method":     e.Method,
		"path":       e.Path,
		"status":     e.Status,
		"size":       e.Size,
		"duration":   e.Duration.Seconds(),
		"ip":         e.IP,
		"user_agent": e.UserAgent,
		"referer":    e.Referer,
		"timestamp":  e.Timestamp,
	}
}
