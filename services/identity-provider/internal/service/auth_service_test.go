// services/identity-provider/internal/service/auth_service_test.go
package service

import (
	"context"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/kafka"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/models"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/repository"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthService_Register(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockUserRepository(ctrl)
	mockKafka := kafka.NewMockProducer(ctrl)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			VerificationCodeTTL: 10 * time.Minute,
		},
		Kafka: config.KafkaConfig{
			Topics: struct {
				EmailVerification string
				ServiceEvents     string
				Logs              string
			}{
				EmailVerification: "email-verification",
			},
		},
	}

	jwtManager := auth.NewJWTManager("test-secret", 24*time.Hour)
	service := NewAuthService(mockRepo, mockKafka, jwtManager, cfg)

	ctx := context.Background()

	t.Run("Register new user", func(t *testing.T) {
		req := &models.RegisterRequest{
			Email:    "new@example.com",
			Password: "password123",
			AppID:    "test-app",
		}

		// Mock: Check if user exists - returns nil (not found)
		mockRepo.EXPECT().
			GetUserByEmail(ctx, req.Email, req.AppID).
			Return(nil, nil)

		// Mock: Create user
		mockRepo.EXPECT().
			CreateUser(ctx, req.Email, req.Password, req.AppID).
			Return(&models.User{
				ID:    uuid.New(),
				Email: req.Email,
				AppID: req.AppID,
			}, nil)

		// Mock: Create verification code
		mockRepo.EXPECT().
			CreateVerificationCode(ctx, gomock.Any(), req.Email, req.AppID, cfg.Auth.VerificationCodeTTL).
			Return(&models.VerificationCode{
				Code: "123456",
			}, nil)

		// Mock: Publish to Kafka
		mockKafka.EXPECT().
			Publish(ctx, cfg.Kafka.Topics.EmailVerification, req.Email, gomock.Any()).
			Return(nil)

		err := service.Register(ctx, req)
		assert.NoError(t, err)
	})

	t.Run("Register existing verified user", func(t *testing.T) {
		req := &models.RegisterRequest{
			Email:    "existing@example.com",
			Password: "password123",
			AppID:    "test-app",
		}

		existingUser := &models.User{
			ID:       uuid.New(),
			Email:    req.Email,
			AppID:    req.AppID,
			Verified: true,
		}

		mockRepo.EXPECT().
			GetUserByEmail(ctx, req.Email, req.AppID).
			Return(existingUser, nil)

		err := service.Register(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists and verified")
	})

	t.Run("Register existing unverified user", func(t *testing.T) {
		req := &models.RegisterRequest{
			Email:    "unverified@example.com",
			Password: "password123",
			AppID:    "test-app",
		}

		existingUser := &models.User{
			ID:       uuid.New(),
			Email:    req.Email,
			AppID:    req.AppID,
			Verified: false,
		}

		mockRepo.EXPECT().
			GetUserByEmail(ctx, req.Email, req.AppID).
			Return(existingUser, nil)

		mockRepo.EXPECT().
			CreateVerificationCode(ctx, existingUser.ID, req.Email, req.AppID, cfg.Auth.VerificationCodeTTL).
			Return(&models.VerificationCode{
				Code: "123456",
			}, nil)

		mockKafka.EXPECT().
			Publish(ctx, cfg.Kafka.Topics.EmailVerification, req.Email, gomock.Any()).
			Return(nil)

		err := service.Register(ctx, req)
		assert.NoError(t, err)
	})

	t.Run("Register with invalid email", func(t *testing.T) {
		req := &models.RegisterRequest{
			Email:    "invalid-email",
			Password: "password123",
			AppID:    "test-app",
		}

		// Add email validation if needed
		err := service.Register(ctx, req)
		// Should fail validation
		assert.Error(t, err)
	})

	t.Run("Register with weak password", func(t *testing.T) {
		req := &models.RegisterRequest{
			Email:    "test@example.com",
			Password: "123", // Too short
			AppID:    "test-app",
		}

		err := service.Register(ctx, req)
		// Should fail password strength validation
		assert.Error(t, err)
	})
}

