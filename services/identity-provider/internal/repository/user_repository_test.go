// services/identity-provider/internal/repository/user_repository_test.go
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-redis/redismock/v8"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRepository_CreateUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rdb, rmock := redismock.NewClientMock()

	repo := NewUserRepository(db, rdb)
	ctx := context.Background()

	t.Run("Create new user", func(t *testing.T) {
		email := "test@example.com"
		password := "hashedpassword"
		appID := "test-app"

		mock.ExpectExec("INSERT INTO users").
			WithArgs(sqlmock.AnyArg(), email, password, appID, false, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		user, err := repo.CreateUser(ctx, email, password, appID)
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, email, user.Email)
		assert.Equal(t, appID, user.AppID)
		assert.False(t, user.Verified)
	})

	t.Run("Create duplicate user", func(t *testing.T) {
		email := "duplicate@example.com"
		password := "hashedpassword"
		appID := "test-app"

		mock.ExpectExec("INSERT INTO users").
			WithArgs(sqlmock.AnyArg(), email, password, appID, false, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnError(fmt.Errorf("duplicate key violation"))

		user, err := repo.CreateUser(ctx, email, password, appID)
		assert.Error(t, err)
		assert.Nil(t, user)
	})
}

func TestUserRepository_GetUserByEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rdb, _ := redismock.NewClientMock()

	repo := NewUserRepository(db, rdb)
	ctx := context.Background()

	t.Run("Get existing user", func(t *testing.T) {
		userID := uuid.New()
		now := time.Now()

		rows := sqlmock.NewRows([]string{"id", "email", "password_hash", "app_id", "verified", "created_at", "updated_at", "last_login_at"}).
			AddRow(userID, "test@example.com", "hash", "test-app", true, now, now, now)

		mock.ExpectQuery("SELECT .+ FROM users WHERE email = .+ AND app_id = .+").
			WithArgs("test@example.com", "test-app").
			WillReturnRows(rows)

		user, err := repo.GetUserByEmail(ctx, "test@example.com", "test-app")
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, userID, user.ID)
		assert.Equal(t, "test@example.com", user.Email)
		assert.True(t, user.Verified)
	})

	t.Run("Get non-existent user", func(t *testing.T) {
		mock.ExpectQuery("SELECT .+ FROM users WHERE email = .+ AND app_id = .+").
			WithArgs("nonexistent@example.com", "test-app").
			WillReturnError(sql.ErrNoRows)

		user, err := repo.GetUserByEmail(ctx, "nonexistent@example.com", "test-app")
		assert.NoError(t, err)
		assert.Nil(t, user)
	})

	t.Run("Database error", func(t *testing.T) {
		mock.ExpectQuery("SELECT .+ FROM users WHERE email = .+ AND app_id = .+").
			WithArgs("test@example.com", "test-app").
			WillReturnError(fmt.Errorf("database connection error"))

		user, err := repo.GetUserByEmail(ctx, "test@example.com", "test-app")
		assert.Error(t, err)
		assert.Nil(t, user)
	})
}

func TestUserRepository_VerifyUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rdb, _ := redismock.NewClientMock()

	repo := NewUserRepository(db, rdb)
	ctx := context.Background()

	t.Run("Verify existing user", func(t *testing.T) {
		userID := uuid.New()

		mock.ExpectExec("UPDATE users SET verified = true, updated_at = .+ WHERE id = .+").
			WithArgs(sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.VerifyUser(ctx, userID)
		assert.NoError(t, err)
	})

	t.Run("Verify non-existent user", func(t *testing.T) {
		userID := uuid.New()

		mock.ExpectExec("UPDATE users SET verified = true, updated_at = .+ WHERE id = .+").
			WithArgs(sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.VerifyUser(ctx, userID)
		assert.Error(t, err)
	})
}

func TestUserRepository_CreateVerificationCode(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rdb, rmock := redismock.NewClientMock()

	repo := NewUserRepository(db, rdb)
	ctx := context.Background()

	t.Run("Create verification code", func(t *testing.T) {
		userID := uuid.New()
		email := "test@example.com"
		appID := "test-app"
		ttl := 10 * time.Minute

		rmock.ExpectSet("verification:"+email+":"+appID, sqlmock.AnyArg(), ttl).SetVal("OK")

		mock.ExpectExec("INSERT INTO verification_codes").
			WithArgs(sqlmock.AnyArg(), userID, sqlmock.AnyArg(), email, appID, sqlmock.AnyArg(), false, sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		code, err := repo.CreateVerificationCode(ctx, userID, email, appID, ttl)
		assert.NoError(t, err)
		assert.NotNil(t, code)
		assert.Equal(t, email, code.Email)
		assert.Equal(t, appID, code.AppID)
		assert.False(t, code.Used)
		assert.Len(t, code.Code, 6)
	})

	t.Run("Redis error", func(t *testing.T) {
		userID := uuid.New()
		email := "test@example.com"
		appID := "test-app"
		ttl := 10 * time.Minute

		rmock.ExpectSet("verification:"+email+":"+appID, sqlmock.AnyArg(), ttl).SetErr(fmt.Errorf("redis error"))

		code, err := repo.CreateVerificationCode(ctx, userID, email, appID, ttl)
		assert.Error(t, err)
		assert.Nil(t, code)
	})
}

func TestUserRepository_VerifyCode(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rdb, rmock := redismock.NewClientMock()

	repo := NewUserRepository(db, rdb)
	ctx := context.Background()

	t.Run("Valid code", func(t *testing.T) {
		email := "test@example.com"
		code := "123456"
		appID := "test-app"

		rmock.ExpectGet("verification:" + email + ":" + appID).SetVal(code)

		mock.ExpectExec("UPDATE verification_codes SET used = true WHERE email = .+ AND app_id = .+ AND code = .+").
			WithArgs(email, appID, code).
			WillReturnResult(sqlmock.NewResult(1, 1))

		rmock.ExpectDel("verification:" + email + ":" + appID).SetVal(1)

		valid, err := repo.VerifyCode(ctx, email, code, appID)
		assert.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("Invalid code", func(t *testing.T) {
		email := "test@example.com"
		code := "123456"
		appID := "test-app"

		rmock.ExpectGet("verification:" + email + ":" + appID).SetVal("654321")

		valid, err := repo.VerifyCode(ctx, email, code, appID)
		assert.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("Code not found", func(t *testing.T) {
		email := "test@example.com"
		code := "123456"
		appID := "test-app"

		rmock.ExpectGet("verification:" + email + ":" + appID).RedisNil()

		valid, err := repo.VerifyCode(ctx, email, code, appID)
		assert.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("Redis error", func(t *testing.T) {
		email := "test@example.com"
		code := "123456"
		appID := "test-app"

		rmock.ExpectGet("verification:" + email + ":" + appID).SetErr(fmt.Errorf("redis error"))

		valid, err := repo.VerifyCode(ctx, email, code, appID)
		assert.Error(t, err)
		assert.False(t, valid)
	})
}
