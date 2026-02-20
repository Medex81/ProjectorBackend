// services/mail-service/cmd/main.go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"github.com/Medex81/ProjectorBackend/services/mail-service/internal/service"
)

func main() {
	// Load configuration
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "configs/config.dev.yaml"
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to load config")
	}

	// Initialize logger
	logger.Init(cfg.Service.Name, cfg.Service.Environment)

	// Initialize tracer
	tp, err := tracing.InitTracer(cfg.Service.Name, cfg.Jaeger.AgentHost, cfg.Jaeger.AgentPort)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize tracer")
	}
	defer tp.Shutdown(context.Background())

	// Create mail service
	mailService, err := service.NewMailService(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create mail service")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start service
	go func() {
		if err := mailService.Start(ctx); err != nil {
			logger.Error().Err(err).Msg("Mail service error")
		}
	}()

	logger.Info().Msg("Mail service started")

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("Shutting down mail service...")

	if err := mailService.Stop(); err != nil {
		logger.Error().Err(err).Msg("Error stopping mail service")
	}

	logger.Info().Msg("Mail service stopped")
}
