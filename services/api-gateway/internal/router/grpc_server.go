// services/api-gateway/internal/router/grpc_server.go
package router

import (
	"context"
	"net"

	"github.com/Medex81/ProjectorBackend/pkg/common/auth"
	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type GRPCServer struct {
	server     *grpc.Server
	config     *config.Config
	jwtManager *auth.JWTManager
}

func NewGRPCServer(cfg *config.Config, jwtManager *auth.JWTManager) *GRPCServer {
	// Create interceptors
	authInterceptor := NewAuthInterceptor(jwtManager)
	loggingInterceptor := NewLoggingInterceptor()
	metricsInterceptor := NewMetricsInterceptor()

	// Create gRPC server with interceptors
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingInterceptor.Unary(),
			metricsInterceptor.Unary(),
			authInterceptor.Unary(),
		),
		grpc.ChainStreamInterceptor(
			loggingInterceptor.Stream(),
			authInterceptor.Stream(),
		),
	)

	return &GRPCServer{
		server:     server,
		config:     cfg,
		jwtManager: jwtManager,
	}
}

func (s *GRPCServer) Serve(lis net.Listener) error {
	return s.server.Serve(lis)
}

func (s *GRPCServer) GracefulStop() {
	s.server.GracefulStop()
}

// AuthInterceptor
type AuthInterceptor struct {
	jwtManager *auth.JWTManager
}

func NewAuthInterceptor(jwtManager *auth.JWTManager) *AuthInterceptor {
	return &AuthInterceptor{jwtManager: jwtManager}
}

func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip auth for certain methods
		if skipAuth(info.FullMethod) {
			return handler(ctx, req)
		}

		// Extract token from metadata
		token, err := extractTokenFromMetadata(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "missing authentication token")
		}

		// Verify token
		claims, err := i.jwtManager.Verify(token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		// Add claims to context
		ctx = context.WithValue(ctx, "claims", claims)

		// Call handler
		return handler(ctx, req)
	}
}

func (i *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Extract token from metadata
		token, err := extractTokenFromMetadata(ss.Context())
		if err != nil {
			return status.Error(codes.Unauthenticated, "missing authentication token")
		}

		// Verify token
		claims, err := i.jwtManager.Verify(token)
		if err != nil {
			return status.Error(codes.Unauthenticated, "invalid token")
		}

		// Create wrapped stream with claims in context
		ctx := context.WithValue(ss.Context(), "claims", claims)
		wrapped := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		return handler(srv, wrapped)
	}
}

// LoggingInterceptor
type LoggingInterceptor struct{}

func NewLoggingInterceptor() *LoggingInterceptor {
	return &LoggingInterceptor{}
}

func (i *LoggingInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		logger.Info().Str("method", info.FullMethod).Msg("gRPC call started")
		resp, err := handler(ctx, req)
		if err != nil {
			logger.Error().Err(err).Str("method", info.FullMethod).Msg("gRPC call failed")
		} else {
			logger.Info().Str("method", info.FullMethod).Msg("gRPC call completed")
		}
		return resp, err
	}
}

func (i *LoggingInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		logger.Info().Str("method", info.FullMethod).Msg("gRPC stream started")
		err := handler(srv, ss)
		if err != nil {
			logger.Error().Err(err).Str("method", info.FullMethod).Msg("gRPC stream failed")
		} else {
			logger.Info().Str("method", info.FullMethod).Msg("gRPC stream completed")
		}
		return err
	}
}

// MetricsInterceptor
type MetricsInterceptor struct{}

func NewMetricsInterceptor() *MetricsInterceptor {
	return &MetricsInterceptor{}
}

func (i *MetricsInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		metrics.GRPCRequestsTotal.WithLabelValues(info.FullMethod, "started").Inc()
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)
		metrics.GRPCRequestDuration.WithLabelValues(info.FullMethod).Observe(duration.Seconds())

		if err != nil {
			metrics.GRPCRequestsTotal.WithLabelValues(info.FullMethod, "error").Inc()
		} else {
			metrics.GRPCRequestsTotal.WithLabelValues(info.FullMethod, "success").Inc()
		}

		return resp, err
	}
}

// Helper functions
func skipAuth(method string) bool {
	// List of methods that don't require authentication
	publicMethods := map[string]bool{
		"/auth.AuthService/Login":    true,
		"/auth.AuthService/Register": true,
		"/auth.AuthService/Verify":   true,
	}
	return publicMethods[method]
}

func extractTokenFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("missing metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", fmt.Errorf("missing authorization header")
	}

	// Remove "Bearer " prefix
	token := strings.TrimPrefix(values[0], "Bearer ")
	if token == "" {
		return "", fmt.Errorf("empty token")
	}

	return token, nil
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