func TestAuthService_Verify(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockUserRepository(ctrl)
	mockKafka := kafka.NewMockProducer(ctrl)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:       "test-secret",
			TokenExpiration: 24 * time.Hour,
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)
	service := NewAuthService(mockRepo, mockKafka, jwtManager, cfg)

	ctx := context.Background()
	userID := uuid.New()

	t.Run("Verify with correct code", func(t *testing.T) {
		req := &models.VerifyRequest{
			Email: "test@example.com",
			Code:  "123456",
			AppID: "test-app",
		}

		mockRepo.EXPECT().
			VerifyCode(ctx, req.Email, req.Code, req.AppID).
			Return(true, nil)

		mockRepo.EXPECT().
			GetUserByEmail(ctx, req.Email, req.AppID).
			Return(&models.User{
				ID:       userID,
				Email:    req.Email,
				AppID:    req.AppID,
				Verified: false,
			}, nil)

		mockRepo.EXPECT().
			VerifyUser(ctx, userID).
			Return(nil)

		mockRepo.EXPECT().
			UpdateLastLogin(ctx, userID).
			Return(nil)

		token, err := service.Verify(ctx, req)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		// Verify token is valid
		claims, err := jwtManager.Verify(token)
		assert.NoError(t, err)
		assert.Equal(t, userID.String(), claims.UserID)
		assert.Equal(t, req.Email, claims.Email)
		assert.Equal(t, req.AppID, claims.AppID)
	})

	t.Run("Verify with incorrect code", func(t *testing.T) {
		req := &models.VerifyRequest{
			Email: "test@example.com",
			Code:  "wrong-code",
			AppID: "test-app",
		}

		mockRepo.EXPECT().
			VerifyCode(ctx, req.Email, req.Code, req.AppID).
			Return(false, nil)

		token, err := service.Verify(ctx, req)
		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "invalid verification code")
	})

	t.Run("Verify with expired code", func(t *testing.T) {
		req := &models.VerifyRequest{
			Email: "test@example.com",
			Code:  "123456",
			AppID: "test-app",
		}

		mockRepo.EXPECT().
			VerifyCode(ctx, req.Email, req.Code, req.AppID).
			Return(false, nil) // Code expired or not found

		token, err := service.Verify(ctx, req)
		assert.Error(t, err)
		assert.Empty(t, token)
	})

	t.Run("Verify non-existent user", func(t *testing.T) {
		req := &models.VerifyRequest{
			Email: "nonexistent@example.com",
			Code:  "123456",
			AppID: "test-app",
		}

		mockRepo.EXPECT().
			VerifyCode(ctx, req.Email, req.Code, req.AppID).
			Return(true, nil)

		mockRepo.EXPECT().
			GetUserByEmail(ctx, req.Email, req.AppID).
			Return(nil, nil)

		token, err := service.Verify(ctx, req)
		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "user not found")
	})

	t.Run("Verify already verified user", func(t *testing.T) {
		req := &models.VerifyRequest{
			Email: "verified@example.com",
			Code:  "123456",
			AppID: "test-app",
		}

		mockRepo.EXPECT().
			VerifyCode(ctx, req.Email, req.Code, req.AppID).
			Return(true, nil)

		mockRepo.EXPECT().
			GetUserByEmail(ctx, req.Email, req.AppID).
			Return(&models.User{
				ID:       userID,
				Email:    req.Email,
				AppID:    req.AppID,
				Verified: true, // Already verified
			}, nil)

		// Should not call VerifyUser again
		mockRepo.EXPECT().
			UpdateLastLogin(ctx, userID).
			Return(nil)

		token, err := service.Verify(ctx, req)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})
}

