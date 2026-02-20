// services/identity-provider/internal/models/user.go
package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID  `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	AppID        string     `json:"app_id"`
	Verified     bool       `json:"verified"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

type VerificationCode struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Code      string    `json:"code"`
	Email     string    `json:"email"`
	AppID     string    `json:"app_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	AppID    string `json:"app_id"`
}

type VerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
	AppID string `json:"app_id"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	AppID    string `json:"app_id"`
}

type AuthResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	AppID     string    `json:"app_id"`
	Verified  bool      `json:"verified"`
}
