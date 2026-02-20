// pkg/common/go.mod
module github.com/Medex81/ProjectorBackend/pkg/common

go 1.25.7

require (
    github.com/go-redis/redis/v8 v8.11.5
    github.com/golang-jwt/jwt/v5 v5.2.1
    github.com/google/uuid v1.6.0
    github.com/gorilla/websocket v1.5.3
    github.com/jackc/pgx/v5 v5.7.2
    github.com/prometheus/client_golang v1.20.5
    github.com/rs/zerolog v1.33.0
    github.com/segmentio/kafka-go v0.4.47
    go.opentelemetry.io/otel v1.34.0
    go.opentelemetry.io/otel/exporters/jaeger v1.17.0
    go.opentelemetry.io/otel/sdk v1.34.0
    go.opentelemetry.io/otel/trace v1.34.0
    google.golang.org/grpc v1.69.4
    google.golang.org/protobuf v1.36.3
    gopkg.in/yaml.v3 v3.0.1
)