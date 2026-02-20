// services/api-gateway/internal/middleware/auth.go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/metrics"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
)

type contextKey string

const (
	UserContextKey contextKey = "user"
)

func AuthMiddleware(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := tracing.StartSpan(r.Context(), "middleware.auth")
			defer span.End()

			// Extract token from header
			token := extractTokenFromHeader(r)
			if token == "" {
				// Try to extract from query parameter for WebSocket
				token = r.URL.Query().Get("token")
			}

			if token == "" {
				metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "401").Inc()
				http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
				return
			}

			// Verify token
			claims, err := jwtManager.Verify(token)
			if err != nil {
				logger.Warn().Err(err).Str("token", token[:10]+"...").Msg("Invalid token")
				metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "401").Inc()
				http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
				return
			}

			// Add claims to context
			ctx = context.WithValue(ctx, UserContextKey, claims)
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "200").Inc()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractTokenFromHeader(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}

	return parts[1]
}

func GetUserFromContext(ctx context.Context) (*auth.Claims, bool) {
	user, ok := ctx.Value(UserContextKey).(*auth.Claims)
	return user, ok
}

func RequireAuth(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return AuthMiddleware(jwtManager)(next)
	}
}

func OptionalAuth(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractTokenFromHeader(r)
			if token != "" {
				if claims, err := jwtManager.Verify(token); err == nil {
					ctx := context.WithValue(r.Context(), UserContextKey, claims)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
