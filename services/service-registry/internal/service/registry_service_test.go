// services/service-registry/internal/service/registry_service_test.go
package service

import (
	"context"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/models"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/repository"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRegistryService_Register(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockServiceRepository(ctrl)
	service := NewRegistryService(mockRepo, nil)

	ctx := context.Background()

	t.Run("Register new service", func(t *testing.T) {
		spec := &api.ServiceAPISpec{
			ServiceName: "test-service",
			Version:     "1.0.0",
		}

		mockRepo.EXPECT().
			GetByName(ctx, "test-service").
			Return(nil, nil)

		mockRepo.EXPECT().
			Create(ctx, gomock.Any()).
			Return(nil)

		err := service.Register(ctx, spec)
		assert.NoError(t, err)
	})

	t.Run("Register existing service", func(t *testing.T) {
		spec := &api.ServiceAPISpec{
			ServiceName: "existing",
			Version:     "1.0.0",
		}

		existingService := &models.Service{
			ID:   uuid.New(),
			Name: "existing",
		}

		mockRepo.EXPECT().
			GetByName(ctx, "existing").
			Return(existingService, nil)

		mockRepo.EXPECT().
			Update(ctx, gomock.Any()).
			Return(nil)

		err := service.Register(ctx, spec)
		assert.NoError(t, err)
	})

	t.Run("Register with empty name", func(t *testing.T) {
		spec := &api.ServiceAPISpec{
			ServiceName: "",
			Version:     "1.0.0",
		}

		err := service.Register(ctx, spec)
		assert.Error(t, err)
	})
}

func TestRegistryService_GetService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockServiceRepository(ctrl)
	service := NewRegistryService(mockRepo, nil)

	ctx := context.Background()

	t.Run("Get existing service", func(t *testing.T) {
		expected := &api.ServiceAPISpec{
			ServiceName: "test",
			Version:     "1.0.0",
		}

		mockRepo.EXPECT().
			GetByName(ctx, "test").
			Return(&models.Service{
				ID:      uuid.New(),
				Name:    "test",
				Version: "1.0.0",
				Spec:    expected,
			}, nil)

		result, err := service.GetService(ctx, "test")
		assert.NoError(t, err)
		assert.Equal(t, "test", result.ServiceName)
	})

	t.Run("Get non-existent service", func(t *testing.T) {
		mockRepo.EXPECT().
			GetByName(ctx, "unknown").
			Return(nil, nil)

		result, err := service.GetService(ctx, "unknown")
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("Repository error", func(t *testing.T) {
		mockRepo.EXPECT().
			GetByName(ctx, "test").
			Return(nil, assert.AnError)

		result, err := service.GetService(ctx, "test")
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestRegistryService_ListServices(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockServiceRepository(ctrl)
	service := NewRegistryService(mockRepo, nil)

	ctx := context.Background()

	t.Run("List all services", func(t *testing.T) {
		mockServices := []*models.Service{
			{
				ID:   uuid.New(),
				Name: "service1",
				Spec: &api.ServiceAPISpec{ServiceName: "service1"},
			},
			{
				ID:   uuid.New(),
				Name: "service2",
				Spec: &api.ServiceAPISpec{ServiceName: "service2"},
			},
		}

		mockRepo.EXPECT().
			List(ctx).
			Return(mockServices, nil)

		services, err := service.ListServices(ctx)
		assert.NoError(t, err)
		assert.Len(t, services, 2)
	})

	t.Run("Empty list", func(t *testing.T) {
		mockRepo.EXPECT().
			List(ctx).
			Return([]*models.Service{}, nil)

		services, err := service.ListServices(ctx)
		assert.NoError(t, err)
		assert.Empty(t, services)
	})

	t.Run("Repository error", func(t *testing.T) {
		mockRepo.EXPECT().
			List(ctx).
			Return(nil, assert.AnError)

		services, err := service.ListServices(ctx)
		assert.Error(t, err)
		assert.Nil(t, services)
	})
}

func TestRegistryService_Unregister(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockServiceRepository(ctrl)
	service := NewRegistryService(mockRepo, nil)

	ctx := context.Background()

	t.Run("Unregister existing service", func(t *testing.T) {
		existing := &models.Service{
			ID:   uuid.New(),
			Name: "test",
		}

		mockRepo.EXPECT().
			GetByName(ctx, "test").
			Return(existing, nil)

		mockRepo.EXPECT().
			Delete(ctx, existing.ID).
			Return(nil)

		err := service.Unregister(ctx, "test")
		assert.NoError(t, err)
	})

	t.Run("Unregister non-existent service", func(t *testing.T) {
		mockRepo.EXPECT().
			GetByName(ctx, "unknown").
			Return(nil, nil)

		err := service.Unregister(ctx, "unknown")
		assert.Error(t, err)
	})
}

func TestRegistryService_HealthCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockServiceRepository(ctrl)
	service := NewRegistryService(mockRepo, nil)

	ctx := context.Background()

	t.Run("Health check - healthy", func(t *testing.T) {
		mockRepo.EXPECT().
			FindUnhealthy(ctx, 30*time.Second).
			Return([]uuid.UUID{}, nil)

		healthy := service.HealthCheck(ctx)
		assert.True(t, healthy)
	})

	t.Run("Health check - unhealthy services found", func(t *testing.T) {
		unhealthyIDs := []uuid.UUID{uuid.New(), uuid.New()}

		mockRepo.EXPECT().
			FindUnhealthy(ctx, 30*time.Second).
			Return(unhealthyIDs, nil)

		// Mark each unhealthy service
		for _, id := range unhealthyIDs {
			mockRepo.EXPECT().
				Update(ctx, gomock.Any()).
				Return(nil)
		}

		healthy := service.HealthCheck(ctx)
		assert.True(t, healthy) // Still healthy, just marked services unhealthy
	})

	t.Run("Health check - repository error", func(t *testing.T) {
		mockRepo.EXPECT().
			FindUnhealthy(ctx, 30*time.Second).
			Return(nil, assert.AnError)

		healthy := service.HealthCheck(ctx)
		assert.False(t, healthy)
	})
}
