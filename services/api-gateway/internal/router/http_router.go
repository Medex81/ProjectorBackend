// services/api-gateway/internal/router/http_router.go
package router

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/metrics"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"github.com/Medex81/ProjectorBackend/services/api-gateway/internal/middleware"
	"github.com/Medex81/ProjectorBackend/services/api-gateway/internal/proxy"
	"github.com/Medex81/ProjectorBackend/services/api-gateway/internal/spec"
	"github.com/gorilla/mux"
)

type HTTPRouter struct {
	router      *mux.Router
	config      *config.Config
	jwtManager  *auth.JWTManager
	specHandler *spec.SpecHandler
	proxies     map[string]*proxy.ServiceProxy
}

func NewHTTPRouter(cfg *config.Config, jwtManager *auth.JWTManager, specHandler *spec.SpecHandler) *HTTPRouter {
	r := &HTTPRouter{
		router:      mux.NewRouter(),
		config:      cfg,
		jwtManager:  jwtManager,
		specHandler: specHandler,
		proxies:     make(map[string]*proxy.ServiceProxy),
	}

	r.setupMiddleware()
	r.setupRoutes()

	return r
}

func (r *HTTPRouter) setupMiddleware() {
	// Add middleware in order
	r.router.Use(middleware.CORSMiddleware)
	r.router.Use(r.metricsMiddleware)
	r.router.Use(r.tracingMiddleware)
	r.router.Use(r.loggingMiddleware)
	r.router.Use(middleware.PerClientRateLimiter(100, time.Minute))
}

func (r *HTTPRouter) setupRoutes() {
	// Health check
	r.router.HandleFunc("/health", r.healthHandler).Methods("GET")

	// Metrics endpoint for Prometheus
	r.router.Handle("/metrics", metrics.Handler())

	// OpenAPI spec endpoints
	specRouter := r.router.PathPrefix("/api/v1/spec").Subrouter()
	specRouter.HandleFunc("", r.getSpecHandler).Methods("GET")
	specRouter.HandleFunc("/{service}", r.getServiceSpecHandler).Methods("GET")

	// Dynamic routes from service specs
	r.router.PathPrefix("/api/v1/").Handler(r.dynamicRouter())

	// Static routes (if any)
	r.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
}

func (r *HTTPRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.router.ServeHTTP(w, req)
}

func (r *HTTPRouter) healthHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"time":    time.Now(),
		"service": r.config.Service.Name,
	})
}

func (r *HTTPRouter) getSpecHandler(w http.ResponseWriter, req *http.Request) {
	ctx, span := tracing.StartSpan(req.Context(), "router.getSpec")
	defer span.End()

	spec := r.specHandler.GetSpec()

	// Check ETag
	etag := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%v", spec))))
	if req.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")

	json.NewEncoder(w).Encode(spec)
}

func (r *HTTPRouter) getServiceSpecHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	serviceName := vars["service"]

	spec := r.specHandler.GetServiceSpec(serviceName)
	if spec == nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(spec)
}

func (r *HTTPRouter) dynamicRouter() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Extract service and path from URL
		path := strings.TrimPrefix(req.URL.Path, "/api/v1/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 0 {
			http.NotFound(w, req)
			return
		}

		serviceName := parts[0]
		servicePath := "/"
		if len(parts) > 1 {
			servicePath = "/" + parts[1]
		}

		// Get or create proxy for service
		proxy, err := r.getServiceProxy(serviceName)
		if err != nil {
			logger.Error().Err(err).Str("service", serviceName).Msg("Service not found")
			http.Error(w, "Service not found", http.StatusNotFound)
			return
		}

		// Update request path
		req.URL.Path = servicePath

		// Proxy request
		proxy.ServeHTTP(w, req)
	})
}

func (r *HTTPRouter) getServiceProxy(serviceName string) (*proxy.ServiceProxy, error) {
	// Check if proxy exists
	if p, ok := r.proxies[serviceName]; ok {
		return p, nil
	}

	// Get service spec
	spec := r.specHandler.GetServiceSpec(serviceName)
	if spec == nil {
		return nil, fmt.Errorf("service %s not found", serviceName)
	}

	// Create new proxy
	p := proxy.NewServiceProxy(spec)
	r.proxies[serviceName] = p

	return p, nil
}

func (r *HTTPRouter) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()

		// Create response writer wrapper to capture status
		wrapper := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapper, req)

		logger.Info().
			Str("method", req.Method).
			Str("path", req.URL.Path).
			Int("status", wrapper.statusCode).
			Dur("duration", time.Since(start)).
			Str("ip", req.RemoteAddr).
			Str("user-agent", req.UserAgent()).
			Msg("HTTP request")
	})
}

func (r *HTTPRouter) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()

		wrapper := &responseWriter{ResponseWriter: w}

		next.ServeHTTP(wrapper, req)

		duration := time.Since(start).Seconds()
		metrics.HTTPRequestDuration.WithLabelValues(req.Method, req.URL.Path).Observe(duration)
		metrics.HTTPRequestsTotal.WithLabelValues(req.Method, req.URL.Path,
			fmt.Sprintf("%d", wrapper.statusCode)).Inc()
	})
}

func (r *HTTPRouter) tracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx, span := tracing.StartSpan(req.Context(), "http."+req.Method)
		defer span.End()

		// Add trace ID to response headers
		traceID := span.SpanContext().TraceID().String()
		w.Header().Set("X-Trace-ID", traceID)

		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
