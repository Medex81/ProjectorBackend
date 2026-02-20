// services/identity-provider/internal/models/auth.go
package models

import (
	"time"
)

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	AppID    string `json:"app_id" validate:"required"`
}

type VerifyRequest struct {
	Email string `json:"email" validate:"required,email"`
	Code  string `json:"code" validate:"required,len=6"`
	AppID string `json:"app_id" validate:"required"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
	AppID    string `json:"app_id" validate:"required"`
}

type RefreshRequest struct {
	Token string `json:"token" validate:"required"`
}

type AuthResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	AppID     string    `json:"app_id"`
	Verified  bool      `json:"verified,omitempty"`
}

type TokenClaims struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	AppID     string    `json:"app_id"`
	ExpiresAt time.Time `json:"expires_at"`
	IssuedAt  time.Time `json:"issued_at"`
}

type PasswordResetRequest struct {
	Email string `json:"email" validate:"required,email"`
	AppID string `json:"app_id" validate:"required"`
}

type PasswordReset struct {
	Email       string `json:"email" validate:"required,email"`
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
	AppID       string `json:"app_id" validate:"required"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (r *RegisterRequest) Validate() []ValidationError {
	var errors []ValidationError

	if r.Email == "" {
		errors = append(errors, ValidationError{Field: "email", Message: "email is required"})
	}

	if r.Password == "" {
		errors = append(errors, ValidationError{Field: "password", Message: "password is required"})
	} else if len(r.Password) < 8 {
		errors = append(errors, ValidationError{Field: "password", Message: "password must be at least 8 characters"})
	}

	if r.AppID == "" {
		errors = append(errors, ValidationError{Field: "app_id", Message: "app_id is required"})
	}

	return errors
}

func (r *VerifyRequest) Validate() []ValidationError {
	var errors []ValidationError

	if r.Email == "" {
		errors = append(errors, ValidationError{Field: "email", Message: "email is required"})
	}

	if r.Code == "" {
		errors = append(errors, ValidationError{Field: "code", Message: "code is required"})
	} else if len(r.Code) != 6 {
		errors = append(errors, ValidationError{Field: "code", Message: "code must be 6 digits"})
	}

	if r.AppID == "" {
		errors = append(errors, ValidationError{Field: "app_id", Message: "app_id is required"})
	}

	return errors
}

func (r *LoginRequest) Validate() []ValidationError {
	var errors []ValidationError

	if r.Email == "" {
		errors = append(errors, ValidationError{Field: "email", Message: "email is required"})
	}

	if r.Password == "" {
		errors = append(errors, ValidationError{Field: "password", Message: "password is required"})
	}

	if r.AppID == "" {
		errors = append(errors, ValidationError{Field: "app_id", Message: "app_id is required"})
	}

	return errors
}
