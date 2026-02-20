// services/identity-provider/internal/api/handler.go
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/models"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/service"
	"github.com/gorilla/mux"
)

type Handler struct {
	authService *service.AuthService
	config      *config.Config
}

func NewHandler(authService *service.AuthService, cfg *config.Config) *Handler {
	return &Handler{
		authService: authService,
		config:      cfg,
	}
}

func (h *Handler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/api/v1/auth/register", h.Register).Methods("POST")
	router.HandleFunc("/api/v1/auth/verify", h.Verify).Methods("POST")
	router.HandleFunc("/api/v1/auth/login", h.Login).Methods("POST")
	router.HandleFunc("/health", h.Health).Methods("GET")
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "handler.Register")
	defer span.End()

	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.authService.Register(ctx, &req); err != nil {
		logger.Error().Err(err).Msg("Registration failed")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"message": "Verification code sent to email",
	})
}

func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "handler.Verify")
	defer span.End()

	var req models.VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	token, err := h.authService.Verify(ctx, &req)
	if err != nil {
		logger.Error().Err(err).Msg("Verification failed")
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	json.NewEncoder(w).Encode(models.AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(h.config.Auth.TokenExpiration),
		Email:     req.Email,
		AppID:     req.AppID,
		Verified:  true,
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "handler.Login")
	defer span.End()

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	token, err := h.authService.Login(ctx, &req)
	if err != nil {
		logger.Error().Err(err).Msg("Login failed")
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	json.NewEncoder(w).Encode(models.AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(h.config.Auth.TokenExpiration),
		Email:     req.Email,
		AppID:     req.AppID,
	})
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": h.config.Service.Name,
	})
}
