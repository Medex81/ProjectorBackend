// services/identity-provider/test/integration/auth_integration_test.go
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/database"
	"github.com/Medex81/ProjectorBackend/pkg/common/kafka"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/api"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/repository"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/service"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestAuthService_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()

	// Setup PostgreSQL container
	postgres, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:15-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_USER":     "test",
				"POSTGRES_PASSWORD": "test",
				"POSTGRES_DB":       "testdb",
			},
			WaitingFor: wait.ForLog("database system is ready"),
		},
		Started: true,
	})
	require.NoError(t, err)
	defer postgres.Terminate(ctx)

	// Get PostgreSQL connection details
	host, err := postgres.Host(ctx)
	require.NoError(t, err)
	port, err := postgres.MappedPort(ctx, "5432")
	require.NoError(t, err)

	// Setup Redis container
	redis, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections"),
		},
		Started: true,
	})
	require.NoError(t, err)
	defer redis.Terminate(ctx)

	redisHost, err := redis.Host(ctx)
	require.NoError(t, err)
	redisPort, err := redis.MappedPort(ctx, "6379")
	require.NoError(t, err)

	// Setup Kafka container
	kafka, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "confluentinc/cp-kafka:latest",
			ExposedPorts: []string{"9092/tcp"},
			Env: map[string]string{
				"KAFKA_BROKER_ID":            "1",
				"KAFKA_ZOOKEEPER_CONNECT":    "zookeeper:2181",
				"KAFKA_ADVERTISED_LISTENERS": "PLAINTEXT://localhost:9092",
			},
			WaitingFor: wait.ForLog("started"),
		},
		Started: true,
	})
	require.NoError(t, err)
	defer kafka.Terminate(ctx)

	// Configuration
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name: "identity-provider-test",
		},
		Database: config.DatabaseConfig{
			Host:     host,
			Port:     port.Int(),
			User:     "test",
			Password: "test",
			DBName:   "testdb",
			SSLMode:  "disable",
		},
		Redis: config.RedisConfig{
			Addr: redisHost + ":" + redisPort.Port(),
			DB:   0,
		},
		Kafka: config.KafkaConfig{
			Brokers: []string{"localhost:9092"},
			Topics: struct {
				EmailVerification string
				ServiceEvents     string
				Logs              string
			}{
				EmailVerification: "email-verification-test",
				ServiceEvents:     "service-events-test",
				Logs:              "logs-test",
			},
		},
		Auth: config.AuthConfig{
			JWTSecret:           "test-secret",
			TokenExpiration:     24 * time.Hour,
			VerificationCodeTTL: 10 * time.Minute,
		},
	}

	// Connect to databases
	db, err := database.NewPostgresConnection(ctx, &cfg.Database)
	require.NoError(t, err)
	defer db.Close()

	redisClient, err := database.NewRedisConnection(&cfg.Redis)
	require.NoError(t, err)
	defer redisClient.Close()

	// Run migrations
	err = runMigrations(db)
	require.NoError(t, err)

	// Initialize components
	userRepo := repository.NewUserRepository(db, redisClient)
	kafkaProducer := kafka.NewProducer(cfg.Kafka.Brokers)
	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)
	authService := service.NewAuthService(userRepo, kafkaProducer, jwtManager, cfg)

	// Setup HTTP server
	router := mux.NewRouter()
	handler := api.NewHandler(authService, cfg)
	handler.RegisterRoutes(router)

	server := httptest.NewServer(router)
	defer server.Close()

	// Test cases
	t.Run("Complete registration and verification flow", func(t *testing.T) {
		// 1. Register
		registerBody := map[string]interface{}{
			"email":    "integration@example.com",
			"password": "Test123!",
			"app_id":   "test-app",
		}
		registerJSON, _ := json.Marshal(registerBody)

		resp, err := http.Post(server.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(registerJSON))
		require.NoError(t, err)
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)

		// 2. Get verification code from Redis
		var code string
		for i := 0; i < 10; i++ {
			code, err = redisClient.Get(ctx, "verification:integration@example.com:test-app").Result()
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		require.NoError(t, err)
		assert.NotEmpty(t, code)

		// 3. Verify with code
		verifyBody := map[string]interface{}{
			"email":  "integration@example.com",
			"code":   code,
			"app_id": "test-app",
		}
		verifyJSON, _ := json.Marshal(verifyBody)

		resp, err = http.Post(server.URL+"/api/v1/auth/verify", "application/json", bytes.NewReader(verifyJSON))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var verifyResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&verifyResp)
		require.NoError(t, err)

		token, ok := verifyResp["token"]
		assert.True(t, ok)
		assert.NotEmpty(t, token)

		// 4. Login with credentials
		loginBody := map[string]interface{}{
			"email":    "integration@example.com",
			"password": "Test123!",
			"app_id":   "test-app",
		}
		loginJSON, _ := json.Marshal(loginBody)

		resp, err = http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(loginJSON))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var loginResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&loginResp)
		require.NoError(t, err)

		loginToken, ok := loginResp["token"]
		assert.True(t, ok)
		assert.NotEmpty(t, loginToken)
	})

	t.Run("Registration with existing email", func(t *testing.T) {
		registerBody := map[string]interface{}{
			"email":    "existing@example.com",
			"password": "Test123!",
			"app_id":   "test-app",
		}
		registerJSON, _ := json.Marshal(registerBody)

		// First registration
		resp, err := http.Post(server.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(registerJSON))
		require.NoError(t, err)
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)

		// Second registration with same email
		resp, err = http.Post(server.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(registerJSON))
		require.NoError(t, err)
		assert.Equal(t, http.StatusAccepted, resp.StatusCode) // Should still accept (resend code)
	})

	t.Run("Login with wrong password", func(t *testing.T) {
		loginBody := map[string]interface{}{
			"email":    "integration@example.com",
			"password": "WrongPassword!",
			"app_id":   "test-app",
		}
		loginJSON, _ := json.Marshal(loginBody)

		resp, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(loginJSON))
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Verify with wrong code", func(t *testing.T) {
		verifyBody := map[string]interface{}{
			"email":  "integration@example.com",
			"code":   "000000",
			"app_id": "test-app",
		}
		verifyJSON, _ := json.Marshal(verifyBody)

		resp, err := http.Post(server.URL+"/api/v1/auth/verify", "application/json", bytes.NewReader(verifyJSON))
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Missing required fields", func(t *testing.T) {
		// Missing email
		invalidBody := map[string]interface{}{
			"password": "Test123!",
			"app_id":   "test-app",
		}
		invalidJSON, _ := json.Marshal(invalidBody)

		resp, err := http.Post(server.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(invalidJSON))
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func runMigrations(db *pgxpool.Pool) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            email VARCHAR(255) NOT NULL,
            password_hash VARCHAR(255) NOT NULL,
            app_id VARCHAR(100) NOT NULL,
            verified BOOLEAN DEFAULT FALSE,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
            last_login_at TIMESTAMP WITH TIME ZONE,
            UNIQUE(email, app_id)
        )`,
		`CREATE TABLE IF NOT EXISTS verification_codes (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            user_id UUID REFERENCES users(id) ON DELETE CASCADE,
            code VARCHAR(10) NOT NULL,
            email VARCHAR(255) NOT NULL,
            app_id VARCHAR(100) NOT NULL,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            used BOOLEAN DEFAULT FALSE,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        )`,
	}

	for _, migration := range migrations {
		_, err := db.Exec(context.Background(), migration)
		if err != nil {
			return err
		}
	}
	return nil
}
