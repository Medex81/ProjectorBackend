// services/service-registry/test/integration/registry_integration_test.go
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/Medex81/ProjectorBackend/pkg/common/database"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/api"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/repository"
	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/service"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestRegistryService_Integration(t *testing.T) {
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

	// Database config
	dbConfig := &database.Config{
		Host:     host,
		Port:     port.Int(),
		User:     "test",
		Password: "test",
		DBName:   "testdb",
		SSLMode:  "disable",
	}

	// Connect to databases
	db, err := database.NewPostgresConnection(ctx, dbConfig)
	require.NoError(t, err)
	defer db.Close()

	redisConfig := &database.RedisConfig{
		Addr: redisHost + ":" + redisPort.Port(),
		DB:   0,
	}
	redisClient, err := database.NewRedisConnection(redisConfig)
	require.NoError(t, err)
	defer redisClient.Close()

	// Run migrations
	err = runMigrations(db)
	require.NoError(t, err)

	// Initialize repository and service
	repo := repository.NewServiceRepository(db, redisClient)
	registryService := service.NewRegistryService(repo, nil)

	// Setup HTTP server
	router := mux.NewRouter()
	handler := api.NewHandler(registryService)
	handler.RegisterRoutes(router)

	server := httptest.NewServer(router)
	defer server.Close()

	t.Run("Register and discover service", func(t *testing.T) {
		// Register service
		spec := &api.ServiceAPISpec{
			ServiceName: "test-service",
			Version:     "1.0.0",
			Spec: api.OpenAPI{
				OpenAPI: "3.0.0",
				Info: api.Info{
					Title:   "Test Service",
					Version: "1.0.0",
				},
				Paths: api.Paths{
					"/test": api.PathItem{
						Get: &api.Operation{},
					},
				},
			},
		}

		body, err := json.Marshal(spec)
		require.NoError(t, err)

		resp, err := http.Post(server.URL+"/api/v1/services", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Get service
		resp, err = http.Get(server.URL + "/api/v1/services/test-service")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var retrieved api.ServiceAPISpec
		err = json.NewDecoder(resp.Body).Decode(&retrieved)
		require.NoError(t, err)
		assert.Equal(t, "test-service", retrieved.ServiceName)
		assert.Equal(t, "1.0.0", retrieved.Version)

		// List services
		resp, err = http.Get(server.URL + "/api/v1/services")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var services []*api.ServiceAPISpec
		err = json.NewDecoder(resp.Body).Decode(&services)
		require.NoError(t, err)
		assert.Len(t, services, 1)
	})

	t.Run("Register multiple services", func(t *testing.T) {
		services := []string{"auth", "game", "payment"}

		for _, name := range services {
			spec := &api.ServiceAPISpec{
				ServiceName: name,
				Version:     "1.0.0",
				Spec: api.OpenAPI{
					Info: api.Info{
						Title: name + " Service",
					},
				},
			}

			body, _ := json.Marshal(spec)
			resp, err := http.Post(server.URL+"/api/v1/services", "application/json", bytes.NewReader(body))
			require.NoError(t, err)
			assert.Equal(t, http.StatusCreated, resp.StatusCode)
		}

		// List all services
		resp, err := http.Get(server.URL + "/api/v1/services")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var allServices []*api.ServiceAPISpec
		err = json.NewDecoder(resp.Body).Decode(&allServices)
		require.NoError(t, err)
		assert.Len(t, allServices, 4) // 3 new + 1 from previous test
	})

	t.Run("Update service", func(t *testing.T) {
		updatedSpec := &api.ServiceAPISpec{
			ServiceName: "test-service",
			Version:     "2.0.0",
			Spec: api.OpenAPI{
				Info: api.Info{
					Title:   "Updated Test Service",
					Version: "2.0.0",
				},
				Paths: api.Paths{
					"/test": api.PathItem{
						Get:  &api.Operation{},
						Post: &api.Operation{},
					},
				},
			},
		}

		body, _ := json.Marshal(updatedSpec)
		req, _ := http.NewRequest("PUT", server.URL+"/api/v1/services/test-service", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify update
		resp, err = http.Get(server.URL + "/api/v1/services/test-service")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var service api.ServiceAPISpec
		err = json.NewDecoder(resp.Body).Decode(&service)
		require.NoError(t, err)
		assert.Equal(t, "2.0.0", service.Version)
	})

	t.Run("Delete service", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", server.URL+"/api/v1/services/test-service", nil)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)

		// Verify deletion
		resp, err = http.Get(server.URL + "/api/v1/services/test-service")
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("Health check", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/health")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var health map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&health)
		require.NoError(t, err)
		assert.Equal(t, "healthy", health["status"])
		assert.True(t, health["healthy"].(bool))
	})

	t.Run("Invalid requests", func(t *testing.T) {
		// Missing service name
		invalidSpec := map[string]interface{}{
			"version": "1.0.0",
			"spec":    map[string]interface{}{},
		}
		body, _ := json.Marshal(invalidSpec)
		resp, err := http.Post(server.URL+"/api/v1/services", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		// Invalid JSON
		resp, err = http.Post(server.URL+"/api/v1/services", "application/json", bytes.NewReader([]byte("invalid json")))
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		// Non-existent service
		resp, err = http.Get(server.URL + "/api/v1/services/nonexistent")
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func runMigrations(db *pgxpool.Pool) error {
	migrations := []string{
		`CREATE TYPE service_status AS ENUM ('active', 'inactive', 'degraded', 'down')`,

		`CREATE TABLE IF NOT EXISTS services (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            name VARCHAR(255) NOT NULL UNIQUE,
            version VARCHAR(50) NOT NULL,
            spec JSONB NOT NULL,
            status service_status NOT NULL DEFAULT 'active',
            metadata JSONB DEFAULT '{}'::jsonb,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
            last_heartbeat TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        )`,

		`CREATE INDEX idx_services_name ON services(name)`,
		`CREATE INDEX idx_services_status ON services(status)`,
		`CREATE INDEX idx_services_last_heartbeat ON services(last_heartbeat)`,

		`CREATE TABLE IF NOT EXISTS endpoints (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
            url VARCHAR(255) NOT NULL,
            protocol VARCHAR(10) NOT NULL,
            methods TEXT[],
            weight INT DEFAULT 1,
            healthy BOOLEAN DEFAULT TRUE,
            last_check TIMESTAMP WITH TIME ZONE,
            response_time BIGINT,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
            UNIQUE(service_id, url)
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
