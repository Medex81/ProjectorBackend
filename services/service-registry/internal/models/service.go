// services/service-registry/internal/models/service.go
package models

import (
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/google/uuid"
)

type Service struct {
	ID            uuid.UUID           `json:"id" db:"id"`
	Name          string              `json:"name" db:"name"`
	Version       string              `json:"version" db:"version"`
	Spec          *api.ServiceAPISpec `json:"spec" db:"spec"`
	Status        ServiceStatus       `json:"status" db:"status"`
	Endpoints     []Endpoint          `json:"endpoints" db:"-"`
	Metadata      map[string]string   `json:"metadata,omitempty" db:"metadata"`
	CreatedAt     time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at" db:"updated_at"`
	LastHeartbeat time.Time           `json:"last_heartbeat" db:"last_heartbeat"`
}

type ServiceStatus string

const (
	StatusActive   ServiceStatus = "active"
	StatusInactive ServiceStatus = "inactive"
	StatusDegraded ServiceStatus = "degraded"
	StatusDown     ServiceStatus = "down"
)

type Endpoint struct {
	ID           string        `json:"id"`
	URL          string        `json:"url"`
	Protocol     string        `json:"protocol"` // http, grpc, ws
	Methods      []string      `json:"methods,omitempty"`
	Weight       int           `json:"weight,omitempty"`
	Healthy      bool          `json:"healthy"`
	LastCheck    time.Time     `json:"last_check"`
	ResponseTime time.Duration `json:"response_time,omitempty"`
}

type ServiceRegistration struct {
	ServiceName string            `json:"service_name"`
	Version     string            `json:"version"`
	Endpoints   []Endpoint        `json:"endpoints"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type ServiceDiscovery struct {
	Services  map[string][]*Endpoint `json:"services"`
	Timestamp time.Time              `json:"timestamp"`
}

type HealthStatus struct {
	ServiceID string    `json:"service_id"`
	Status    string    `json:"status"`
	LastCheck time.Time `json:"last_check"`
	Message   string    `json:"message,omitempty"`
}

type Metrics struct {
	TotalServices      int           `json:"total_services"`
	ActiveServices     int           `json:"active_services"`
	DegradedServices   int           `json:"degraded_services"`
	DownServices       int           `json:"down_services"`
	TotalEndpoints     int           `json:"total_endpoints"`
	Registrations      int64         `json:"registrations"`
	Deregistrations    int64         `json:"deregistrations"`
	HealthChecks       int64         `json:"health_checks"`
	FailedHealthChecks int64         `json:"failed_health_checks"`
	AvgResponseTime    time.Duration `json:"avg_response_time"`
}

func NewService(name, version string, spec *api.ServiceAPISpec) *Service {
	return &Service{
		ID:            uuid.New(),
		Name:          name,
		Version:       version,
		Spec:          spec,
		Status:        StatusActive,
		Endpoints:     []Endpoint{},
		Metadata:      make(map[string]string),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		LastHeartbeat: time.Now(),
	}
}

func (s *Service) IsHealthy() bool {
	return s.Status == StatusActive && time.Since(s.LastHeartbeat) < 30*time.Second
}

func (s *Service) UpdateHeartbeat() {
	s.LastHeartbeat = time.Now()
	if s.Status == StatusDown {
		s.Status = StatusActive
	}
}

func (s *Service) MarkDown(reason string) {
	s.Status = StatusDown
	s.Metadata["down_reason"] = reason
	s.Metadata["down_time"] = time.Now().String()
}

func (e *Endpoint) Validate() bool {
	return e.URL != "" && e.Protocol != ""
}
