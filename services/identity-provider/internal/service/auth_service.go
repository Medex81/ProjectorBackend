// services/identity-provider/internal/service/auth_service.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/kafka"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/models"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo      *repository.UserRepository
	kafkaProducer *kafka.Producer
	jwtManager    *auth.JWTManager
	config        *config.Config
}

func NewAuthService(
	userRepo *repository.UserRepository,
	kafkaProducer *kafka.Producer,
	jwtManager *auth.JWTManager,
	cfg *config.Config,
) *AuthService {
	return &AuthService{
		userRepo:      userRepo,
		kafkaProducer: kafkaProducer,
		jwtManager:    jwtManager,
		config:        cfg,
	}
}

func (s *AuthService) Register(ctx context.Context, req *models.RegisterRequest) error {
	ctx, span := tracing.StartSpan(ctx, "service.Register")
	defer span.End()

	// Check if user exists
	existingUser, err := s.userRepo.GetUserByEmail(ctx, req.Email, req.AppID)
	if err != nil {
		return err
	}

	if existingUser != nil {
		if existingUser.Verified {
			return errors.New("user already exists and verified")
		}
		// User exists but not verified, generate new code
		return s.sendVerificationCode(ctx, existingUser.ID, req.Email, req.AppID)
	}

	// Create new user
	user, err := s.userRepo.CreateUser(ctx, req.Email, req.Password, req.AppID)
	if err != nil {
		return err
	}

	// Send verification code
	return s.sendVerificationCode(ctx, user.ID, req.Email, req.AppID)
}

func (s *AuthService) sendVerificationCode(ctx context.Context, userID uuid.UUID, email, appID string) error {
	// Create verification code
	code, err := s.userRepo.CreateVerificationCode(ctx, userID, email, appID, s.config.Auth.VerificationCodeTTL)
	if err != nil {
		return err
	}

	// Send email via Kafka
	msgData := map[string]interface{}{
		"email":  email,
		"code":   code.Code,
		"app_id": appID,
	}

	data, err := json.Marshal(msgData)
	if err != nil {
		return err
	}

	kafkaMsg := kafka.Message{
		ID:        uuid.New().String(),
		Type:      "email_verification",
		Source:    s.config.Service.Name,
		Timestamp: time.Now(),
		Data:      data,
	}

	return s.kafkaProducer.Publish(ctx, s.config.Kafka.Topics.EmailVerification, email, kafkaMsg)
}

func (s *AuthService) Verify(ctx context.Context, req *models.VerifyRequest) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "service.Verify")
	defer span.End()

	// Verify code
	valid, err := s.userRepo.VerifyCode(ctx, req.Email, req.Code, req.AppID)
	if err != nil {
		return "", err
	}

	if !valid {
		return "", errors.New("invalid verification code")
	}

	// Get user
	user, err := s.userRepo.GetUserByEmail(ctx, req.Email, req.AppID)
	if err != nil {
		return "", err
	}

	if user == nil {
		return "", errors.New("user not found")
	}

	// Mark user as verified
	if !user.Verified {
		if err := s.userRepo.VerifyUser(ctx, user.ID); err != nil {
			return "", err
		}
	}

	// Generate JWT token
	token, err := s.jwtManager.Generate(user.ID.String(), user.Email, user.AppID)
	if err != nil {
		return "", err
	}

	// Update last login
	s.userRepo.UpdateLastLogin(ctx, user.ID)

	logger.Info().Str("user_id", user.ID.String()).Msg("User verified and logged in")
	return token, nil
}

func (s *AuthService) Login(ctx context.Context, req *models.LoginRequest) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "service.Login")
	defer span.End()

	// Get user
	user, err := s.userRepo.GetUserByEmail(ctx, req.Email, req.AppID)
	if err != nil {
		return "", err
	}

	if user == nil {
		return "", errors.New("invalid credentials")
	}

	if !user.Verified {
		return "", errors.New("email not verified")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return "", errors.New("invalid credentials")
	}

	// Generate token
	token, err := s.jwtManager.Generate(user.ID.String(), user.Email, user.AppID)
	if err != nil {
		return "", err
	}

	// Update last login
	s.userRepo.UpdateLastLogin(ctx, user.ID)

	logger.Info().Str("user_id", user.ID.String()).Msg("User logged in")
	return token, nil
}
