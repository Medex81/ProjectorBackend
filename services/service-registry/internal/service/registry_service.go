// services/service-registry/internal/service/registry_service.go
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/Medex81/ProjectorBackend/pkg/common/kafka"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/models"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/repository"
	"github.com/google/uuid"
)

type RegistryService struct {
	repo                *repository.ServiceRepository
	kafkaProducer       *kafka.Producer
	mu                  sync.RWMutex
	healthCheckInterval time.Duration
}

func NewRegistryService(repo *repository.ServiceRepository, kafkaProducer *kafka.Producer) *RegistryService {
	s := &RegistryService{
		repo:                repo,
		kafkaProducer:       kafkaProducer,
		healthCheckInterval: 30 * time.Second,
	}

	// Start health checker
	go s.startHealthChecker()

	return s
}

func (s *RegistryService) Register(ctx context.Context, spec *api.ServiceAPISpec) error {
	if spec.ServiceName == "" {
		return fmt.Errorf("service name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if service already exists
	existing, err := s.repo.GetByName(ctx, spec.ServiceName)
	if err != nil {
		return err
	}

	if existing != nil {
		// Update existing service
		existing.Spec = spec
		existing.Version = spec.Version
		existing.UpdateHeartbeat()
		existing.Status = models.StatusActive

		if err := s.repo.Update(ctx, existing); err != nil {
			return err
		}

		logger.Info().Str("service", spec.ServiceName).Msg("Service updated")
		return nil
	}

	// Create new service
	service := models.NewService(spec.ServiceName, spec.Version, spec)

	if err := s.repo.Create(ctx, service); err != nil {
		return err
	}

	// Publish service registered event
	s.publishServiceEvent("service_registered", service)

	logger.Info().Str("service", spec.ServiceName).Msg("Service registered")
	return nil
}

func (s *RegistryService) Unregister(ctx context.Context, serviceName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	service, err := s.repo.GetByName(ctx, serviceName)
	if err != nil {
		return err
	}

	if service == nil {
		return fmt.Errorf("service %s not found", serviceName)
	}

	if err := s.repo.Delete(ctx, service.ID); err != nil {
		return err
	}

	// Publish service unregistered event
	s.publishServiceEvent("service_unregistered", service)

	logger.Info().Str("service", serviceName).Msg("Service unregistered")
	return nil
}

func (s *RegistryService) GetService(ctx context.Context, serviceName string) (*api.ServiceAPISpec, error) {
	service, err := s.repo.GetByName(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	if service == nil {
		return nil, nil
	}

	return service.Spec, nil
}

func (s *RegistryService) ListServices(ctx context.Context) ([]*api.ServiceAPISpec, error) {
	services, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	specs := make([]*api.ServiceAPISpec, 0, len(services))
	for _, svc := range services {
		specs = append(specs, svc.Spec)
	}

	return specs, nil
}

func (s *RegistryService) Update(ctx context.Context, serviceName string, spec *api.ServiceAPISpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	service, err := s.repo.GetByName(ctx, serviceName)
	if err != nil {
		return err
	}

	if service == nil {
		return fmt.Errorf("service %s not found", serviceName)
	}

	service.Spec = spec
	service.Version = spec.Version
	service.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, service); err != nil {
		return err
	}

	// Publish service updated event
	s.publishServiceEvent("service_updated", service)

	logger.Info().Str("service", serviceName).Msg("Service updated")
	return nil
}

func (s *RegistryService) Heartbeat(ctx context.Context, serviceName string) error {
	service, err := s.repo.GetByName(ctx, serviceName)
	if err != nil {
		return err
	}

	if service == nil {
		return fmt.Errorf("service %s not found", serviceName)
	}

	return s.repo.UpdateHeartbeat(ctx, service.ID)
}

func (s *RegistryService) HealthCheck(ctx context.Context) bool {
	// Check for services that haven't sent heartbeat
	unhealthy, err := s.repo.FindUnhealthy(ctx, 30*time.Second)
	if err != nil {
		logger.Error().Err(err).Msg("Health check failed")
		return false
	}

	// Mark unhealthy services
	for _, id := range unhealthy {
		service, err := s.getServiceByID(ctx, id)
		if err != nil || service == nil {
			continue
		}

		service.Status = models.StatusDown
		s.repo.Update(ctx, service)

		logger.Warn().Str("service", service.Name).Msg("Service marked unhealthy")
	}

	return true
}

func (s *RegistryService) GetMetrics() map[string]interface{} {
	ctx := context.Background()
	services, _ := s.repo.List(ctx)

	active := 0
	degraded := 0
	down := 0

	for _, svc := range services {
		switch svc.Status {
		case models.StatusActive:
			active++
		case models.StatusDegraded:
			degraded++
		case models.StatusDown:
			down++
		}
	}

	return map[string]interface{}{
		"total_services":    len(services),
		"active_services":   active,
		"degraded_services": degraded,
		"down_services":     down,
		"timestamp":         time.Now(),
	}
}

func (s *RegistryService) startHealthChecker() {
	ticker := time.NewTicker(s.healthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		s.HealthCheck(ctx)
	}
}

func (s *RegistryService) publishServiceEvent(eventType string, service *models.Service) {
	if s.kafkaProducer == nil {
		return
	}

	// Publish to Kafka for other services
	// Implementation depends on your message format
}

func (s *RegistryService) getServiceByID(ctx context.Context, id uuid.UUID) (*models.Service, error) {
	// Helper method to get service by ID
	// This would need to be implemented in the repository
	return nil, nil
}
