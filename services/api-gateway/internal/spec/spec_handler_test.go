// services/api-gateway/internal/spec/spec_handler_test.go
package spec

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecHandler_GetSpec(t *testing.T) {
	handler := NewSpecHandler()

	// Add test specs
	spec1 := &api.ServiceAPISpec{
		ServiceName: "auth",
		Version:     "1.0.0",
		Spec: api.OpenAPI{
			Paths: api.Paths{
				"/auth/login": api.PathItem{
					Post: &api.Operation{},
				},
			},
		},
	}

	spec2 := &api.ServiceAPISpec{
		ServiceName: "game",
		Version:     "1.0.0",
		Spec: api.OpenAPI{
			Paths: api.Paths{
				"/game/state": api.PathItem{
					Get: &api.Operation{},
				},
			},
		},
	}

	handler.UpdateSpec(spec1)
	handler.UpdateSpec(spec2)

	tests := []struct {
		name           string
		headers        map[string]string
		expectedStatus int
		expectedETag   bool
		validateFunc   func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "Get full spec",
			headers:        nil,
			expectedStatus: http.StatusOK,
			expectedETag:   true,
			validateFunc: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var spec api.ServiceAPISpec
				err := json.NewDecoder(rr.Body).Decode(&spec)
				require.NoError(t, err)

				assert.Equal(t, "ProjectorBackend API", spec.Info.Title)
				assert.Contains(t, spec.Paths, "/auth/login")
				assert.Contains(t, spec.Paths, "/game/state")
			},
		},
		{
			name: "With If-None-Match - same ETag",
			headers: map[string]string{
				"If-None-Match": handler.GetETag(),
			},
			expectedStatus: http.StatusNotModified,
			expectedETag:   false,
		},
		{
			name: "With If-None-Match - different ETag",
			headers: map[string]string{
				"If-None-Match": "wrong-etag",
			},
			expectedStatus: http.StatusOK,
			expectedETag:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/spec", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedETag {
				assert.NotEmpty(t, rr.Header().Get("ETag"))
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, rr)
			}
		})
	}
}

func TestSpecHandler_ETagGeneration(t *testing.T) {
	handler := NewSpecHandler()

	// Empty spec should generate ETag
	etag1 := handler.GetETag()
	assert.NotEmpty(t, etag1)

	// Add spec should change ETag
	spec := &api.ServiceAPISpec{
		ServiceName: "test",
		Spec: api.OpenAPI{
			Paths: api.Paths{
				"/test": api.PathItem{},
			},
		},
	}
	handler.UpdateSpec(spec)

	etag2 := handler.GetETag()
	assert.NotEqual(t, etag1, etag2)

	// Same spec should generate same ETag
	handler.UpdateSpec(spec)
	etag3 := handler.GetETag()
	assert.Equal(t, etag2, etag3)

	// Verify ETag is valid MD5
	_, err := hex.DecodeString(etag2)
	assert.NoError(t, err)
	assert.Equal(t, 32, len(etag2))
}

func TestSpecHandler_ConcurrentUpdates(t *testing.T) {
	handler := NewSpecHandler()

	// Concurrent updates from multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				spec := &api.ServiceAPISpec{
					ServiceName: "test",
					Spec: api.OpenAPI{
						Paths: api.Paths{
							"/test": api.PathItem{},
						},
					},
				}
				handler.UpdateSpec(spec)
				_ = handler.GetSpec()
				_ = handler.GetETag()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Final spec should be valid
	finalSpec := handler.GetSpec()
	assert.NotNil(t, finalSpec)
}
