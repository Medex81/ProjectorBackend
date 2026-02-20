// services/service-registry/internal/repository/service_repository.go
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Medex81/ProjectorBackend/services/service-registry/internal/models"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ServiceRepository struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewServiceRepository(db *pgxpool.Pool, redis *redis.Client) *ServiceRepository {
	return &ServiceRepository{
		db:    db,
		redis: redis,
	}
}

func (r *ServiceRepository) Create(ctx context.Context, service *models.Service) error {
	specJSON, err := json.Marshal(service.Spec)
	if err != nil {
		return err
	}

	metadataJSON, err := json.Marshal(service.Metadata)
	if err != nil {
		return err
	}

	query := `INSERT INTO services (
        id, name, version, spec, status, metadata, created_at, updated_at, last_heartbeat
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = r.db.Exec(ctx, query,
		service.ID, service.Name, service.Version, specJSON,
		service.Status, metadataJSON, service.CreatedAt, service.UpdatedAt,
		service.LastHeartbeat)

	if err != nil {
		return err
	}

	// Cache in Redis
	cacheKey := fmt.Sprintf("service:%s:%s", service.Name, service.Version)
	serviceJSON, _ := json.Marshal(service)
	r.redis.Set(ctx, cacheKey, serviceJSON, 5*time.Minute)

	return nil
}

func (r *ServiceRepository) GetByName(ctx context.Context, name string) (*models.Service, error) {
	// Try Redis first
	cacheKey := fmt.Sprintf("service:%s", name)
	cached, err := r.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var service models.Service
		if err := json.Unmarshal([]byte(cached), &service); err == nil {
			return &service, nil
		}
	}

	// Fallback to PostgreSQL
	query := `SELECT id, name, version, spec, status, metadata, created_at, updated_at, last_heartbeat 
              FROM services WHERE name = $1 AND status != 'inactive'`

	var service models.Service
	var specJSON []byte
	var metadataJSON []byte

	err = r.db.QueryRow(ctx, query, name).Scan(
		&service.ID, &service.Name, &service.Version, &specJSON,
		&service.Status, &metadataJSON, &service.CreatedAt,
		&service.UpdatedAt, &service.LastHeartbeat)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Parse JSON fields
	if err := json.Unmarshal(specJSON, &service.Spec); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(metadataJSON, &service.Metadata); err != nil {
		return nil, err
	}

	// Cache in Redis
	cachedJSON, _ := json.Marshal(service)
	r.redis.Set(ctx, cacheKey, cachedJSON, 5*time.Minute)

	return &service, nil
}

func (r *ServiceRepository) List(ctx context.Context) ([]*models.Service, error) {
	query := `SELECT id, name, version, spec, status, metadata, created_at, updated_at, last_heartbeat 
              FROM services WHERE status != 'inactive'`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []*models.Service
	for rows.Next() {
		var service models.Service
		var specJSON []byte
		var metadataJSON []byte

		err := rows.Scan(
			&service.ID, &service.Name, &service.Version, &specJSON,
			&service.Status, &metadataJSON, &service.CreatedAt,
			&service.UpdatedAt, &service.LastHeartbeat)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(specJSON, &service.Spec); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(metadataJSON, &service.Metadata); err != nil {
			return nil, err
		}

		services = append(services, &service)
	}

	return services, nil
}

func (r *ServiceRepository) Update(ctx context.Context, service *models.Service) error {
	specJSON, err := json.Marshal(service.Spec)
	if err != nil {
		return err
	}

	metadataJSON, err := json.Marshal(service.Metadata)
	if err != nil {
		return err
	}

	query := `UPDATE services SET 
        version = $1, spec = $2, status = $3, metadata = $4, 
        updated_at = $5, last_heartbeat = $6
        WHERE id = $7`

	_, err = r.db.Exec(ctx, query,
		service.Version, specJSON, service.Status, metadataJSON,
		time.Now(), service.LastHeartbeat, service.ID)

	if err != nil {
		return err
	}

	// Invalidate cache
	cacheKey := fmt.Sprintf("service:%s", service.Name)
	r.redis.Del(ctx, cacheKey)

	return nil
}

func (r *ServiceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE services SET status = 'inactive', updated_at = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, time.Now(), id)
	return err
}

func (r *ServiceRepository) UpdateHeartbeat(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE services SET last_heartbeat = $1, updated_at = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, time.Now(), id)
	return err
}

func (r *ServiceRepository) FindUnhealthy(ctx context.Context, timeout time.Duration) ([]uuid.UUID, error) {
	threshold := time.Now().Add(-timeout)

	query := `SELECT id FROM services WHERE last_heartbeat < $1 AND status != 'inactive'`

	rows, err := r.db.Query(ctx, query, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}
