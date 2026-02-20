// services/api-gateway/cmd/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/database"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"github.com/Medex81/ProjectorBackend/services/api-gateway/internal/proxy"
	"github.com/Medex81/ProjectorBackend/services/api-gateway/internal/router"
	"github.com/Medex81/ProjectorBackend/services/api-gateway/internal/spec"
)

func main() {
	// Load configuration
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "configs/config.dev.yaml"
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Initialize logger
	logger.Init(cfg.Service.Name, cfg.Service.Environment)

	// Initialize tracer
	tp, err := tracing.InitTracer(cfg.Service.Name, cfg.Jaeger.AgentHost, cfg.Jaeger.AgentPort)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize tracer")
	}
	defer tp.Shutdown(context.Background())

	// Connect to Redis for rate limiting and session storage
	redisClient, err := database.NewRedisConnection(&cfg.Redis)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer redisClient.Close()

	// Initialize JWT manager
	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)

	// Initialize spec handler for OpenAPI
	specHandler := spec.NewSpecHandler()

	// Initialize service registry client
	registryClient := proxy.NewServiceRegistryClient(cfg)

	// Start background spec updater
	go startSpecUpdater(specHandler, registryClient, cfg)

	// Initialize routers
	httpRouter := router.NewHTTPRouter(cfg, jwtManager, specHandler)
	wsRouter := router.NewWebSocketRouter(cfg, jwtManager)
	grpcServer := router.NewGRPCServer(cfg, jwtManager)
	udpServer := router.NewUDPServer(cfg, jwtManager)

	// Start HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Gateway.HTTPPort),
		Handler:      httpRouter,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info().Int("port", cfg.Gateway.HTTPPort).Msg("Starting HTTP server")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Failed to start HTTP server")
		}
	}()

	// Start WebSocket server
	go func() {
		wsAddr := fmt.Sprintf(":%d", cfg.Gateway.WSPort)
		logger.Info().Int("port", cfg.Gateway.WSPort).Msg("Starting WebSocket server")
		if err := wsRouter.Start(wsAddr); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Failed to start WebSocket server")
		}
	}()

	// Start gRPC server
	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Gateway.GRPCPort))
		if err != nil {
			logger.Fatal().Err(err).Msg("Failed to listen for gRPC")
		}
		logger.Info().Int("port", cfg.Gateway.GRPCPort).Msg("Starting gRPC server")
		if err := grpcServer.Serve(lis); err != nil {
			logger.Fatal().Err(err).Msg("Failed to start gRPC server")
		}
	}()

	// Start UDP server
	go func() {
		logger.Info().Int("port", cfg.Gateway.UDPPort).Msg("Starting UDP server")
		if err := udpServer.Start(); err != nil {
			logger.Fatal().Err(err).Msg("Failed to start UDP server")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("Shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Service.ShutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("HTTP server shutdown error")
	}

	if err := wsRouter.Stop(ctx); err != nil {
		logger.Error().Err(err).Msg("WebSocket server shutdown error")
	}

	grpcServer.GracefulStop()
	udpServer.Stop()

	logger.Info().Msg("Servers stopped")
}

func startSpecUpdater(specHandler *spec.SpecHandler, registryClient *proxy.ServiceRegistryClient, cfg *config.Config) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		specs, err := registryClient.GetAllServiceSpecs()
		if err != nil {
			logger.Error().Err(err).Msg("Failed to fetch service specs")
			continue
		}

		for _, spec := range specs {
			specHandler.UpdateSpec(spec)
		}

		logger.Debug().Int("count", len(specs)).Msg("Updated service specs")
	}
}
