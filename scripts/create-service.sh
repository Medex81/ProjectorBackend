#!/bin/bash
# scripts/create-service.sh

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}ProjectorBackend Service Generator${NC}"
echo "======================================"

# Get service name
read -p "Enter service name (e.g., user-service): " SERVICE_NAME
if [ -z "$SERVICE_NAME" ]; then
    echo -e "${RED}Error: Service name cannot be empty${NC}"
    exit 1
fi

# Get service port
read -p "Enter HTTP port (default: 8080): " HTTP_PORT
HTTP_PORT=${HTTP_PORT:-8080}

read -p "Enter gRPC port (default: 50051): " GRPC_PORT
GRPC_PORT=${GRPC_PORT:-50051}

# Create service directory
SERVICE_DIR="services/$SERVICE_NAME"
if [ -d "$SERVICE_DIR" ]; then
    echo -e "${RED}Error: Service directory already exists${NC}"
    exit 1
fi

echo -e "${YELLOW}Creating service: $SERVICE_NAME${NC}"

# Create directory structure
mkdir -p "$SERVICE_DIR"/{cmd,configs,internal/{models,repository,service,api},migrations,test/{unit,integration}}

# Create go.mod
cat > "$SERVICE_DIR/go.mod" << EOF
module github.com/Medex81/ProjectorBackend/services/$SERVICE_NAME

go 1.25.7

require (
    github.com/Medex81/ProjectorBackend/pkg/common v0.0.0
    github.com/gorilla/mux v1.8.1
    google.golang.org/grpc v1.69.4
)

replace github.com/Medex81/ProjectorBackend/pkg/common => ../../pkg/common
EOF

# Create main.go
cat > "$SERVICE_DIR/cmd/main.go" << EOF
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
    "github.com/Medex81/ProjectorBackend/pkg/common/logger"
    "github.com/Medex81/ProjectorBackend/pkg/common/tracing"
    "github.com/Medex81/ProjectorBackend/services/$SERVICE_NAME/internal/api"
    "github.com/Medex81/ProjectorBackend/services/$SERVICE_NAME/internal/repository"
    "github.com/Medex81/ProjectorBackend/services/$SERVICE_NAME/internal/service"
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

    // Initialize repositories
    repo := repository.NewRepository(db, redisClient)

    // Initialize service
    svc := service.NewService(repo, cfg)

    // Initialize HTTP server
    router := mux.NewRouter()
    handler := api.NewHandler(svc, cfg)
    handler.RegisterRoutes(router)

    httpServer := &http.Server{
        Addr:         fmt.Sprintf(":%d", cfg.Service.Port),
        Handler:      router,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    // Start server
    go func() {
        logger.Info().Int("port", cfg.Service.Port).Msg("Starting HTTP server")
        if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Fatal().Err(err).Msg("Failed to start HTTP server")
        }
    }()

    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    logger.Info().Msg("Shutting down server...")

    ctx, cancel := context.WithTimeout(context.Background(), cfg.Service.ShutdownTimeout)
    defer cancel()

    if err := httpServer.Shutdown(ctx); err != nil {
        logger.Error().Err(err).Msg("HTTP server shutdown error")
    }

    logger.Info().Msg("Server stopped")
}
EOF

# Create config.dev.yaml
cat > "$SERVICE_DIR/configs/config.dev.yaml" << EOF
service:
  name: $SERVICE_NAME
  port: $HTTP_PORT
  grpc_port: $GRPC_PORT
  environment: development
  version: 1.0.0
  shutdown_timeout: 30s

database:
  host: postgres
  port: 5432
  user: projector
  password: projector123
  dbname: $SERVICE_NAME
  sslmode: disable

redis:
  addr: redis:6379
  password: ""
  db: 0

kafka:
  brokers:
    - kafka:9092
  topics:
    email_verification: email-verification
    service_events: service-events
    logs: service-logs

auth:
  jwt_secret: your-secret-key-here
  token_expiration: 24h
  verification_code_ttl: 10m

jaeger:
  agent_host: jaeger
  agent_port: 6831
EOF

# Create Dockerfile.dev
cat > "$SERVICE_DIR/Dockerfile.dev" << EOF
FROM golang:1.25.7-alpine AS builder

WORKDIR /app

RUN go install github.com/air-verse/air@latest

COPY $SERVICE_NAME/go.mod $SERVICE_NAME/go.sum ./
COPY pkg/common/go.mod pkg/common/go.sum ../../pkg/common/

RUN go mod download

