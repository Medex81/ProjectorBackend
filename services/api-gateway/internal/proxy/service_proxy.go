// services/api-gateway/internal/proxy/service_proxy.go
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/metrics"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"github.com/google/uuid"
)

type ServiceProxy struct {
	serviceName string
	endpoints   []*Endpoint
	mu          sync.RWMutex
	roundRobin  int
	client      *http.Client
	healthCheck *HealthChecker
}

type Endpoint struct {
	URL       *url.URL
	Healthy   bool
	LastCheck time.Time
	FailCount int
}

type HealthChecker struct {
	interval time.Duration
	timeout  time.Duration
	path     string
	stop     chan bool
}

type ServiceRegistryClient struct {
	registryURL string
	client      *http.Client
}

func NewServiceProxy(spec *api.ServiceAPISpec) *ServiceProxy {
	endpoints := make([]*Endpoint, 0, len(spec.Spec.Servers))
	for _, server := range spec.Spec.Servers {
		u, err := url.Parse(server.URL)
		if err != nil {
			logger.Error().Err(err).Str("url", server.URL).Msg("Invalid server URL")
			continue
		}
		endpoints = append(endpoints, &Endpoint{
			URL:     u,
			Healthy: true,
		})
	}

	proxy := &ServiceProxy{
		serviceName: spec.ServiceName,
		endpoints:   endpoints,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// Start health checks
	proxy.healthCheck = &HealthChecker{
		interval: 30 * time.Second,
		timeout:  5 * time.Second,
		path:     "/health",
		stop:     make(chan bool),
	}
	go proxy.startHealthChecks()

	return proxy
}

func (p *ServiceProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "proxy."+p.serviceName)
	defer span.End()

	// Get healthy endpoint
	endpoint := p.getHealthyEndpoint()
	if endpoint == nil {
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "503").Inc()
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(endpoint.URL)
	proxy.ModifyResponse = p.modifyResponse
	proxy.ErrorHandler = p.errorHandler

	// Add tracing headers
	r.Header.Set("X-Request-ID", uuid.New().String())
	r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	r.Header.Set("X-Forwarded-Proto", r.URL.Scheme)

	// Log request
	logger.Info().
		Str("service", p.serviceName).
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("endpoint", endpoint.URL.String()).
		Msg("Proxying request")

	// Proxy request
	start := time.Now()
	proxy.ServeHTTP(w, r)
	duration := time.Since(start)

	// Record metrics
	metrics.HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration.Seconds())
	metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "200").Inc()
}

func (p *ServiceProxy) getHealthyEndpoint() *Endpoint {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.endpoints) == 0 {
		return nil
	}

	// Try to find a healthy endpoint
	for i := 0; i < len(p.endpoints); i++ {
		p.roundRobin = (p.roundRobin + 1) % len(p.endpoints)
		endpoint := p.endpoints[p.roundRobin]
		if endpoint.Healthy {
			return endpoint
		}
	}

	return nil
}

func (p *ServiceProxy) modifyResponse(resp *http.Response) error {
	// Add response headers
	resp.Header.Set("X-Service", p.serviceName)
	return nil
}

func (p *ServiceProxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	logger.Error().Err(err).Str("service", p.serviceName).Msg("Proxy error")

	// Mark endpoint as unhealthy
	p.markUnhealthy(r.URL)

	metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "502").Inc()
	http.Error(w, "Bad gateway", http.StatusBadGateway)
}

func (p *ServiceProxy) markUnhealthy(url *url.URL) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, endpoint := range p.endpoints {
		if endpoint.URL.Host == url.Host {
			endpoint.Healthy = false
			endpoint.LastCheck = time.Now()
			endpoint.FailCount++
			logger.Warn().Str("endpoint", endpoint.URL.String()).Msg("Marked endpoint unhealthy")
			break
		}
	}
}

func (p *ServiceProxy) startHealthChecks() {
	ticker := time.NewTicker(p.healthCheck.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.checkHealth()
		case <-p.healthCheck.stop:
			return
		}
	}
}

func (p *ServiceProxy) checkHealth() {
	p.mu.RLock()
	endpoints := make([]*Endpoint, len(p.endpoints))
	copy(endpoints, p.endpoints)
	p.mu.RUnlock()

	for _, endpoint := range endpoints {
		if !endpoint.Healthy && time.Since(endpoint.LastCheck) < time.Minute {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), p.healthCheck.timeout)
		defer cancel()

		healthURL := endpoint.URL.String() + p.healthCheck.path
		req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
		if err != nil {
			continue
		}

		resp, err := p.client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			p.updateEndpointHealth(endpoint.URL, false)
			continue
		}
		defer resp.Body.Close()

		p.updateEndpointHealth(endpoint.URL, true)
	}
}

func (p *ServiceProxy) updateEndpointHealth(url *url.URL, healthy bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, endpoint := range p.endpoints {
		if endpoint.URL.Host == url.Host {
			endpoint.Healthy = healthy
			endpoint.LastCheck = time.Now()
			if healthy {
				endpoint.FailCount = 0
				logger.Info().Str("endpoint", endpoint.URL.String()).Msg("Endpoint healthy")
			}
			break
		}
	}
}

func (p *ServiceProxy) Stop() {
	if p.healthCheck != nil {
		p.healthCheck.stop <- true
	}
}

// ServiceRegistryClient implementation
func NewServiceRegistryClient(cfg *config.Config) *ServiceRegistryClient {
	return &ServiceRegistryClient{
		registryURL: fmt.Sprintf("http://%s:%d", cfg.ServiceRegistry.Host, cfg.ServiceRegistry.Port),
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *ServiceRegistryClient) GetAllServiceSpecs() ([]*api.ServiceAPISpec, error) {
	resp, err := c.client.Get(c.registryURL + "/api/v1/services")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var specs []*api.ServiceAPISpec
	if err := json.NewDecoder(resp.Body).Decode(&specs); err != nil {
		return nil, err
	}

	return specs, nil
}

func (c *ServiceRegistryClient) GetServiceSpec(serviceName string) (*api.ServiceAPISpec, error) {
	resp, err := c.client.Get(c.registryURL + "/api/v1/services/" + serviceName)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var spec api.ServiceAPISpec
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		return nil, err
	}

	return &spec, nil
}
