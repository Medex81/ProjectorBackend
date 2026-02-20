// services/api-gateway/test/mocks/mock_service_registry.go
package mocks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
)

type MockServiceRegistry struct {
	server   *httptest.Server
	mu       sync.RWMutex
	services map[string]*api.ServiceAPISpec
}

func NewMockServiceRegistry() *MockServiceRegistry {
	m := &MockServiceRegistry{
		services: make(map[string]*api.ServiceAPISpec),
	}

	m.server = httptest.NewServer(m)
	return m
}

func (m *MockServiceRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch r.Method {
	case http.MethodGet:
		if r.URL.Path == "/api/v1/services" {
			// Return all services
			services := make([]*api.ServiceAPISpec, 0, len(m.services))
			for _, spec := range m.services {
				services = append(services, spec)
			}
			json.NewEncoder(w).Encode(services)
		} else {
			// Return specific service
			serviceName := r.URL.Path[len("/api/v1/services/"):]
			if spec, ok := m.services[serviceName]; ok {
				json.NewEncoder(w).Encode(spec)
			} else {
				http.NotFound(w, r)
			}
		}

	case http.MethodPost:
		var spec api.ServiceAPISpec
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.services[spec.ServiceName] = &spec
		m.mu.Unlock()
		w.WriteHeader(http.StatusCreated)

	case http.MethodDelete:
		serviceName := r.URL.Path[len("/api/v1/services/"):]
		m.mu.Lock()
		delete(m.services, serviceName)
		m.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *MockServiceRegistry) URL() string {
	return m.server.URL
}

func (m *MockServiceRegistry) Close() {
	m.server.Close()
}

func (m *MockServiceRegistry) AddService(spec *api.ServiceAPISpec) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.services[spec.ServiceName] = spec
}

func (m *MockServiceRegistry) RemoveService(serviceName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.services, serviceName)
}

func (m *MockServiceRegistry) GetService(serviceName string) *api.ServiceAPISpec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.services[serviceName]
}

func (m *MockServiceRegistry) GetAllServices() []*api.ServiceAPISpec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	services := make([]*api.ServiceAPISpec, 0, len(m.services))
	for _, spec := range m.services {
		services = append(services, spec)
	}
	return services
}

// Predefined mock services
func NewMockAuthService() *api.ServiceAPISpec {
	return &api.ServiceAPISpec{
		ServiceName: "auth",
		Version:     "1.0.0",
		Spec: api.OpenAPI{
			OpenAPI: "3.0.0",
			Info: api.Info{
				Title:   "Auth Service",
				Version: "1.0.0",
			},
			Paths: api.Paths{
				"/auth/register": api.PathItem{
					Post: &api.Operation{},
				},
				"/auth/verify": api.PathItem{
					Post: &api.Operation{},
				},
				"/auth/login": api.PathItem{
					Post: &api.Operation{},
				},
			},
		},
	}
}

func NewMockGameService() *api.ServiceAPISpec {
	return &api.ServiceAPISpec{
		ServiceName: "game",
		Version:     "1.0.0",
		Spec: api.OpenAPI{
			OpenAPI: "3.0.0",
			Info: api.Info{
				Title:   "Game Service",
				Version: "1.0.0",
			},
			Paths: api.Paths{
				"/game/state": api.PathItem{
					Get: &api.Operation{},
				},
				"/game/move": api.PathItem{
					Post: &api.Operation{},
				},
			},
		},
	}
}
