// services/api-gateway/internal/router/ws_router.go
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/metrics"
	"github.com/gorilla/websocket"
)

type WebSocketRouter struct {
	config      *config.Config
	jwtManager  *auth.JWTManager
	upgrader    websocket.Upgrader
	connections sync.Map
	handlers    map[string]WebSocketHandler
}

type WebSocketHandler func(conn *websocket.Conn, msg *WSMessage) error

type WSMessage struct {
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
	TraceID string          `json:"trace_id,omitempty"`
}

type WSResponse struct {
	Type  string      `json:"type"`
	Data  interface{} `json:"data"`
	Error string      `json:"error,omitempty"`
}

func NewWebSocketRouter(cfg *config.Config, jwtManager *auth.JWTManager) *WebSocketRouter {
	return &WebSocketRouter{
		config:     cfg,
		jwtManager: jwtManager,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins in dev
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		handlers: make(map[string]WebSocketHandler),
	}
}

func (r *WebSocketRouter) Start(addr string) error {
	http.HandleFunc("/ws", r.handleWebSocket)

	// Register default handlers
	r.registerDefaultHandlers()

	logger.Info().Str("addr", addr).Msg("WebSocket server starting")
	return http.ListenAndServe(addr, nil)
}

func (r *WebSocketRouter) Stop(ctx context.Context) error {
	// Close all connections
	r.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*websocket.Conn); ok {
			conn.WriteMessage(websocket.CloseMessage, []byte{})
			conn.Close()
		}
		return true
	})

	logger.Info().Msg("WebSocket server stopped")
	return nil
}

func (r *WebSocketRouter) registerDefaultHandlers() {
	// Ping handler
	r.handlers["ping"] = func(conn *websocket.Conn, msg *WSMessage) error {
		return r.sendResponse(conn, "pong", map[string]interface{}{
			"timestamp": time.Now(),
		})
	}

	// Echo handler
	r.handlers["echo"] = func(conn *websocket.Conn, msg *WSMessage) error {
		return r.sendResponse(conn, "echo", msg.Data)
	}

	// Subscribe to service events
	r.handlers["subscribe"] = r.handleSubscribe
}

func (r *WebSocketRouter) handleWebSocket(w http.ResponseWriter, req *http.Request) {
	// Authenticate connection
	token := req.URL.Query().Get("token")
	if token == "" {
		token = req.Header.Get("Authorization")
		if token != "" {
			token = token[len("Bearer "):]
		}
	}

	if token == "" {
		http.Error(w, "Missing authentication token", http.StatusUnauthorized)
		return
	}

	claims, err := r.jwtManager.Verify(token)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Upgrade connection
	conn, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to upgrade to WebSocket")
		return
	}
	defer conn.Close()

	// Store connection
	connID := claims.UserID + ":" + claims.AppID
	r.connections.Store(connID, conn)
	metrics.ServiceActiveConnections.WithLabelValues("websocket", "connections").Inc()

	logger.Info().
		Str("user_id", claims.UserID).
		Str("app_id", claims.AppID).
		Msg("WebSocket connection established")

	// Handle messages
	for {
		var msg WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error().Err(err).Msg("WebSocket read error")
			}
			break
		}

		// Find handler
		handler, exists := r.handlers[msg.Type]
		if !exists {
			r.sendError(conn, msg.Type, "Unknown message type")
			continue
		}

		// Handle message
		if err := handler(conn, &msg); err != nil {
			logger.Error().Err(err).Str("type", msg.Type).Msg("Handler error")
			r.sendError(conn, msg.Type, err.Error())
		}
	}

	// Cleanup
	r.connections.Delete(connID)
	metrics.ServiceActiveConnections.WithLabelValues("websocket", "connections").Dec()
	logger.Info().Str("conn_id", connID).Msg("WebSocket connection closed")
}

func (r *WebSocketRouter) sendResponse(conn *websocket.Conn, msgType string, data interface{}) error {
	response := WSResponse{
		Type: msgType,
		Data: data,
	}
	return conn.WriteJSON(response)
}

func (r *WebSocketRouter) sendError(conn *websocket.Conn, msgType string, errMsg string) {
	response := WSResponse{
		Type:  msgType,
		Error: errMsg,
	}
	conn.WriteJSON(response)
}

func (r *WebSocketRouter) handleSubscribe(conn *websocket.Conn, msg *WSMessage) error {
	var subscription struct {
		Topics []string `json:"topics"`
	}

	if err := json.Unmarshal(msg.Data, &subscription); err != nil {
		return fmt.Errorf("invalid subscription data: %w", err)
	}

	// Store subscriptions for this connection
	// Implementation depends on your message broker

	return r.sendResponse(conn, "subscribed", map[string]interface{}{
		"topics": subscription.Topics,
	})
}

func (r *WebSocketRouter) Broadcast(msgType string, data interface{}) {
	response := WSResponse{
		Type: msgType,
		Data: data,
	}

	r.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*websocket.Conn); ok {
			conn.WriteJSON(response)
		}
		return true
	})
}

func (r *WebSocketRouter) SendToUser(userID, appID, msgType string, data interface{}) error {
	connID := userID + ":" + appID
	if value, ok := r.connections.Load(connID); ok {
		if conn, ok := value.(*websocket.Conn); ok {
			return r.sendResponse(conn, msgType, data)
		}
	}
	return fmt.Errorf("user %s not connected", userID)
}
