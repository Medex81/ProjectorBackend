// services/identity-provider/test/benchmark/auth_benchmark_test.go
package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/models"
	"github.com/Medex81/ProjectorBackend/services/identity-provider/internal/service"
)

func BenchmarkAuthService_Register(b *testing.B) {
	// Setup
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:           "benchmark-secret",
			TokenExpiration:     24 * time.Hour,
			VerificationCodeTTL: 10 * time.Minute,
		},
	}

	// Use mocks for heavy operations
	mockRepo := NewMockUserRepository()
	mockKafka := NewMockKafkaProducer()
	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)

	service := service.NewAuthService(mockRepo, mockKafka, jwtManager, cfg)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &models.RegisterRequest{
			Email:    "benchmark@example.com",
			Password: "Benchmark123!",
			AppID:    "benchmark-app",
		}
		service.Register(ctx, req)
	}
}

func BenchmarkAuthService_Login(b *testing.B) {
	// Setup with pre-created user
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:       "benchmark-secret",
			TokenExpiration: 24 * time.Hour,
		},
	}

	mockRepo := NewMockUserRepositoryWithUser()
	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)

	service := service.NewAuthService(mockRepo, nil, jwtManager, cfg)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &models.LoginRequest{
			Email:    "benchmark@example.com",
			Password: "Benchmark123!",
			AppID:    "benchmark-app",
		}
		service.Login(ctx, req)
	}
}

func BenchmarkJWT_Generate(b *testing.B) {
	manager := auth.NewJWTManager("benchmark-secret", 24*time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.Generate("user-id", "email@example.com", "app-id")
	}
}

func BenchmarkJWT_Verify(b *testing.B) {
	manager := auth.NewJWTManager("benchmark-secret", 24*time.Hour)
	token, _ := manager.Generate("user-id", "email@example.com", "app-id")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.Verify(token)
	}
}

func BenchmarkParallelAuth(b *testing.B) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:       "benchmark-secret",
			TokenExpiration: 24 * time.Hour,
		},
	}

	jwtManager := auth.NewJWTManager(cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			token, _ := jwtManager.Generate("user-id", "email@example.com", "app-id")
			jwtManager.Verify(token)
		}
	})
}
