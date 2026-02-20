// services/mail-service/internal/service/mail_service_test.go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/kafka"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/gomail.v2"
)

type mockDialer struct {
	sendFunc func(msg *gomail.Message) error
}

func (m *mockDialer) DialAndSend(msg *gomail.Message) error {
	return m.sendFunc(msg)
}

func TestMailService_HandleVerificationEmail(t *testing.T) {
	cfg := &config.Config{
		Mail: config.MailConfig{
			SMTPHost:     "smtp.gmail.com",
			SMTPPort:     587,
			SMTPUser:     "test@gmail.com",
			SMTPPassword: "password",
			FromEmail:    "noreply@test.com",
		},
	}

	t.Run("Send verification email successfully", func(t *testing.T) {
		sent := false
		mockDialer := &mockDialer{
			sendFunc: func(msg *gomail.Message) error {
				sent = true

				// Verify email content
				assert.Equal(t, cfg.Mail.FromEmail, msg.GetHeader("From")[0])
				assert.Equal(t, "test@example.com", msg.GetHeader("To")[0])
				assert.Equal(t, "Email Verification Code", msg.GetHeader("Subject")[0])

				return nil
			},
		}

		service := &MailService{
			config: cfg,
			dialer: mockDialer,
		}

		msgData := map[string]interface{}{
			"email":  "test@example.com",
			"code":   "123456",
			"app_id": "test-app",
		}
		data, err := json.Marshal(msgData)
		require.NoError(t, err)

		kafkaMsg := kafka.Message{
			ID:        "test-id",
			Type:      "email_verification",
			Source:    "test",
			Timestamp: time.Now(),
			Data:      data,
		}

		err = service.handleMessage(context.Background(), kafkaMsg)
		assert.NoError(t, err)
		assert.True(t, sent)
	})

	t.Run("Handle unknown message type", func(t *testing.T) {
		mockDialer := &mockDialer{
			sendFunc: func(msg *gomail.Message) error {
				t.Error("Should not send email for unknown type")
				return nil
			},
		}

		service := &MailService{
			config: cfg,
			dialer: mockDialer,
		}

		kafkaMsg := kafka.Message{
			ID:        "test-id",
			Type:      "unknown_type",
			Source:    "test",
			Timestamp: time.Now(),
			Data:      []byte("{}"),
		}

		err := service.handleMessage(context.Background(), kafkaMsg)
		assert.NoError(t, err) // Should ignore unknown type
	})

	t.Run("Invalid message data", func(t *testing.T) {
		mockDialer := &mockDialer{
			sendFunc: func(msg *gomail.Message) error {
				t.Error("Should not send email with invalid data")
				return nil
			},
		}

		service := &MailService{
			config: cfg,
			dialer: mockDialer,
		}

		kafkaMsg := kafka.Message{
			ID:        "test-id",
			Type:      "email_verification",
			Source:    "test",
			Timestamp: time.Now(),
			Data:      []byte("invalid json"),
		}

		err := service.handleMessage(context.Background(), kafkaMsg)
		assert.Error(t, err)
	})

	t.Run("SMTP server error", func(t *testing.T) {
		mockDialer := &mockDialer{
			sendFunc: func(msg *gomail.Message) error {
				return fmt.Errorf("SMTP connection failed")
			},
		}

		service := &MailService{
			config: cfg,
			dialer: mockDialer,
		}

		msgData := map[string]interface{}{
			"email": "test@example.com",
			"code":  "123456",
		}
		data, err := json.Marshal(msgData)
		require.NoError(t, err)

		kafkaMsg := kafka.Message{
			ID:        "test-id",
			Type:      "email_verification",
			Source:    "test",
			Timestamp: time.Now(),
			Data:      data,
		}

		err = service.handleMessage(context.Background(), kafkaMsg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SMTP connection failed")
	})

	t.Run("Missing required fields", func(t *testing.T) {
		mockDialer := &mockDialer{
			sendFunc: func(msg *gomail.Message) error {
				t.Error("Should not send email with missing fields")
				return nil
			},
		}

		service := &MailService{
			config: cfg,
			dialer: mockDialer,
		}

		// Missing email field
		msgData := map[string]interface{}{
			"code": "123456",
		}
		data, err := json.Marshal(msgData)
		require.NoError(t, err)

		kafkaMsg := kafka.Message{
			ID:        "test-id",
			Type:      "email_verification",
			Source:    "test",
			Timestamp: time.Now(),
			Data:      data,
		}

		err = service.handleMessage(context.Background(), kafkaMsg)
		assert.Error(t, err)
	})
}

func TestMailService_EmailTemplates(t *testing.T) {
	cfg := &config.Config{
		Mail: config.MailConfig{
			FromEmail: "noreply@test.com",
		},
	}

	t.Run("Verification email template", func(t *testing.T) {
		var capturedBody string
		mockDialer := &mockDialer{
			sendFunc: func(msg *gomail.Message) error {
				// Get HTML body
				for _, part := range msg.GetParts() {
					if part.GetHeader("Content-Type")[0] == "text/html" {
						capturedBody = string(part.GetBody())
						break
					}
				}
				return nil
			},
		}

		service := &MailService{
			config: cfg,
			dialer: mockDialer,
		}

		err := service.sendVerificationEmail(context.Background(), json.RawMessage(`{
            "email": "test@example.com",
            "code": "123456"
        }`))
		require.NoError(t, err)

		// Verify template content
		assert.Contains(t, capturedBody, "123456")
		assert.Contains(t, capturedBody, "Email Verification")
		assert.Contains(t, capturedBody, "10 minutes")
	})

	t.Run("Email with special characters", func(t *testing.T) {
		mockDialer := &mockDialer{
			sendFunc: func(msg *gomail.Message) error {
				// Verify headers are properly encoded
				to := msg.GetHeader("To")
				assert.Equal(t, "test+special@example.com", to[0])
				return nil
			},
		}

		service := &MailService{
			config: cfg,
			dialer: mockDialer,
		}

		err := service.sendVerificationEmail(context.Background(), json.RawMessage(`{
            "email": "test+special@example.com",
            "code": "123456"
        }`))
		assert.NoError(t, err)
	})
}

func TestMailService_ConcurrentProcessing(t *testing.T) {
	cfg := &config.Config{
		Mail: config.MailConfig{
			FromEmail: "noreply@test.com",
		},
	}

	sentCount := 0
	var mu sync.Mutex

	mockDialer := &mockDialer{
		sendFunc: func(msg *gomail.Message) error {
			mu.Lock()
			sentCount++
			mu.Unlock()
			return nil
		},
	}

	service := &MailService{
		config: cfg,
		dialer: mockDialer,
	}

	// Process multiple messages concurrently
	concurrency := 10
	messagesPerGoroutine := 10

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				msgData := map[string]interface{}{
					"email": "test@example.com",
					"code":  "123456",
				}
				data, _ := json.Marshal(msgData)

				kafkaMsg := kafka.Message{
					ID:        "test-id",
					Type:      "email_verification",
					Source:    "test",
					Timestamp: time.Now(),
					Data:      data,
				}

				service.handleMessage(context.Background(), kafkaMsg)
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, concurrency*messagesPerGoroutine, sentCount)
}
