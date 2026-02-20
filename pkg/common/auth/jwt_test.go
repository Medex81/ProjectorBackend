// pkg/common/auth/jwt_test.go
package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTManager_Generate(t *testing.T) {
	tests := []struct {
		name        string
		secretKey   string
		expiry      time.Duration
		userID      string
		email       string
		appID       string
		expectError bool
	}{
		{
			name:        "Valid token generation",
			secretKey:   "test-secret-key",
			expiry:      24 * time.Hour,
			userID:      "user-123",
			email:       "test@example.com",
			appID:       "test-app",
			expectError: false,
		},
		{
			name:        "Empty secret key",
			secretKey:   "",
			expiry:      24 * time.Hour,
			userID:      "user-123",
			email:       "test@example.com",
			appID:       "test-app",
			expectError: true,
		},
		{
			name:        "Zero expiry",
			secretKey:   "test-secret-key",
			expiry:      0,
			userID:      "user-123",
			email:       "test@example.com",
			appID:       "test-app",
			expectError: false,
		},
		{
			name:        "Empty userID",
			secretKey:   "test-secret-key",
			expiry:      24 * time.Hour,
			userID:      "",
			email:       "test@example.com",
			appID:       "test-app",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewJWTManager(tt.secretKey, tt.expiry)

			token, err := manager.Generate(tt.userID, tt.email, tt.appID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, token)
			}
		})
	}
}

func TestJWTManager_Verify(t *testing.T) {
	secretKey := "test-secret-key"
	manager := NewJWTManager(secretKey, 24*time.Hour)

	// Generate valid token
	validToken, err := manager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	// Generate expired token
	expiredManager := NewJWTManager(secretKey, -1*time.Hour)
	expiredToken, err := expiredManager.Generate("user-123", "test@example.com", "test-app")
	require.NoError(t, err)

	tests := []struct {
		name        string
		token       string
		expectError bool
		validate    func(*testing.T, *Claims)
	}{
		{
			name:        "Valid token",
			token:       validToken,
			expectError: false,
			validate: func(t *testing.T, claims *Claims) {
				assert.Equal(t, "user-123", claims.UserID)
				assert.Equal(t, "test@example.com", claims.Email)
				assert.Equal(t, "test-app", claims.AppID)
				assert.NotEmpty(t, claims.ID)
				assert.NotEmpty(t, claims.ExpiresAt)
			},
		},
		{
			name:        "Expired token",
			token:       expiredToken,
			expectError: true,
		},
		{
			name:        "Malformed token",
			token:       "malformed.token.string",
			expectError: true,
		},
		{
			name:        "Empty token",
			token:       "",
			expectError: true,
		},
		{
			name:        "Token with wrong signature",
			token:       validToken + "tampered",
			expectError: true,
		},
		{
			name: "Token from different secret",
			token: func() string {
				differentManager := NewJWTManager("different-secret", 24*time.Hour)
				token, _ := differentManager.Generate("user-123", "test@example.com", "test-app")
				return token
			}(),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := manager.Verify(tt.token)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, claims)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, claims)
				if tt.validate != nil {
					tt.validate(t, claims)
				}
			}
		})
	}
}

func TestJWTManager_TokenClaims(t *testing.T) {
	manager := NewJWTManager("test-secret-key", 24*time.Hour)

	t.Run("Token contains all required claims", func(t *testing.T) {
		token, err := manager.Generate("user-123", "test@example.com", "test-app")
		require.NoError(t, err)

		parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte("test-secret-key"), nil
		})
		require.NoError(t, err)

		claims, ok := parsed.Claims.(*Claims)
		require.True(t, ok)

		// Verify standard claims
		assert.NotEmpty(t, claims.ID)
		assert.NotEmpty(t, claims.IssuedAt)
		assert.NotEmpty(t, claims.ExpiresAt)
		assert.Equal(t, "ProjectorBackend", claims.Issuer)

		// Verify custom claims
		assert.Equal(t, "user-123", claims.UserID)
		assert.Equal(t, "test@example.com", claims.Email)
		assert.Equal(t, "test-app", claims.AppID)
	})

	t.Run("Token expiration time is correct", func(t *testing.T) {
		shortManager := NewJWTManager("test-secret-key", 1*time.Hour)
		token, err := shortManager.Generate("user-123", "test@example.com", "test-app")
		require.NoError(t, err)

		claims, err := shortManager.Verify(token)
		require.NoError(t, err)

		expectedExpiry := time.Now().Add(1 * time.Hour)
		assert.WithinDuration(t, expectedExpiry, claims.ExpiresAt.Time, 1*time.Second)
	})
}

func TestJWTManager_Concurrent(t *testing.T) {
	manager := NewJWTManager("test-secret-key", 24*time.Hour)

	// Concurrent token generation and verification
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				userID := "user-123"
				email := "test@example.com"
				appID := "test-app"

				token, err := manager.Generate(userID, email, appID)
				assert.NoError(t, err)

				claims, err := manager.Verify(token)
				assert.NoError(t, err)
				assert.Equal(t, userID, claims.UserID)
				assert.Equal(t, email, claims.Email)
				assert.Equal(t, appID, claims.AppID)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
