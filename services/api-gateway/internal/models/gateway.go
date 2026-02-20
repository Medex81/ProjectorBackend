// services/api-gateway/internal/models/gateway.go
package models

import (
	"net/http"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/google/uuid"
)

type RouteConfig struct {
	ID          uuid.UUID      `json:"id"`
	Path        string         `json:"path"`
	Methods     []string       `json:"methods"`
	ServiceID   string         `json:"service_id"`
	ServiceName string         `json:"service_name"`
	Version     string         `json:"version"`
	Auth        bool           `json:"auth"`
	RateLimit   *RateLimitRule `json:"rate_limit,omitempty"`
	Timeout     time.Duration  `json:"timeout"`
	Retries     int            `json:"retries"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type RateLimitRule struct {
	Limit    int           `json:"limit"`
	Interval time.Duration `json:"interval"`
	PerIP    bool          `json:"per_ip"`
}

type ServiceEndpoint struct {
	ServiceName string              `json:"service_name"`
	ServiceID   string              `json:"service_id"`
	Version     string              `json:"version"`
	Addresses   []string            `json:"addresses"`
	Spec        *api.ServiceAPISpec `json:"spec"`
	Healthy     bool                `json:"healthy"`
	LastCheck   time.Time           `json:"last_check"`
	Metrics     *ServiceMetrics     `json:"metrics,omitempty"`
}

type ServiceMetrics struct {
	RequestCount int64         `json:"request_count"`
	ErrorCount   int64         `json:"error_count"`
	AvgLatency   time.Duration `json:"avg_latency"`
	LastRequest  time.Time     `json:"last_request"`
	StatusCodes  map[int]int64 `json:"status_codes"`
}

type WebSocketConnection struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	AppID       string    `json:"app_id"`
	ConnectedAt time.Time `json:"connected_at"`
	LastPing    time.Time `json:"last_ping"`
	RemoteAddr  string    `json:"remote_addr"`
	UserAgent   string    `json:"user_agent"`
}

type UDPClient struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	AppID       string    `json:"app_id"`
	Address     string    `json:"address"`
	ConnectedAt time.Time `json:"connected_at"`
	LastSeen    time.Time `json:"last_seen"`
	Sequence    uint32    `json:"sequence"`
}

type GatewayStats struct {
	StartTime          time.Time     `json:"start_time"`
	Uptime             time.Duration `json:"uptime"`
	TotalRequests      int64         `json:"total_requests"`
	TotalErrors        int64         `json:"total_errors"`
	ActiveConnections  int           `json:"active_connections"`
	ActiveWebSockets   int           `json:"active_websockets"`
	ActiveUDPClients   int           `json:"active_udp_clients"`
	RegisteredServices int           `json:"registered_services"`
	Routes             int           `json:"routes"`
	RequestsPerSecond  float64       `json:"requests_per_second"`
	AvgResponseTime    time.Duration `json:"avg_response_time"`
}

type ErrorResponse struct {
	Error     string    `json:"error"`
	Message   string    `json:"message,omitempty"`
	Code      int       `json:"code"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id,omitempty"`
}

func NewErrorResponse(err error, code int) *ErrorResponse {
	return &ErrorResponse{
		Error:     http.StatusText(code),
		Message:   err.Error(),
		Code:      code,
		Timestamp: time.Now(),
	}
}
