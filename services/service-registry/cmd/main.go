// services/service-registry/cmd/main.go
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

	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/database"
	"github.com/Medex81/ProjectorBackend/pkg/common/kafka"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/api"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/repository"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/service"
	"github.com/gorilla/mux"
	"google.golang.org/grpc"
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

	// Connect to database
	ctx := context.Background()
	db, err := database.NewPostgresConnection(ctx, &cfg.Database)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	// Connect to Redis
	redisClient, err := database.NewRedisConnection(&cfg.Redis)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer redisClient.Close()

	// Initialize Kafka producer
	kafkaProducer := kafka.NewProducer(cfg.Kafka.Brokers)
	defer kafkaProducer.Close()

	// Initialize repositories
	serviceRepo := repository.NewServiceRepository(db, redisClient)

	// Initialize service
	registryService := service.NewRegistryService(serviceRepo, kafkaProducer, cfg)

	// Initialize HTTP server
	router := mux.NewRouter()
	handler := api.NewHandler(registryService, cfg)

	// Register routes
	handler.RegisterRoutes(router)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Service.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Initialize gRPC server
	grpcServer := grpc.NewServer()
	// Register gRPC services here

	// Start servers
	go func() {
		logger.Info().Int("port", cfg.Service.Port).Msg("Starting HTTP server")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Failed to start HTTP server")
		}
	}()

	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Service.GRPCPort))
		if err != nil {
			logger.Fatal().Err(err).Msg("Failed to listen for gRPC")
		}
		logger.Info().Int("port", cfg.Service.GRPCPort).Msg("Starting gRPC server")
		if err := grpcServer.Serve(lis); err != nil {
			logger.Fatal().Err(err).Msg("Failed to start gRPC server")
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

	grpcServer.GracefulStop()

	logger.Info().Msg("Servers stopped")
}
