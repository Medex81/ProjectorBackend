// pkg/common/database/postgres_test.go
package database

import (
	"context"
	"testing"

	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPostgresConnection(t *testing.T) {
	t.Run("Valid connection string", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "testuser",
			Password: "testpass",
			DBName:   "testdb",
			SSLMode:  "disable",
		}

		// This would actually try to connect, so we mock it in unit tests
		// For unit tests, we'll just test the DSN generation
		dsn := formatDSN(cfg)
		expected := "postgres://testuser:testpass@localhost:5432/testdb?sslmode=disable"
		assert.Equal(t, expected, dsn)
	})

	t.Run("Missing required fields", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host: "localhost",
		}

		dsn := formatDSN(cfg)
		expected := "postgres://:@localhost:0/?sslmode="
		assert.Equal(t, expected, dsn)
	})
}

func TestPostgresConnection_Pool(t *testing.T) {
	// Create a mock pool for testing
	ctx := context.Background()

	t.Run("Connection pool configuration", func(t *testing.T) {
		cfg := &config.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "test",
			Password: "test",
			DBName:   "test",
			SSLMode:  "disable",
		}

		poolConfig, err := pgxpool.ParseConfig(formatDSN(cfg))
		require.NoError(t, err)

		assert.Equal(t, "localhost", poolConfig.ConnConfig.Host)
		assert.Equal(t, uint16(5432), poolConfig.ConnConfig.Port)
		assert.Equal(t, "test", poolConfig.ConnConfig.User)
		assert.Equal(t, "testdb", poolConfig.ConnConfig.Database)
	})
}

func TestPostgresConnection_WithSSL(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "test",
		Password: "test",
		DBName:   "test",
		SSLMode:  "require",
	}

	dsn := formatDSN(cfg)
	assert.Contains(t, dsn, "sslmode=require")
}

func formatDSN(cfg *config.DatabaseConfig) string {
	return "postgres://" + cfg.User + ":" + cfg.Password + "@" +
		cfg.Host + ":" + string(rune(cfg.Port)) + "/" + cfg.DBName +
		"?sslmode=" + cfg.SSLMode
}
