// services/mail-service/internal/service/mail_service.go
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/kafka"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"gopkg.in/gomail.v2"
)

type MailService struct {
	kafkaConsumer *kafka.Consumer
	config        *config.Config
	dialer        *gomail.Dialer
}

func NewMailService(cfg *config.Config) (*MailService, error) {
	dialer := gomail.NewDialer(
		cfg.Mail.SMTPHost,
		cfg.Mail.SMTPPort,
		cfg.Mail.SMTPUser,
		cfg.Mail.SMTPPassword,
	)

	return &MailService{
		config: cfg,
		dialer: dialer,
	}, nil
}

func (s *MailService) Start(ctx context.Context) error {
	// Initialize Kafka consumer
	s.kafkaConsumer = kafka.NewConsumer(
		s.config.Kafka.Brokers,
		s.config.Kafka.Topics.EmailVerification,
		"mail-service",
	)

	logger.Info().Msg("Starting mail service consumer")

	// Start consuming messages
	return s.kafkaConsumer.Consume(ctx, s.handleMessage)
}

func (s *MailService) handleMessage(ctx context.Context, msg kafka.Message) error {
	ctx, span := tracing.StartSpan(ctx, "mail.handleMessage")
	defer span.End()

	switch msg.Type {
	case "email_verification":
		return s.sendVerificationEmail(ctx, msg.Data)
	default:
		logger.Warn().Str("type", msg.Type).Msg("Unknown message type")
		return nil
	}
}

func (s *MailService) sendVerificationEmail(ctx context.Context, data json.RawMessage) error {
	var emailData struct {
		Email string `json:"email"`
		Code  string `json:"code"`
		AppID string `json:"app_id"`
	}

	if err := json.Unmarshal(data, &emailData); err != nil {
		return err
	}

	logger.Info().Str("email", emailData.Email).Msg("Sending verification email")

	m := gomail.NewMessage()
	m.SetHeader("From", s.config.Mail.FromEmail)
	m.SetHeader("To", emailData.Email)
	m.SetHeader("Subject", "Email Verification Code")
	m.SetBody("text/html", fmt.Sprintf(`
        <h2>Email Verification</h2>
        <p>Your verification code is:</p>
        <h1 style="font-size: 32px; letter-spacing: 5px; background: #f0f0f0; padding: 10px; text-align: center;">%s</h1>
        <p>This code will expire in 10 minutes.</p>
        <p>If you didn't request this, please ignore this email.</p>
    `, emailData.Code))

	if err := s.dialer.DialAndSend(m); err != nil {
		logger.Error().Err(err).Str("email", emailData.Email).Msg("Failed to send email")
		return err
	}

	logger.Info().Str("email", emailData.Email).Msg("Verification email sent")
	return nil
}

func (s *MailService) Stop() error {
	if s.kafkaConsumer != nil {
		return s.kafkaConsumer.Close()
	}
	return nil
}
