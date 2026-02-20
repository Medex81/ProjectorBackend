// pkg/common/middleware/tracing.go
package middleware

import (
	"net/http"

	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract trace context from headers
		ctx := tracing.Propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Start span
		ctx, span := tracing.StartSpan(ctx, "http."+r.Method)
		defer span.End()

		// Add request attributes
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
			attribute.String("http.host", r.Host),
			attribute.String("http.user_agent", r.UserAgent()),
			attribute.String("http.remote_addr", r.RemoteAddr),
		)

		// Add trace ID to response headers
		traceID := span.SpanContext().TraceID().String()
		w.Header().Set("X-Trace-ID", traceID)

		// Create response writer wrapper to capture status code
		rw := &responseWriter{ResponseWriter: w}

		// Call next handler with traced context
		next.ServeHTTP(rw, r.WithContext(ctx))

		// Add response attributes
		span.SetAttributes(
			attribute.Int("http.status_code", rw.statusCode),
			attribute.Int("http.response_size", rw.size),
		)
	})
}

func TracingMiddlewareWithService(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := tracing.Propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			ctx, span := tracing.StartSpan(ctx, serviceName+"."+r.Method)
			defer span.End()

			span.SetAttributes(
				attribute.String("service.name", serviceName),
				attribute.String("http.method", r.Method),
				attribute.String("http.path", r.URL.Path),
			)

			rw := &responseWriter{ResponseWriter: w}
			next.ServeHTTP(rw, r.WithContext(ctx))

			span.SetAttributes(attribute.Int("http.status_code", rw.statusCode))
		})
	}
}

func ExtractTraceInfo(r *http.Request) (traceID, spanID string) {
	span := trace.SpanFromContext(r.Context())
	if span.SpanContext().IsValid() {
		traceID = span.SpanContext().TraceID().String()
		spanID = span.SpanContext().SpanID().String()
	}
	return
}

type TracingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	traceID    string
}

func NewTracingResponseWriter(w http.ResponseWriter, traceID string) *TracingResponseWriter {
	return &TracingResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		traceID:        traceID,
	}
}

func (w *TracingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.Header().Set("X-Trace-ID", w.traceID)
	w.ResponseWriter.WriteHeader(code)
}