func TestAuthService_Login(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockUserRepository(ctrl)
	mockKafka := kafka.NewMockProducer(ctrl)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:       "test-secret",
			TokenExpiration: 24 * time.Hour,
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)
	service := NewAuthService(mockRepo, mockKafka, jwtManager, cfg)

	ctx := context.Background()
	userID := uuid.New()
	password := "correct-password"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	t.Run("Login with correct credentials", func(t *testing.T) {
		req := &models.LoginRequest{
			Email:    "test@example.com",
			Password: password,
			AppID:    "test-app",
		}

		mockRepo.EXPECT().
			GetUserByEmail(ctx, req.Email, req.AppID).
			Return(&models.User{
				ID:           userID,
				Email:        req.Email,
				AppID:        req.AppID,
				Verified:     true,
				PasswordHash: string(hashedPassword),
			}, nil)

		mockRepo.EXPECT().
			UpdateLastLogin(ctx, userID).
			Return(nil)

		token, err := service.Login(ctx, req)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("Login with wrong password", func(t *testing.T) {
		req := &models.LoginRequest{
			Email:    "test@example.com",
			Password: "wrong-password",
			AppID:    "test-app",
		}

		mockRepo.EXPECT().
			GetUserByEmail(ctx, req.Email, req.AppID).
			Return(&models.User{
				ID:           userID,
				Email:        req.Email,
				AppID:        req.AppID,
				Verified:     true,
				PasswordHash: string(hashedPassword),
			}, nil)

		token, err := service.Login(ctx, req)
		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("Login with unverified user", func(t *testing.T) {
		req := &models.LoginRequest{
			Email:    "unverified@example.com",
			Password: password,
			AppID:    "test-app",
		}

		mockRepo.EXPECT().
			GetUserByEmail(ctx, req.Email, req.AppID).
			Return(&models.User{
				ID:           userID,
				Email:        req.Email,
				AppID:        req.AppID,
				Verified:     false,
				PasswordHash: string(hashedPassword),
			}, nil)

		token, err := service.Login(ctx, req)
		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "email not verified")
	})

	t.Run("Login non-existent user", func(t *testing.T) {
		req := &models.LoginRequest{
			Email:    "nonexistent@example.com",
			Password: password,
			AppID:    "test-app",
		}

		mockRepo.EXPECT().
			GetUserByEmail(ctx, req.Email, req.AppID).
			Return(nil, nil)

		token, err := service.Login(ctx, req)
		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("Login with empty password", func(t *testing.T) {
		req := &models.LoginRequest{
			Email:    "test@example.com",
			Password: "",
			AppID:    "test-app",
		}

		token, err := service.Login(ctx, req)
		assert.Error(t, err)
		assert.Empty(t, token)
	})
}

func TestAuthService_ConcurrentOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockUserRepository(ctrl)
	mockKafka := kafka.NewMockProducer(ctrl)

	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:           "test-secret",
			TokenExpiration:     24 * time.Hour,
			VerificationCodeTTL: 10 * time.Minute,
		},
		Kafka: config.KafkaConfig{
			Topics: struct {
				EmailVerification string
				ServiceEvents     string
				Logs              string
			}{
				EmailVerification: "email-verification",
			},
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)
	service := NewAuthService(mockRepo, mockKafka, jwtManager, cfg)

	ctx := context.Background()
	email := "test@example.com"
	appID := "test-app"

	t.Run("Concurrent registrations same email", func(t *testing.T) {
		// First registration check
		mockRepo.EXPECT().
			GetUserByEmail(ctx, email, appID).
			Return(nil, nil).
			Times(1)

		// Create user
		mockRepo.EXPECT().
			CreateUser(ctx, email, gomock.Any(), appID).
			Return(&models.User{ID: uuid.New()}, nil).
			Times(1)

		// Create verification code
		mockRepo.EXPECT().
			CreateVerificationCode(ctx, gomock.Any(), email, appID, cfg.Auth.VerificationCodeTTL).
			Return(&models.VerificationCode{Code: "123456"}, nil).
			Times(1)

		// Kafka publish
		mockKafka.EXPECT().
			Publish(ctx, cfg.Kafka.Topics.EmailVerification, email, gomock.Any()).
			Return(nil).
			Times(1)

		// Second registration should fail because user now exists
		mockRepo.EXPECT().
			GetUserByEmail(ctx, email, appID).
			Return(&models.User{Verified: true}, nil).
			Times(1)

		// Run registrations concurrently
		errChan := make(chan error, 2)

		go func() {
			errChan <- service.Register(ctx, &models.RegisterRequest{
				Email:    email,
				Password: "password123",
				AppID:    appID,
			})
		}()

		go func() {
			errChan <- service.Register(ctx, &models.RegisterRequest{
				Email:    email,
				Password: "password456",
				AppID:    appID,
			})
		}()

		// One should succeed, one should fail
		err1 := <-errChan
		err2 := <-errChan

		assert.True(t, (err1 == nil && err2 != nil) || (err1 != nil && err2 == nil))
	})
}
