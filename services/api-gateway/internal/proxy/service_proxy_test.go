// services/api-gateway/internal/proxy/service_proxy_test.go
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceProxy_RoundRobin(t *testing.T) {
	// Create test backend servers
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend1"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend2"))
	}))
	defer backend2.Close()

	backend3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend3"))
	}))
	defer backend3.Close()

	// Create service proxy with multiple endpoints
	proxy := NewServiceProxy(&api.ServiceAPISpec{
		ServiceName: "test-service",
		Spec: api.OpenAPI{
			Servers: []api.Server{
				{URL: backend1.URL},
				{URL: backend2.URL},
				{URL: backend3.URL},
			},
		},
	})

	// Test round-robin distribution
	expected := []string{"backend1", "backend2", "backend3", "backend1", "backend2", "backend3"}

	for i, expectedBody := range expected {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		proxy.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, expectedBody, rr.Body.String(), "Request %d should go to %s", i+1, expectedBody)
	}
}

func TestServiceProxy_ServiceUnavailable(t *testing.T) {
	// Create proxy with no available backends
	proxy := NewServiceProxy(&api.ServiceAPISpec{
		ServiceName: "test-service",
		Spec: api.OpenAPI{
			Servers: []api.Server{
				{URL: "http://localhost:9999"}, // Non-existent server
			},
		},
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	var response map[string]interface{}
	err := json.NewDecoder(rr.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "service_unavailable", response["error"])
	assert.Contains(t, response["message"], "No healthy endpoints available")
}

func TestServiceProxy_Timeout(t *testing.T) {
	// Create slow backend server
	slowBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Simulate slow response
		w.WriteHeader(http.StatusOK)
	}))
	defer slowBackend.Close()

	proxy := NewServiceProxy(&api.ServiceAPISpec{
		ServiceName: "test-service",
		Spec: api.OpenAPI{
			Servers: []api.Server{
				{URL: slowBackend.URL},
			},
		},
	})

	// Set timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusGatewayTimeout, rr.Code)
}

func TestServiceProxy_RetryOnFailure(t *testing.T) {
	failCount := 0
	// Create backend that fails first then succeeds
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failCount++
		if failCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer backend.Close()

	proxy := NewServiceProxy(&api.ServiceAPISpec{
		ServiceName: "test-service",
		Spec: api.OpenAPI{
			Servers: []api.Server{
				{URL: backend.URL},
			},
		},
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "success", rr.Body.String())
	assert.Equal(t, 3, failCount, "Should retry 3 times")
}

func TestServiceProxy_HealthCheck(t *testing.T) {
	// Create backend that reports unhealthy
	unhealthyBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer unhealthyBackend.Close()

	// Create healthy backend
	healthyBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	}))
	defer healthyBackend.Close()

	proxy := NewServiceProxy(&api.ServiceAPISpec{
		ServiceName: "test-service",
		Spec: api.OpenAPI{
			Servers: []api.Server{
				{URL: unhealthyBackend.URL},
				{URL: healthyBackend.URL},
			},
		},
	})

	// Run health checks
	proxy.CheckHealth()

	// Request should go to healthy backend
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "healthy", rr.Body.String())
}

func TestServiceProxy_RequestHeaders(t *testing.T) {
	// Create backend that echoes headers
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := map[string]string{
			"X-Forwarded-For":   r.Header.Get("X-Forwarded-For"),
			"X-Forwarded-Proto": r.Header.Get("X-Forwarded-Proto"),
			"X-Request-ID":      r.Header.Get("X-Request-ID"),
			"User-Agent":        r.Header.Get("User-Agent"),
		}
		json.NewEncoder(w).Encode(headers)
	}))
	defer backend.Close()

	proxy := NewServiceProxy(&api.ServiceAPISpec{
		ServiceName: "test-service",
		Spec: api.OpenAPI{
			Servers: []api.Server{
				{URL: backend.URL},
			},
		},
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Custom-Header", "custom-value")
	req.Header.Set("User-Agent", "test-agent")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)

	var headers map[string]string
	err := json.NewDecoder(rr.Body).Decode(&headers)
	require.NoError(t, err)

	assert.Equal(t, "192.168.1.1", headers["X-Forwarded-For"])
	assert.Equal(t, "http", headers["X-Forwarded-Proto"])
	assert.NotEmpty(t, headers["X-Request-ID"])
	assert.Equal(t, "test-agent", headers["User-Agent"])
}

func TestServiceProxy_PostRequestBody(t *testing.T) {
	// Create backend that echoes request body
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(body)
	}))
	defer backend.Close()

	proxy := NewServiceProxy(&api.ServiceAPISpec{
		ServiceName: "test-service",
		Spec: api.OpenAPI{
			Servers: []api.Server{
				{URL: backend.URL},
			},
		},
	})

	requestBody := []byte(`{"test": "data", "value": 123}`)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, string(requestBody), rr.Body.String())
}
