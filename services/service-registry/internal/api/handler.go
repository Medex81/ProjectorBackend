// services/service-registry/internal/api/handler.go
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/service"
	"github.com/gorilla/mux"
)

type Handler struct {
	registryService *service.RegistryService
}

func NewHandler(registryService *service.RegistryService) *Handler {
	return &Handler{
		registryService: registryService,
	}
}

func (h *Handler) RegisterRoutes(router *mux.Router) {
	// Service management
	router.HandleFunc("/api/v1/services", h.ListServices).Methods("GET")
	router.HandleFunc("/api/v1/services", h.RegisterService).Methods("POST")
	router.HandleFunc("/api/v1/services/{name}", h.GetService).Methods("GET")
	router.HandleFunc("/api/v1/services/{name}", h.UpdateService).Methods("PUT")
	router.HandleFunc("/api/v1/services/{name}", h.DeleteService).Methods("DELETE")

	// Health check
	router.HandleFunc("/health", h.Health).Methods("GET")

	// Metrics
	router.HandleFunc("/metrics", h.Metrics).Methods("GET")
}

func (h *Handler) RegisterService(w http.ResponseWriter, r *http.Request) {
	var spec api.ServiceAPISpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		logger.Error().Err(err).Msg("Failed to decode service spec")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if spec.ServiceName == "" {
		http.Error(w, "Service name is required", http.StatusBadRequest)
		return
	}

	if err := h.registryService.Register(r.Context(), &spec); err != nil {
		logger.Error().Err(err).Str("service", spec.ServiceName).Msg("Failed to register service")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logger.Info().Str("service", spec.ServiceName).Msg("Service registered successfully")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "registered",
		"service": spec.ServiceName,
	})
}

func (h *Handler) GetService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	spec, err := h.registryService.GetService(r.Context(), serviceName)
	if err != nil {
		logger.Error().Err(err).Str("service", serviceName).Msg("Service not found")
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(spec)
}

func (h *Handler) ListServices(w http.ResponseWriter, r *http.Request) {
	services, err := h.registryService.ListServices(r.Context())
	if err != nil {
		logger.Error().Err(err).Msg("Failed to list services")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

func (h *Handler) UpdateService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	var spec api.ServiceAPISpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		logger.Error().Err(err).Msg("Failed to decode service spec")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.registryService.Update(r.Context(), serviceName, &spec); err != nil {
		logger.Error().Err(err).Str("service", serviceName).Msg("Failed to update service")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "updated",
		"service": serviceName,
	})
}

func (h *Handler) DeleteService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	if err := h.registryService.Unregister(r.Context(), serviceName); err != nil {
		logger.Error().Err(err).Str("service", serviceName).Msg("Failed to delete service")
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	logger.Info().Str("service", serviceName).Msg("Service unregistered")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	healthy := h.registryService.HealthCheck(r.Context())

	status := "healthy"
	code := http.StatusOK

	if !healthy {
		status = "unhealthy"
		code = http.StatusServiceUnavailable
	}

	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  status,
		"healthy": healthy,
		"time":    time.Now(),
	})
}

func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	metrics := h.registryService.GetMetrics()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}
