// pkg/common/config/config_test.go
package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create temporary config file
	configContent := `
service:
  name: test-service
  port: 8080
  grpc_port: 50051
  environment: development
  version: 1.0.0
  shutdown_timeout: 30s

database:
  host: localhost
  port: 5432
  user: testuser
  password: testpass
  dbname: testdb
  sslmode: disable

redis:
  addr: localhost:6379
  password: ""
  db: 0

kafka:
  brokers:
    - localhost:9092
  topics:
    email_verification: email-verification
    service_events: service-events
    logs: service-logs

auth:
  jwt_secret: test-secret
  token_expiration: 24h
  verification_code_ttl: 10m

gateway:
  http_port: 8080
  https_port: 8443
  ws_port: 8080
  udp_port: 8080
  grpc_port: 50052

mail:
  smtp_host: smtp.gmail.com
  smtp_port: 587
  smtp_user: test@gmail.com
  smtp_password: testpass
  from_email: noreply@test.com

jaeger:
  agent_host: localhost
  agent_port: 6831
`

	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write([]byte(configContent))
	require.NoError(t, err)
	err = tmpfile.Close()
	require.NoError(t, err)

	t.Run("Load valid config", func(t *testing.T) {
		cfg, err := LoadConfig(tmpfile.Name())
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// Verify service config
		assert.Equal(t, "test-service", cfg.Service.Name)
		assert.Equal(t, 8080, cfg.Service.Port)
		assert.Equal(t, 50051, cfg.Service.GRPCPort)
		assert.Equal(t, "development", cfg.Service.Environment)
		assert.Equal(t, "1.0.0", cfg.Service.Version)
		assert.Equal(t, 30*time.Second, cfg.Service.ShutdownTimeout)

		// Verify database config
		assert.Equal(t, "localhost", cfg.Database.Host)
		assert.Equal(t, 5432, cfg.Database.Port)
		assert.Equal(t, "testuser", cfg.Database.User)
		assert.Equal(t, "testpass", cfg.Database.Password)
		assert.Equal(t, "testdb", cfg.Database.DBName)
		assert.Equal(t, "disable", cfg.Database.SSLMode)

		// Verify Redis config
		assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
		assert.Equal(t, "", cfg.Redis.Password)
		assert.Equal(t, 0, cfg.Redis.DB)

		// Verify Kafka config
		assert.Len(t, cfg.Kafka.Brokers, 1)
		assert.Equal(t, "localhost:9092", cfg.Kafka.Brokers[0])
		assert.Equal(t, "email-verification", cfg.Kafka.Topics.EmailVerification)
		assert.Equal(t, "service-events", cfg.Kafka.Topics.ServiceEvents)
		assert.Equal(t, "service-logs", cfg.Kafka.Topics.Logs)

		// Verify Auth config
		assert.Equal(t, "test-secret", cfg.Auth.JWTSecret)
		assert.Equal(t, 24*time.Hour, cfg.Auth.TokenExpiration)
		assert.Equal(t, 10*time.Minute, cfg.Auth.VerificationCodeTTL)

		// Verify Gateway config
		assert.Equal(t, 8080, cfg.Gateway.HTTPPort)
		assert.Equal(t, 8443, cfg.Gateway.HTTPSPort)
		assert.Equal(t, 8080, cfg.Gateway.WSPort)
		assert.Equal(t, 8080, cfg.Gateway.UDPPort)
		assert.Equal(t, 50052, cfg.Gateway.GRPCPort)

		// Verify Mail config
		assert.Equal(t, "smtp.gmail.com", cfg.Mail.SMTPHost)
		assert.Equal(t, 587, cfg.Mail.SMTPPort)
		assert.Equal(t, "test@gmail.com", cfg.Mail.SMTPUser)
		assert.Equal(t, "testpass", cfg.Mail.SMTPPassword)
		assert.Equal(t, "noreply@test.com", cfg.Mail.FromEmail)

		// Verify Jaeger config
		assert.Equal(t, "localhost", cfg.Jaeger.AgentHost)
		assert.Equal(t, 6831, cfg.Jaeger.AgentPort)
	})

	t.Run("Load non-existent file", func(t *testing.T) {
		cfg, err := LoadConfig("non-existent.yaml")
		assert.Error(t, err)
		assert.Nil(t, cfg)
	})

	t.Run("Load invalid YAML", func(t *testing.T) {
		invalidFile, err := os.CreateTemp("", "invalid-*.yaml")
		require.NoError(t, err)
		defer os.Remove(invalidFile.Name())

		_, err = invalidFile.Write([]byte("invalid: yaml: : :"))
		require.NoError(t, err)
		invalidFile.Close()

		cfg, err := LoadConfig(invalidFile.Name())
		assert.Error(t, err)
		assert.Nil(t, cfg)
	})

	t.Run("Load with environment variables override", func(t *testing.T) {
		os.Setenv("SERVICE_NAME", "env-override")
		os.Setenv("DATABASE_HOST", "env-host")
		defer os.Unsetenv("SERVICE_NAME")
		defer os.Unsetenv("DATABASE_HOST")

		cfg, err := LoadConfig(tmpfile.Name())
		require.NoError(t, err)

		// Note: This requires implementing env override in config.go
		// assert.Equal(t, "env-override", cfg.Service.Name)
		// assert.Equal(t, "env-host", cfg.Database.Host)
	})
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		expectErr bool
	}{
		{
			name: "Valid config",
			config: &Config{
				Service: ServiceConfig{
					Name:    "test",
					Port:    8080,
					Version: "1.0.0",
				},
				Database: DatabaseConfig{
					Host: "localhost",
					Port: 5432,
					User: "user",
				},
				Auth: AuthConfig{
					JWTSecret: "secret",
				},
			},
			expectErr: false,
		},
		{
			name: "Missing service name",
			config: &Config{
				Service: ServiceConfig{
					Port: 8080,
				},
			},
			expectErr: true,
		},
		{
			name: "Invalid port",
			config: &Config{
				Service: ServiceConfig{
					Name: "test",
					Port: 99999,
				},
			},
			expectErr: true,
		},
		{
			name: "Missing JWT secret",
			config: &Config{
				Service: ServiceConfig{
					Name: "test",
					Port: 8080,
				},
				Auth: AuthConfig{
					JWTSecret: "",
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