COPY $SERVICE_NAME/ .
COPY pkg/common ../../pkg/common/

RUN echo 'root = "."\ntmp_dir = "tmp"\n[build]\n  bin = "./tmp/main"\n  cmd = "go build -o ./tmp/main ./cmd/main.go"\n  delay = 1000\n  exclude_dir = ["assets", "tmp", "vendor"]\n  exclude_file = []\n  exclude_regex = ["_test.go"]\n  exclude_unchanged = false\n  follow_symlink = false\n  full_bin = ""\n  include_dir = []\n  include_ext = ["go", "yaml", "yml"]\n  kill_delay = "0s"\n  log = "build-errors.log"\n  send_interrupt = false\n  stop_on_error = false\n[color]\n  app = ""\n  build = "yellow"\n  main = "magenta"\n  runner = "green"\n  watcher = "cyan"\n[log]\n  time = false\n[misc]\n  clean_on_exit = false' > .air.toml

EXPOSE $HTTP_PORT $GRPC_PORT

CMD ["air", "-c", ".air.toml"]
EOF

# Create repository.go
cat > "$SERVICE_DIR/internal/repository/repository.go" << EOF
package repository

import (
    "context"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/go-redis/redis/v8"
)

type Repository struct {
    db    *pgxpool.Pool
    redis *redis.Client
}

func NewRepository(db *pgxpool.Pool, redis *redis.Client) *Repository {
    return &Repository{
        db:    db,
        redis: redis,
    }
}

// Add your repository methods here
EOF

# Create service.go
cat > "$SERVICE_DIR/internal/service/service.go" << EOF
package service

import (
    "context"

    "github.com/Medex81/ProjectorBackend/pkg/common/config"
    "github.com/Medex81/ProjectorBackend/services/$SERVICE_NAME/internal/repository"
)

type Service struct {
    repo   *repository.Repository
    config *config.Config
}

func NewService(repo *repository.Repository, cfg *config.Config) *Service {
    return &Service{
        repo:   repo,
        config: cfg,
    }
}

// Add your service methods here
EOF

# Create handler.go
cat > "$SERVICE_DIR/internal/api/handler.go" << EOF
package api

import (
    "encoding/json"
    "net/http"

    "github.com/Medex81/ProjectorBackend/pkg/common/config"
    "github.com/Medex81/ProjectorBackend/services/$SERVICE_NAME/internal/service"
    "github.com/gorilla/mux"
)

type Handler struct {
    service *service.Service
    config  *config.Config
}

func NewHandler(service *service.Service, cfg *config.Config) *Handler {
    return &Handler{
        service: service,
        config:  cfg,
    }
}

func (h *Handler) RegisterRoutes(router *mux.Router) {
    router.HandleFunc("/health", h.Health).Methods("GET")
    // Add your routes here
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "healthy",
        "service": h.config.Service.Name,
    })
}
EOF

# Create initial migration
cat > "$SERVICE_DIR/migrations/001_init.sql" << EOF
-- +goose Up
CREATE TABLE IF NOT EXISTS examples (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS examples;
EOF

# Update go.work
echo "go 1.25.7" > go.work
echo "" >> go.work
echo "use (" >> go.work
echo "    ./pkg/common" >> go.work
for dir in services/*/; do
    echo "    ./${dir}" >> go.work
done
echo ")" >> go.work

# Update docker-compose.dev.yml
cat >> docker-compose.dev.yml << EOF

  $SERVICE_NAME:
    build:
      context: ./services/$SERVICE_NAME
      dockerfile: Dockerfile.dev
    ports:
      - "$HTTP_PORT:$HTTP_PORT"
      - "$GRPC_PORT:$GRPC_PORT"
    volumes:
      - ./services/$SERVICE_NAME:/app
      - /app/tmp
    environment:
      - CONFIG_PATH=/app/configs/config.dev.yaml
      - DB_HOST=postgres
      - DB_PORT=5432
      - REDIS_ADDR=redis:6379
      - KAFKA_BROKERS=kafka:9092
    depends_on:
      - postgres
      - redis
      - kafka
    networks:
      - projector-network
EOF

echo -e "${GREEN}✓ Service $SERVICE_NAME created successfully!${NC}"
echo -e "${YELLOW}Next steps:${NC}"
echo "1. cd $SERVICE_DIR"
echo "2. Implement your business logic"
echo "3. Update database models"
echo "4. Add tests"
echo "5. Run: docker-compose -f docker-compose.dev.yml up $SERVICE_NAME"