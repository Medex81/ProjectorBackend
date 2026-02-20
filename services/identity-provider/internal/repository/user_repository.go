// services/identity-provider/internal/repository/user_repository.go
package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/metrics"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/models"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type UserRepository struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewUserRepository(db *pgxpool.Pool, redis *redis.Client) *UserRepository {
	return &UserRepository{
		db:    db,
		redis: redis,
	}
}

func (r *UserRepository) CreateUser(ctx context.Context, email, password, appID string) (*models.User, error) {
	ctx, span := tracing.StartSpan(ctx, "repository.CreateUser")
	defer span.End()

	start := time.Now()
	defer func() {
		metrics.DatabaseQueryDuration.WithLabelValues("insert", "users").Observe(time.Since(start).Seconds())
		metrics.DatabaseQueriesTotal.WithLabelValues("insert", "users").Inc()
	}()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: string(passwordHash),
		AppID:        appID,
		Verified:     false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	query := `INSERT INTO users (id, email, password_hash, app_id, verified, created_at, updated_at) 
              VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err = r.db.Exec(ctx, query,
		user.ID, user.Email, user.PasswordHash, user.AppID, user.Verified,
		user.CreatedAt, user.UpdatedAt)

	if err != nil {
		logger.Error().Err(err).Str("email", email).Msg("Failed to create user")
		return nil, err
	}

	logger.Info().Str("user_id", user.ID.String()).Str("email", email).Msg("User created")
	return user, nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email, appID string) (*models.User, error) {
	ctx, span := tracing.StartSpan(ctx, "repository.GetUserByEmail")
	defer span.End()

	start := time.Now()
	defer func() {
		metrics.DatabaseQueryDuration.WithLabelValues("select", "users").Observe(time.Since(start).Seconds())
		metrics.DatabaseQueriesTotal.WithLabelValues("select", "users").Inc()
	}()

	var user models.User
	query := `SELECT id, email, password_hash, app_id, verified, created_at, updated_at, last_login_at 
              FROM users WHERE email = $1 AND app_id = $2`

	err := r.db.QueryRow(ctx, query, email, appID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.AppID, &user.Verified,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &user, nil
}

func (r *UserRepository) VerifyUser(ctx context.Context, userID uuid.UUID) error {
	ctx, span := tracing.StartSpan(ctx, "repository.VerifyUser")
	defer span.End()

	query := `UPDATE users SET verified = true, updated_at = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, time.Now(), userID)

	if err != nil {
		logger.Error().Err(err).Str("user_id", userID.String()).Msg("Failed to verify user")
		return err
	}

	logger.Info().Str("user_id", userID.String()).Msg("User verified")
	return nil
}

func (r *UserRepository) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE users SET last_login_at = $1, updated_at = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, time.Now(), userID)
	return err
}

func (r *UserRepository) CreateVerificationCode(ctx context.Context, userID uuid.UUID, email, appID string, ttl time.Duration) (*models.VerificationCode, error) {
	ctx, span := tracing.StartSpan(ctx, "repository.CreateVerificationCode")
	defer span.End()

	// Generate 6-digit code
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	code := fmt.Sprintf("%06d", int(bytes[0])<<16|int(bytes[1])<<8|int(bytes[2])%1000000)

	verification := &models.VerificationCode{
		ID:        uuid.New(),
		UserID:    userID,
		Code:      code,
		Email:     email,
		AppID:     appID,
		ExpiresAt: time.Now().Add(ttl),
		Used:      false,
		CreatedAt: time.Now(),
	}

	// Store in Redis
	key := fmt.Sprintf("verification:%s:%s", email, appID)
	if err := r.redis.Set(ctx, key, code, ttl).Err(); err != nil {
		return nil, err
	}

	// Store in PostgreSQL for history
	query := `INSERT INTO verification_codes (id, user_id, code, email, app_id, expires_at, used, created_at) 
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.db.Exec(ctx, query,
		verification.ID, verification.UserID, verification.Code, verification.Email,
		verification.AppID, verification.ExpiresAt, verification.Used, verification.CreatedAt)

	if err != nil {
		logger.Error().Err(err).Str("email", email).Msg("Failed to create verification code")
		return nil, err
	}

	logger.Info().Str("email", email).Str("code", code).Msg("Verification code created")
	return verification, nil
}

func (r *UserRepository) VerifyCode(ctx context.Context, email, code, appID string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "repository.VerifyCode")
	defer span.End()

	key := fmt.Sprintf("verification:%s:%s", email, appID)
	storedCode, err := r.redis.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}

	if storedCode == code {
		// Mark as used in PostgreSQL
		query := `UPDATE verification_codes SET used = true WHERE email = $1 AND app_id = $2 AND code = $3`
		_, err := r.db.Exec(ctx, query, email, appID, code)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to mark code as used")
		}

		// Delete from Redis
		r.redis.Del(ctx, key)
		return true, nil
	}

	return false, nil
}
