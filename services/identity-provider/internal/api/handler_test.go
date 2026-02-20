// services/identity-provider/internal/api/handler_test.go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/models"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/service"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_Register(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := service.NewMockAuthService(ctrl)
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name: "test-service",
		},
	}

	handler := NewHandler(mockService, cfg)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	tests := []struct {
		name           string
		requestBody    interface{}
		mockBehavior   func()
		expectedStatus int
		expectedBody   map[string]string
	}{
		{
			name: "Successful registration",
			requestBody: map[string]interface{}{
				"email":    "test@example.com",
				"password": "password123",
				"app_id":   "test-app",
			},
			mockBehavior: func() {
				mockService.EXPECT().
					Register(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedStatus: http.StatusAccepted,
			expectedBody: map[string]string{
				"status":  "accepted",
				"message": "Verification code sent to email",
			},
		},
		{
			name: "Missing email",
			requestBody: map[string]interface{}{
				"password": "password123",
				"app_id":   "test-app",
			},
			mockBehavior:   func() {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing password",
			requestBody: map[string]interface{}{
				"email":  "test@example.com",
				"app_id": "test-app",
			},
			mockBehavior:   func() {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "User already exists",
			requestBody: map[string]interface{}{
				"email":    "existing@example.com",
				"password": "password123",
				"app_id":   "test-app",
			},
			mockBehavior: func() {
				mockService.EXPECT().
					Register(gomock.Any(), gomock.Any()).
					Return(assert.AnError)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON",
			requestBody:    "invalid json",
			mockBehavior:   func() {},
			expectedStatus: http.StatusBadRequest,
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

			req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedBody != nil {
				var response map[string]string
				err := json.NewDecoder(rr.Body).Decode(&response)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedBody, response)
			}
		})
	}
}

func TestHandler_Verify(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := service.NewMockAuthService(ctrl)
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name: "test-service",
		},
	}

	handler := NewHandler(mockService, cfg)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	tests := []struct {
		name           string
		requestBody    interface{}
		mockBehavior   func()
		expectedStatus int
		expectedToken  bool
	}{
		{
			name: "Successful verification",
			requestBody: map[string]interface{}{
				"email":  "test@example.com",
				"code":   "123456",
				"app_id": "test-app",
			},
			mockBehavior: func() {
				mockService.EXPECT().
					Verify(gomock.Any(), gomock.Any()).
					Return("valid-token", nil)
			},
			expectedStatus: http.StatusOK,
			expectedToken:  true,
		},
		{
			name: "Invalid code",
			requestBody: map[string]interface{}{
				"email":  "test@example.com",
				"code":   "wrong",
				"app_id": "test-app",
			},
			mockBehavior: func() {
				mockService.EXPECT().
					Verify(gomock.Any(), gomock.Any()).
					Return("", assert.AnError)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedToken:  false,
		},
		{
			name: "Missing email",
			requestBody: map[string]interface{}{
				"code":   "123456",
				"app_id": "test-app",
			},
			mockBehavior:   func() {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing code",
			requestBody: map[string]interface{}{
				"email":  "test@example.com",
				"app_id": "test-app",
			},
			mockBehavior:   func() {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockBehavior()

			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/api/v1/auth/verify", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedToken {
				var response models.AuthResponse
				err := json.NewDecoder(rr.Body).Decode(&response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.Token)
			}
		})
	}
}

func TestHandler_Login(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := service.NewMockAuthService(ctrl)
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name: "test-service",
		},
	}

	handler := NewHandler(mockService, cfg)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	tests := []struct {
		name           string
		requestBody    interface{}
		mockBehavior   func()
		expectedStatus int
		expectedToken  bool
	}{
		{
			name: "Successful login",
			requestBody: map[string]interface{}{
				"email":    "test@example.com",
				"password": "password123",
				"app_id":   "test-app",
			},
			mockBehavior: func() {
				mockService.EXPECT().
					Login(gomock.Any(), gomock.Any()).
					Return("valid-token", nil)
			},
			expectedStatus: http.StatusOK,
			expectedToken:  true,
		},
		{
			name: "Invalid credentials",
			requestBody: map[string]interface{}{
				"email":    "test@example.com",
				"password": "wrong",
				"app_id":   "test-app",
			},
			mockBehavior: func() {
				mockService.EXPECT().
					Login(gomock.Any(), gomock.Any()).
					Return("", assert.AnError)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedToken:  false,
		},
		{
			name: "Missing email",
			requestBody: map[string]interface{}{
				"password": "password123",
				"app_id":   "test-app",
			},
			mockBehavior:   func() {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockBehavior()

			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedToken {
				var response models.AuthResponse
				err := json.NewDecoder(rr.Body).Decode(&response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.Token)
			}
		})
	}
}

func TestHandler_Health(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := service.NewMockAuthService(ctrl)
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name: "test-service",
		},
	}

	handler := NewHandler(mockService, cfg)
	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response map[string]string
	err := json.NewDecoder(rr.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, "test-service", response["service"])
}
