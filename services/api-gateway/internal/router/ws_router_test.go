// services/api-gateway/internal/router/ws_router_test.go
package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocketRouter_Upgrade(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)
	token, err := jwtManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	// Create test WebSocket server
	router := NewRouter(cfg, jwtManager)

	server := httptest.NewServer(router)
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	tests := []struct {
		name          string
		url           string
		token         string
		expectedError bool
		expectedClose bool
	}{
		{
			name:          "Valid WebSocket connection with token",
			url:           wsURL,
			token:         token,
			expectedError: false,
			expectedClose: false,
		},
		{
			name:          "WebSocket without token",
			url:           wsURL,
			token:         "",
			expectedError: true,
			expectedClose: true,
		},
		{
			name:          "Invalid token",
			url:           wsURL,
			token:         "invalid-token",
			expectedError: true,
			expectedClose: true,
		},
		{
			name:          "Invalid WebSocket path",
			url:           wsURL + "/invalid",
			token:         token,
			expectedError: true,
			expectedClose: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set headers
			header := http.Header{}
			if tt.token != "" {
				header.Set("Authorization", "Bearer "+tt.token)
			}

			// Connect to WebSocket
			conn, _, err := websocket.DefaultDialer.Dial(tt.url, header)

			if tt.expectedError {
				assert.Error(t, err)
				if conn != nil {
					conn.Close()
				}
				return
			}

			require.NoError(t, err)
			defer conn.Close()

			// Test message exchange
			err = conn.WriteMessage(websocket.TextMessage, []byte("ping"))
			assert.NoError(t, err)

			_, msg, err := conn.ReadMessage()
			assert.NoError(t, err)
			assert.Equal(t, "pong", string(msg))
		})
	}
}

func TestWebSocketRouter_MessageTypes(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)
	token, err := jwtManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	router := NewRouter(cfg, jwtManager)
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	require.NoError(t, err)
	defer conn.Close()

	tests := []struct {
		name         string
		messageType  int
		message      []byte
		expectedType int
		expectedMsg  []byte
	}{
		{
			name:         "Text message",
			messageType:  websocket.TextMessage,
			message:      []byte("ping"),
			expectedType: websocket.TextMessage,
			expectedMsg:  []byte("pong"),
		},
		{
			name:         "JSON message",
			messageType:  websocket.TextMessage,
			message:      []byte(`{"type":"ping","data":{}}`),
			expectedType: websocket.TextMessage,
			expectedMsg:  []byte(`{"type":"pong","data":{}}`),
		},
		{
			name:         "Binary message",
			messageType:  websocket.BinaryMessage,
			message:      []byte{0x01, 0x02, 0x03},
			expectedType: websocket.BinaryMessage,
			expectedMsg:  []byte{0x01, 0x02, 0x03, 0x04},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := conn.WriteMessage(tt.messageType, tt.message)
			require.NoError(t, err)

			msgType, msg, err := conn.ReadMessage()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedType, msgType)
			assert.Equal(t, tt.expectedMsg, msg)
		})
	}
}

func TestWebSocketRouter_ConcurrentConnections(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret: "test-secret",
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, 24*time.Hour)
	router := NewRouter(cfg, jwtManager)
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Create multiple concurrent connections
	connCount := 10
	conns := make([]*websocket.Conn, connCount)

	for i := 0; i < connCount; i++ {
		token, err := jwtManager.Generate("user-123", "test@example.com", "test-app")
		require.NoError(t, err)

		header := http.Header{}
		header.Set("Authorization", "Bearer "+token)

		conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		require.NoError(t, err)
		conns[i] = conn
	}

	// Test all connections work
	for i, conn := range conns {
		err := conn.WriteMessage(websocket.TextMessage, []byte("ping"))
		assert.NoError(t, err, "Connection %d should send message", i)

		_, msg, err := conn.ReadMessage()
		assert.NoError(t, err, "Connection %d should receive response", i)
		assert.Equal(t, "pong", string(msg), "Connection %d should get pong", i)
	}

	// Clean up
	for _, conn := range conns {
		conn.Close()
	}
}
