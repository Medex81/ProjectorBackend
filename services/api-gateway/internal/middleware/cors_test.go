// services/api-gateway/internal/middleware/cors_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCORSMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		method         string
		options        *CORSOptions
		expectedOrigin string
		expectedStatus int
	}{
		{
			name:   "Simple request with allowed origin",
			origin: "http://localhost:3000",
			method: "GET",
			options: &CORSOptions{
				AllowedOrigins: []string{"http://localhost:3000"},
			},
			expectedOrigin: "http://localhost:3000",
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Request with wildcard origin",
			origin: "http://anywhere.com",
			method: "GET",
			options: &CORSOptions{
				AllowedOrigins: []string{"*"},
			},
			expectedOrigin: "*",
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Request with disallowed origin",
			origin: "http://evil.com",
			method: "GET",
			options: &CORSOptions{
				AllowedOrigins: []string{"http://localhost:3000"},
			},
			expectedOrigin: "",
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Preflight request",
			origin: "http://localhost:3000",
			method: "OPTIONS",
			options: &CORSOptions{
				AllowedOrigins: []string{"http://localhost:3000"},
				AllowedMethods: []string{"GET", "POST"},
				AllowedHeaders: []string{"Content-Type"},
			},
			expectedOrigin: "http://localhost:3000",
			expectedStatus: http.StatusNoContent,
		},
		{
			name:   "Preflight with credentials",
			origin: "http://localhost:3000",
			method: "OPTIONS",
			options: &CORSOptions{
				AllowedOrigins:   []string{"http://localhost:3000"},
				AllowCredentials: true,
			},
			expectedOrigin: "http://localhost:3000",
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "No origin header",
			origin:         "",
			method:         "GET",
			options:        DefaultCORSOptions(),
			expectedOrigin: "",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Apply middleware
			handler := CORS(tt.options)(testHandler)

			// Create request
			req := httptest.NewRequest(tt.method, "http://example.com/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			// Record response
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Check status
			assert.Equal(t, tt.expectedStatus, rr.Code)

			// Check CORS headers
			if tt.expectedOrigin != "" {
				assert.Equal(t, tt.expectedOrigin, rr.Header().Get("Access-Control-Allow-Origin"))
			}

			if tt.method == "OPTIONS" {
				assert.NotEmpty(t, rr.Header().Get("Access-Control-Allow-Methods"))
				assert.NotEmpty(t, rr.Header().Get("Access-Control-Allow-Headers"))
			}

			if tt.options.AllowCredentials {
				assert.Equal(t, "true", rr.Header().Get("Access-Control-Allow-Credentials"))
			}
		})
	}
}

func TestIsOriginAllowed(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		allowedOrigins []string
		expected       bool
	}{
		{
			name:           "Exact match",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{"http://localhost:3000"},
			expected:       true,
		},
		{
			name:           "Wildcard",
			origin:         "http://any.com",
			allowedOrigins: []string{"*"},
			expected:       true,
		},
		{
			name:           "No match",
			origin:         "http://evil.com",
			allowedOrigins: []string{"http://localhost:3000"},
			expected:       false,
		},
		{
			name:           "Empty allowed origins",
			origin:         "http://localhost:3000",
			allowedOrigins: []string{},
			expected:       false,
		},
		{
			name:           "Multiple origins - match",
			origin:         "http://app.com",
			allowedOrigins: []string{"http://localhost:3000", "http://app.com"},
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOriginAllowed(tt.origin, tt.allowedOrigins)
			assert.Equal(t, tt.expected, result)
		})
	}
}
