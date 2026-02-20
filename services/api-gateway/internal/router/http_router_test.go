// services/api-gateway/internal/router/http_router_test.go
package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter_OpenAPIEndpoint(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)

	// Create test router
	router := NewRouter(cfg, jwtManager)

	// Add test service specs
	testSpec := &api.ServiceAPISpec{
		ServiceName: "test-service",
		Version:     "1.0.0",
		Spec: api.OpenAPI{
			OpenAPI: "3.0.0",
			Info: api.Info{
				Title:   "Test API",
				Version: "1.0.0",
			},
			Paths: api.Paths{
				"/test": api.PathItem{
					Get: &api.Operation{
						Summary:     "Test endpoint",
						OperationID: "test",
					},
				},
			},
		},
	}

	router.UpdateOpenAPISpec(testSpec)

	// Test cases
	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		validateFunc   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "Get OpenAPI spec",
			method:         "GET",
			path:           "/api/v1/spec",
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var spec api.ServiceAPISpec
				err := json.NewDecoder(rr.Body).Decode(&spec)
				require.NoError(t, err)
				assert.Equal(t, "test-service", spec.ServiceName)
				assert.Equal(t, "3.0.0", spec.Spec.OpenAPI)
				assert.Contains(t, spec.Spec.Paths, "/test")
			},
		},
		{
			name:           "Get spec with ETag",
			method:         "GET",
			path:           "/api/v1/spec",
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, rr *httptest.ResponseRecorder) {
				assert.NotEmpty(t, rr.Header().Get("ETag"))
				assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
			},
		},
		{
			name:           "Get spec with If-None-Match",
			method:         "GET",
			path:           "/api/v1/spec",
			expectedStatus: http.StatusNotModified,
			setupFunc: func(req *http.Request) {
				req.Header.Set("If-None-Match", "some-etag")
			},
		},
		{
			name:           "Invalid path",
			method:         "GET",
			path:           "/api/v1/invalid",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.setupFunc != nil {
				tt.setupFunc(req)
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.validateFunc != nil {
				tt.validateFunc(t, rr)
			}
		})
	}
}

func TestRouter_Routing(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)
	router := NewRouter(cfg, jwtManager)

	// Create mock backend services
	authBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("auth-service"))
	}))
	defer authBackend.Close()

	gameBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("game-service"))
	}))
	defer gameBackend.Close()

	// Register services with router
	authSpec := &api.ServiceAPISpec{
		ServiceName: "auth",
		Spec: api.OpenAPI{
			Servers: []api.Server{
				{URL: authBackend.URL},
			},
			Paths: api.Paths{
				"/auth/login": api.PathItem{
					Post: &api.Operation{
						Tags: []string{"auth"},
					},
				},
			},
		},
	}

	gameSpec := &api.ServiceAPISpec{
		ServiceName: "game",
		Spec: api.OpenAPI{
			Servers: []api.Server{
				{URL: gameBackend.URL},
			},
			Paths: api.Paths{
				"/game/state": api.PathItem{
					Get: &api.Operation{
						Tags: []string{"game"},
					},
				},
			},
		},
	}

	router.UpdateOpenAPISpec(authSpec)
	router.UpdateOpenAPISpec(gameSpec)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Route to auth service",
			path:           "/auth/login",
			expectedStatus: http.StatusOK,
			expectedBody:   "auth-service",
		},
		{
			name:           "Route to game service",
			path:           "/game/state",
			expectedStatus: http.StatusOK,
			expectedBody:   "game-service",
		},
		{
			name:           "Unknown route",
			path:           "/unknown/path",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, rr.Body.String())
			}
		})
	}
}

func TestRouter_VersionedAPI(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)
	router := NewRouter(cfg, jwtManager)

	// Create mock backend for different versions
	v1Backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v1"))
	}))
	defer v1Backend.Close()

	v2Backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v2"))
	}))
	defer v2Backend.Close()

	// Register both versions
	v1Spec := &api.ServiceAPISpec{
		ServiceName: "test",
		Version:     "1.0.0",
		Spec: api.OpenAPI{
			Servers: []api.Server{{URL: v1Backend.URL}},
			Paths: api.Paths{
				"/test": api.PathItem{
					Get: &api.Operation{},
				},
			},
		},
	}

	v2Spec := &api.ServiceAPISpec{
		ServiceName: "test",
		Version:     "2.0.0",
		Spec: api.OpenAPI{
			Servers: []api.Server{{URL: v2Backend.URL}},
			Paths: api.Paths{
				"/test": api.PathItem{
					Get: &api.Operation{},
				},
			},
		},
	}

	router.UpdateOpenAPISpec(v1Spec)
	router.UpdateOpenAPISpec(v2Spec)

	tests := []struct {
		name         string
		path         string
		expectedBody string
	}{
		{
			name:         "Route to v1",
			path:         "/api/v1/test",
			expectedBody: "v1",
		},
		{
			name:         "Route to v2",
			path:         "/api/v2/test",
			expectedBody: "v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, tt.expectedBody, rr.Body.String())
		})
	}
}
