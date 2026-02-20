// services/service-registry/internal/repository/service_repository_test.go
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Medex81/ProjectorBackend/pkg/common/api"
	"github.com/go-redis/redismock/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceRepository_Create(t *testing.T) {
	// Setup mock DB
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Setup mock Redis
	rdb, rmock := redismock.NewClientMock()

	repo := NewServiceRepository(db, rdb)

	ctx := context.Background()
	service := &api.ServiceAPISpec{
		ServiceName: "test-service",
		Version:     "1.0.0",
		Spec: api.OpenAPI{
			Info: api.Info{
				Title: "Test Service",
			},
		},
	}

	t.Run("Create new service", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO services").
			WithArgs(sqlmock.AnyArg(), "test-service", "1.0.0", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		rmock.ExpectSet("service:test-service:1.0.0", sqlmock.AnyArg()).
			SetVal("OK")

		err := repo.Register(ctx, service)
		assert.NoError(t, err)
	})

	t.Run("Create duplicate service", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO services").
			WithArgs(sqlmock.AnyArg(), "test-service", "1.0.0", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnError(fmt.Errorf("duplicate key violation"))

		err := repo.Register(ctx, service)
		assert.Error(t, err)
	})

	t.Run("Create with missing required fields", func(t *testing.T) {
		invalidService := &api.ServiceAPISpec{
			// Missing ServiceName
		}

		err := repo.Register(ctx, invalidService)
		assert.Error(t, err)
	})
}

func TestServiceRepository_Get(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rdb, rmock := redismock.NewClientMock()
	repo := NewServiceRepository(db, rdb)

	ctx := context.Background()
	serviceID := "test-service-id"

	t.Run("Get existing service from cache", func(t *testing.T) {
		cachedSpec := &api.ServiceAPISpec{
			ServiceName: "test-service",
			Version:     "1.0.0",
		}
		cachedData, _ := json.Marshal(cachedSpec)

		rmock.ExpectGet("service:" + serviceID).
			SetVal(string(cachedData))

		spec, err := repo.Get(ctx, serviceID)
		assert.NoError(t, err)
		assert.Equal(t, "test-service", spec.ServiceName)
	})

	t.Run("Get service from DB when not in cache", func(t *testing.T) {
		rmock.ExpectGet("service:" + serviceID).
			RedisNil()

		rows := sqlmock.NewRows([]string{"id", "name", "version", "spec", "created_at", "updated_at"}).
			AddRow(serviceID, "test-service", "1.0.0", `{"openapi":"3.0.0"}`, time.Now(), time.Now())

		mock.ExpectQuery("SELECT .+ FROM services WHERE id").
			WithArgs(serviceID).
			WillReturnRows(rows)

		rmock.ExpectSet("service:"+serviceID, sqlmock.AnyArg()).
			SetVal("OK")

		spec, err := repo.Get(ctx, serviceID)
		assert.NoError(t, err)
		assert.Equal(t, "test-service", spec.ServiceName)
	})

	t.Run("Get non-existent service", func(t *testing.T) {
		rmock.ExpectGet("service:" + serviceID).
			RedisNil()

		mock.ExpectQuery("SELECT .+ FROM services WHERE id").
			WithArgs(serviceID).
			WillReturnError(sql.ErrNoRows)

		spec, err := repo.Get(ctx, serviceID)
		assert.Error(t, err)
		assert.Nil(t, spec)
	})
}

func TestServiceRepository_List(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rdb, _ := redismock.NewClientMock()
	repo := NewServiceRepository(db, rdb)

	ctx := context.Background()

	t.Run("List all services", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "name", "version", "spec", "created_at", "updated_at"}).
			AddRow("id1", "service1", "1.0.0", `{"openapi":"3.0.0"}`, time.Now(), time.Now()).
			AddRow("id2", "service2", "1.0.0", `{"openapi":"3.0.0"}`, time.Now(), time.Now())

		mock.ExpectQuery("SELECT .+ FROM services").
			WillReturnRows(rows)

		services, err := repo.List(ctx)
		assert.NoError(t, err)
		assert.Len(t, services, 2)
	})

	t.Run("Empty list", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "name", "version", "spec", "created_at", "updated_at"})

		mock.ExpectQuery("SELECT .+ FROM services").
			WillReturnRows(rows)

		services, err := repo.List(ctx)
		assert.NoError(t, err)
		assert.Empty(t, services)
	})

	t.Run("Database error", func(t *testing.T) {
		mock.ExpectQuery("SELECT .+ FROM services").
			WillReturnError(fmt.Errorf("database connection error"))

		services, err := repo.List(ctx)
		assert.Error(t, err)
		assert.Nil(t, services)
	})
}

func TestServiceRepository_Update(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rdb, rmock := redismock.NewClientMock()
	repo := NewServiceRepository(db, rdb)

	ctx := context.Background()
	serviceID := "test-service-id"
	service := &api.ServiceAPISpec{
		ServiceName: "test-service",
		Version:     "2.0.0",
	}

	t.Run("Update existing service", func(t *testing.T) {
		mock.ExpectExec("UPDATE services SET").
			WithArgs("2.0.0", sqlmock.AnyArg(), sqlmock.AnyArg(), serviceID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		rmock.ExpectDel("service:" + serviceID).
			SetVal(1)

		err := repo.Update(ctx, serviceID, service)
		assert.NoError(t, err)
	})

	t.Run("Update non-existent service", func(t *testing.T) {
		mock.ExpectExec("UPDATE services SET").
			WithArgs("2.0.0", sqlmock.AnyArg(), sqlmock.AnyArg(), serviceID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Update(ctx, serviceID, service)
		assert.Error(t, err)
	})
}

func TestServiceRepository_Delete(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rdb, rmock := redismock.NewClientMock()
	repo := NewServiceRepository(db, rdb)

	ctx := context.Background()
	serviceID := "test-service-id"

	t.Run("Delete existing service", func(t *testing.T) {
		mock.ExpectExec("DELETE FROM services WHERE id").
			WithArgs(serviceID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		rmock.ExpectDel("service:" + serviceID).
			SetVal(1)

		err := repo.Delete(ctx, serviceID)
		assert.NoError(t, err)
	})

	t.Run("Delete non-existent service", func(t *testing.T) {
		mock.ExpectExec("DELETE FROM services WHERE id").
			WithArgs(serviceID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(ctx, serviceID)
		assert.Error(t, err)
	})
}

func TestServiceRepository_HealthCheck(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rdb, rmock := redismock.NewClientMock()
	repo := NewServiceRepository(db, rdb)

	ctx := context.Background()

	t.Run("Health check - all systems operational", func(t *testing.T) {
		mock.ExpectPing()

		rmock.ExpectPing()
		rmock.SetVal("PONG")

		healthy := repo.HealthCheck(ctx)
		assert.True(t, healthy)
	})

	t.Run("Health check - DB down", func(t *testing.T) {
		mock.ExpectPing().
			WillReturnError(fmt.Errorf("database down"))

		healthy := repo.HealthCheck(ctx)
		assert.False(t, healthy)
	})

	t.Run("Health check - Redis down", func(t *testing.T) {
		mock.ExpectPing()

		rmock.ExpectPing()
		rmock.SetErr(fmt.Errorf("redis down"))

		healthy := repo.HealthCheck(ctx)
		assert.False(t, healthy)
	})
}
