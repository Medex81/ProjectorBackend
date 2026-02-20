// services/service-registry/internal/api/grpc.go
package api

import (
	"context"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcServer struct {
	registryService *service.RegistryService
	UnimplementedServiceRegistryServer
}

func RegisterGRPCServer(s *grpc.Server, registryService *service.RegistryService) {
	RegisterServiceRegistryServer(s, &grpcServer{
		registryService: registryService,
	})
}

func (s *grpcServer) RegisterService(ctx context.Context, req *RegisterServiceRequest) (*RegisterServiceResponse, error) {
	spec := &api.ServiceAPISpec{
		ServiceName: req.ServiceName,
		Version:     req.Version,
		Spec:        api.OpenAPI{}, // Parse from req.Spec
	}

	err := s.registryService.Register(ctx, spec)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to register service: %v", err)
	}

	return &RegisterServiceResponse{
		ServiceId:  spec.ServiceName,
		Registered: true,
	}, nil
}

func (s *grpcServer) UnregisterService(ctx context.Context, req *UnregisterServiceRequest) (*UnregisterServiceResponse, error) {
	err := s.registryService.Unregister(ctx, req.ServiceId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unregister service: %v", err)
	}

	return &UnregisterServiceResponse{
		Unregistered: true,
	}, nil
}

func (s *grpcServer) GetService(ctx context.Context, req *GetServiceRequest) (*GetServiceResponse, error) {
	spec, err := s.registryService.GetService(ctx, req.ServiceName)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "service not found: %v", err)
	}

	// Convert spec to protobuf
	return &GetServiceResponse{
		ServiceName: spec.ServiceName,
		Version:     spec.Version,
		Spec:        []byte{}, // Marshal spec to JSON
	}, nil
}

func (s *grpcServer) ListServices(ctx context.Context, req *ListServicesRequest) (*ListServicesResponse, error) {
	services, err := s.registryService.ListServices(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list services: %v", err)
	}

	response := &ListServicesResponse{
		Services: make([]*ServiceInfo, 0, len(services)),
	}

	for _, svc := range services {
		response.Services = append(response.Services, &ServiceInfo{
			ServiceName: svc.ServiceName,
			Version:     svc.Version,
			Status:      "active",
		})
	}

	return response, nil
}

func (s *grpcServer) WatchServices(req *WatchServicesRequest, stream ServiceRegistry_WatchServicesServer) error {
	// Implement service watch
	return nil
}

func (s *grpcServer) HealthCheck(ctx context.Context, req *HealthCheckRequest) (*HealthCheckResponse, error) {
	healthy := s.registryService.HealthCheck(ctx)
	return &HealthCheckResponse{
		Status: map[string]bool{
			"service": healthy,
		},
	}, nil
}
