// services/api-gateway/internal/middleware/auth_test.go
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthMiddleware(t *testing.T) {
	// Setup
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:       "test-secret-key",
			TokenExpiration: 24 * time.Hour,
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)

	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get user from context
		claims := r.Context().Value("user").(*auth.Claims)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"user_id": claims.UserID,
			"email":   claims.Email,
			"app_id":  claims.AppID,
		})
	})

	// Generate valid token
	validToken, err := jwtManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	// Generate expired token
	expiredManager := auth.NewJWTManager(cfg.Auth.JWTSecret, -1*time.Hour)
	expiredToken, err := expiredManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	tests := []struct {
		name           string
		token          string
		expectedStatus int
		expectedBody   map[string]string
		setupContext   func(context.Context) context.Context
	}{
		{
			name:           "Valid token",
			token:          validToken,
			expectedStatus: http.StatusOK,
			expectedBody: map[string]string{
				"user_id": "user-123",
				"email":   "test@example.com",
				"app_id":  "test-app",
			},
		},
		{
			name:           "No token",
			token:          "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid token format",
			token:          "invalid.token.format",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Expired token",
			token:          expiredToken,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Token with wrong signing method",
			token:          "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.signature",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Malformed token",
			token:          "not-a-token-at-all",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Token missing claims",
			token:          "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.signature",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}

			// Apply middleware
			wrappedHandler := AuthMiddleware(jwtManager)(testHandler)

			// Record response
			rr := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rr, req)

			// Check status
			assert.Equal(t, tt.expectedStatus, rr.Code)

			// Check body for valid token
			if tt.expectedStatus == http.StatusOK {
				var body map[string]string
				err := json.NewDecoder(rr.Body).Decode(&body)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedBody, body)
			}
		})
	}
}

func TestAuthMiddleware_ExtractTokenFromHeader(t *testing.T) {
	tests := []struct {
		name          string
		header        string
		expectedToken string
		expectedError bool
	}{
		{
			name:          "Valid Bearer token",
			header:        "Bearer valid-token-123",
			expectedToken: "valid-token-123",
			expectedError: false,
		},
		{
			name:          "Lowercase bearer",
			header:        "bearer valid-token-123",
			expectedToken: "valid-token-123",
			expectedError: false,
		},
		{
			name:          "Extra spaces",
			header:        "Bearer   valid-token-123  ",
			expectedToken: "valid-token-123",
			expectedError: false,
		},
		{
			name:          "Missing Bearer prefix",
			header:        "valid-token-123",
			expectedToken: "",
			expectedError: true,
		},
		{
			name:          "Empty header",
			header:        "",
			expectedToken: "",
			expectedError: true,
		},
		{
			name:          "Wrong prefix",
			header:        "Basic dXNlcjpwYXNz",
			expectedToken: "",
			expectedError: true,
		},
		{
			name:          "Bearer without token",
			header:        "Bearer",
			expectedToken: "",
			expectedError: true,
		},
		{
			name:          "Bearer with empty token",
			header:        "Bearer ",
			expectedToken: "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", tt.header)

			token, err := extractTokenFromHeader(req)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedToken, token)
			}
		})
	}
}

func TestAuthMiddleware_ContextPropagation(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:       "test-secret-key",
			TokenExpiration: 24 * time.Hour,
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)
	token, err := jwtManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	// Test handler that extracts context
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value("user").(*auth.Claims)
		assert.True(t, ok, "User claims should be in context")
		assert.NotNil(t, claims)

		// Verify specific fields
		assert.Equal(t, "user-123", claims.UserID)
		assert.Equal(t, "test@example.com", claims.Email)
		assert.Equal(t, "test-app", claims.AppID)
		assert.NotEmpty(t, claims.ID) // JWT ID
		assert.NotEmpty(t, claims.IssuedAt)

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	wrappedHandler := AuthMiddleware(jwtManager)(testHandler)
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
