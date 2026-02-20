// services/service-registry/internal/api/handler_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/service"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_RegisterService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := service.NewMockRegistryService(ctrl)
	handler := NewHandler(mockService)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	tests := []struct {
		name           string
		requestBody    interface{}
		mockBehavior   func()
		expectedStatus int
	}{
		{
			name: "Register service successfully",
			requestBody: map[string]interface{}{
				"service_name": "test-service",
				"version":      "1.0.0",
				"spec": map[string]interface{}{
					"openapi": "3.0.0",
					"info": map[string]interface{}{
						"title": "Test API",
					},
				},
			},
			mockBehavior: func() {
				mockService.EXPECT().
					Register(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "Missing service name",
			requestBody: map[string]interface{}{
				"version": "1.0.0",
				"spec":    map[string]interface{}{},
			},
			mockBehavior:   func() {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON",
			requestBody:    "invalid json",
			mockBehavior:   func() {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Service already exists",
			requestBody: map[string]interface{}{
				"service_name": "existing",
				"version":      "1.0.0",
				"spec":         map[string]interface{}{},
			},
			mockBehavior: func() {
				mockService.EXPECT().
					Register(gomock.Any(), gomock.Any()).
					Return(assert.AnError)
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockBehavior()

			var body []byte
			var err error
			switch v := tt.requestBody.(type) {
			case string:
				body = []byte(v)
			default:
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest("POST", "/api/v1/services", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}

func TestHandler_GetService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := service.NewMockRegistryService(ctrl)
	handler := NewHandler(mockService)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("Get existing service", func(t *testing.T) {
		expectedSpec := &api.ServiceAPISpec{
			ServiceName: "test-service",
			Version:     "1.0.0",
		}

		mockService.EXPECT().
			GetService(gomock.Any(), "test-service").
			Return(expectedSpec, nil)

		req := httptest.NewRequest("GET", "/api/v1/services/test-service", nil)
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var spec api.ServiceAPISpec
		err := json.NewDecoder(rr.Body).Decode(&spec)
		require.NoError(t, err)
		assert.Equal(t, "test-service", spec.ServiceName)
	})

	t.Run("Get non-existent service", func(t *testing.T) {
		mockService.EXPECT().
			GetService(gomock.Any(), "unknown").
			Return(nil, assert.AnError)

		req := httptest.NewRequest("GET", "/api/v1/services/unknown", nil)
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

func TestHandler_ListServices(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := service.NewMockRegistryService(ctrl)
	handler := NewHandler(mockService)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("List all services", func(t *testing.T) {
		expectedServices := []*api.ServiceAPISpec{
			{
				ServiceName: "service1",
				Version:     "1.0.0",
			},
			{
				ServiceName: "service2",
				Version:     "1.0.0",
			},
		}

		mockService.EXPECT().
			ListServices(gomock.Any()).
			Return(expectedServices, nil)

		req := httptest.NewRequest("GET", "/api/v1/services", nil)
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var services []*api.ServiceAPISpec
		err := json.NewDecoder(rr.Body).Decode(&services)
		require.NoError(t, err)
		assert.Len(t, services, 2)
	})

	t.Run("Empty list", func(t *testing.T) {
		mockService.EXPECT().
			ListServices(gomock.Any()).
			Return([]*api.ServiceAPISpec{}, nil)

		req := httptest.NewRequest("GET", "/api/v1/services", nil)
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var services []*api.ServiceAPISpec
		err := json.NewDecoder(rr.Body).Decode(&services)
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}

func TestHandler_DeleteService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := service.NewMockRegistryService(ctrl)
	handler := NewHandler(mockService)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("Delete existing service", func(t *testing.T) {
		mockService.EXPECT().
			Unregister(gomock.Any(), "test-service").
			Return(nil)

		req := httptest.NewRequest("DELETE", "/api/v1/services/test-service", nil)
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNoContent, rr.Code)
	})

	t.Run("Delete non-existent service", func(t *testing.T) {
		mockService.EXPECT().
			Unregister(gomock.Any(), "unknown").
			Return(assert.AnError)

		req := httptest.NewRequest("DELETE", "/api/v1/services/unknown", nil)
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

func TestHandler_Health(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := service.NewMockRegistryService(ctrl)
	handler := NewHandler(mockService)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("Health check", func(t *testing.T) {
		mockService.EXPECT().
			HealthCheck(gomock.Any()).
			Return(true)

		req := httptest.NewRequest("GET", "/health", nil)
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var response map[string]interface{}
		err := json.NewDecoder(rr.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "healthy", response["status"])
	})
}
