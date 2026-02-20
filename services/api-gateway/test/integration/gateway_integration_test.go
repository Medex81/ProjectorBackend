// services/api-gateway/test/integration/gateway_integration_test.go
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/services/api-gateway/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGateway_EndToEnd(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:       "test-secret",
			TokenExpiration: 24 * time.Hour,
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)

	// Create mock services
	authService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/register":
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			// Validate request
			if req["email"] == nil || req["password"] == nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "accepted",
				"message": "Verification code sent",
			})

		case "/auth/verify":
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			if req["code"] == "123456" {
				token, _ := jwtManager.Generate("user-123", req["email"].(string), req["app_id"].(string))
				json.NewEncoder(w).Encode(map[string]string{
					"token": token,
				})
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer authService.Close()

	gameService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify JWT token from header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"state": "running",
			"score": "100",
		})
	}))
	defer gameService.Close()

	// Create gateway with service specs
	gateway := router.NewRouter(cfg, jwtManager)

	authSpec := &api.ServiceAPISpec{
		ServiceName: "auth",
		Spec: api.OpenAPI{
			Servers: []api.Server{{URL: authService.URL}},
			Paths: api.Paths{
				"/auth/register": api.PathItem{
					Post: &api.Operation{},
				},
				"/auth/verify": api.PathItem{
					Post: &api.Operation{},
				},
			},
		},
	}

	gameSpec := &api.ServiceAPISpec{
		ServiceName: "game",
		Spec: api.OpenAPI{
			Servers: []api.Server{{URL: gameService.URL}},
			Paths: api.Paths{
				"/game/state": api.PathItem{
					Get: &api.Operation{
						Security: []map[string][]string{
							{"bearerAuth": {}},
						},
					},
				},
			},
		},
	}

	gateway.UpdateOpenAPISpec(authSpec)
	gateway.UpdateOpenAPISpec(gameSpec)

	// Test cases
	t.Run("Complete auth flow", func(t *testing.T) {
		// 1. Register
		registerBody := map[string]interface{}{
			"email":    "test@example.com",
			"password": "password123",
			"app_id":   "test-app",
		}
		registerJSON, _ := json.Marshal(registerBody)

		req := httptest.NewRequest("POST", "/auth/register", bytes.NewReader(registerJSON))
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		gateway.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusAccepted, rr.Code)

		// 2. Verify with correct code
		verifyBody := map[string]interface{}{
			"email":  "test@example.com",
			"code":   "123456",
			"app_id": "test-app",
		}
		verifyJSON, _ := json.Marshal(verifyBody)

		req = httptest.NewRequest("POST", "/auth/verify", bytes.NewReader(verifyJSON))
		req.Header.Set("Content-Type", "application/json")

		rr = httptest.NewRecorder()
		gateway.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var response map[string]string
		err := json.NewDecoder(rr.Body).Decode(&response)
		require.NoError(t, err)

		token := response["token"]
		assert.NotEmpty(t, token)

		// 3. Access protected game endpoint with token
		req = httptest.NewRequest("GET", "/game/state", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		rr = httptest.NewRecorder()
		gateway.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var gameState map[string]string
		err = json.NewDecoder(rr.Body).Decode(&gameState)
		require.NoError(t, err)

		assert.Equal(t, "running", gameState["state"])
	})

	t.Run("Invalid verification code", func(t *testing.T) {
		verifyBody := map[string]interface{}{
			"email":  "test@example.com",
			"code":   "wrong-code",
			"app_id": "test-app",
		}
		verifyJSON, _ := json.Marshal(verifyBody)

		req := httptest.NewRequest("POST", "/auth/verify", bytes.NewReader(verifyJSON))
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		gateway.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Access protected endpoint without token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/game/state", nil)

		rr := httptest.NewRecorder()
		gateway.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Access non-existent endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/non-existent", nil)

		rr := httptest.NewRecorder()
		gateway.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

func TestGateway_SpecEndpoint(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)
	gateway := router.NewRouter(cfg, jwtManager)

	// Add test specs
	testSpec := &api.ServiceAPISpec{
		ServiceName: "test",
		Spec: api.OpenAPI{
			Info: api.Info{
				Title:   "Test API",
				Version: "1.0.0",
			},
		},
	}
	gateway.UpdateOpenAPISpec(testSpec)

	t.Run("Get API spec", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/spec", nil)

		rr := httptest.NewRecorder()
		gateway.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
		assert.NotEmpty(t, rr.Header().Get("ETag"))

		var spec api.ServiceAPISpec
		err := json.NewDecoder(rr.Body).Decode(&spec)
		require.NoError(t, err)

		assert.Equal(t, "Test API", spec.Info.Title)
	})

	t.Run("Get spec with ETag caching", func(t *testing.T) {
		// First request to get ETag
		req1 := httptest.NewRequest("GET", "/api/v1/spec", nil)
		rr1 := httptest.NewRecorder()
		gateway.ServeHTTP(rr1, req1)

		etag := rr1.Header().Get("ETag")

		// Second request with If-None-Match
		req2 := httptest.NewRequest("GET", "/api/v1/spec", nil)
		req2.Header.Set("If-None-Match", etag)

		rr2 := httptest.NewRecorder()
		gateway.ServeHTTP(rr2, req2)

		assert.Equal(t, http.StatusNotModified, rr2.Code)
	})
}
