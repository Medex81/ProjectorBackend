// services/mail-service/test/integration/mail_integration_test.go
package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/kafka"
	"github.com/Medex81/ProjectorBackend/services/mail-service/internal/service"
	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMailService_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()

	// Setup Kafka container
	kafkaContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "confluentinc/cp-kafka:latest",
			ExposedPorts: []string{"9092/tcp"},
			Env: map[string]string{
				"KAFKA_BROKER_ID":                        "1",
				"KAFKA_ZOOKEEPER_CONNECT":                "zookeeper:2181",
				"KAFKA_ADVERTISED_LISTENERS":             "PLAINTEXT://localhost:9092",
				"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR": "1",
			},
			WaitingFor: wait.ForLog("started"),
		},
		Started: true,
	})
	require.NoError(t, err)
	defer kafkaContainer.Terminate(ctx)

	// Setup Zookeeper container
	zookeeperContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "confluentinc/cp-zookeeper:latest",
			ExposedPorts: []string{"2181/tcp"},
			Env: map[string]string{
				"ZOOKEEPER_CLIENT_PORT": "2181",
				"ZOOKEEPER_TICK_TIME":   "2000",
			},
			WaitingFor: wait.ForLog("binding to port"),
		},
		Started: true,
	})
	require.NoError(t, err)
	defer zookeeperContainer.Terminate(ctx)

	// Get Kafka port
	kafkaPort, err := kafkaContainer.MappedPort(ctx, "9092")
	require.NoError(t, err)

	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name: "mail-service-test",
		},
		Kafka: config.KafkaConfig{
			Brokers: []string{"localhost:" + kafkaPort.Port()},
			Topics: struct {
				EmailVerification string
				ServiceEvents     string
				Logs              string
			}{
				EmailVerification: "email-verification-test",
				ServiceEvents:     "service-events-test",
				Logs:              "logs-test",
			},
		},
		Mail: config.MailConfig{
			SMTPHost:     "smtp.gmail.com",
			SMTPPort:     587,
			SMTPUser:     "test@gmail.com",
			SMTPPassword: "test",
			FromEmail:    "test@test.com",
		},
	}

	// Create test email server
	emailServer := NewTestEmailServer()
	defer emailServer.Close()

	// Override SMTP settings for test
	cfg.Mail.SMTPHost = "localhost"
	cfg.Mail.SMTPPort = emailServer.Port()

	// Create mail service
	mailService, err := service.NewMailService(cfg)
	require.NoError(t, err)

	// Create Kafka producer
	producer := kafka.NewProducer(cfg.Kafka.Brokers)
	defer producer.Close()

	// Start mail service
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		mailService.Start(ctx)
	}()

	// Wait for service to start
	time.Sleep(2 * time.Second)

	t.Run("Send verification email", func(t *testing.T) {
		// Create message
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

		// Publish message
		err = producer.Publish(ctx, cfg.Kafka.Topics.EmailVerification, "test@example.com", kafkaMsg)
		require.NoError(t, err)

		// Wait for email to be sent
		time.Sleep(2 * time.Second)

		// Check if email was received
		emails := emailServer.GetEmails()
		assert.Len(t, emails, 1)
		assert.Equal(t, "test@example.com", emails[0].To)
		assert.Contains(t, emails[0].Body, "123456")
	})

	t.Run("Send multiple emails", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			msgData := map[string]interface{}{
				"email":  "test@example.com",
				"code":   "123456",
				"app_id": "test-app",
			}
			data, _ := json.Marshal(msgData)

			kafkaMsg := kafka.Message{
				ID:        "test-id",
				Type:      "email_verification",
				Source:    "test",
				Timestamp: time.Now(),
				Data:      data,
			}

			err := producer.Publish(ctx, cfg.Kafka.Topics.EmailVerification, "test@example.com", kafkaMsg)
			require.NoError(t, err)
		}

		time.Sleep(3 * time.Second)

		emails := emailServer.GetEmails()
		assert.Len(t, emails, 6) // 1 from previous test + 5 new
	})
}

type TestEmailServer struct {
	port   int
	emails []*ReceivedEmail
	server *smtp.Server
}

type ReceivedEmail struct {
	To      string
	From    string
	Subject string
	Body    string
}

func NewTestEmailServer() *TestEmailServer {
	s := &TestEmailServer{
		emails: []*ReceivedEmail{},
	}

	// Find free port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	s.port = listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create SMTP server
	be := &testBackend{server: s}
	s.server = smtp.NewServer(be)
	s.server.Addr = fmt.Sprintf(":%d", s.port)
	s.server.Domain = "localhost"
	s.server.ReadTimeout = 10 * time.Second
	s.server.WriteTimeout = 10 * time.Second

	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			log.Printf("Test email server error: %v", err)
		}
	}()

	return s
}

func (s *TestEmailServer) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

func (s *TestEmailServer) Port() int {
	return s.port
}

func (s *TestEmailServer) GetEmails() []*ReceivedEmail {
	return s.emails
}

type testBackend struct {
	server *TestEmailServer
}

func (b *testBackend) NewSession(_ smtp.ConnectionState) (smtp.Session, error) {
	return &testSession{server: b.server}, nil
}

type testSession struct {
	server *TestEmailServer
	from   string
	to     string
}

func (s *testSession) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	return nil
}

func (s *testSession) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.to = to
	return nil
}

func (s *testSession) Data(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	// Parse email
	msg, _ := mail.ReadMessage(bytes.NewReader(data))
	subject := msg.Header.Get("Subject")

	body, _ := io.ReadAll(msg.Body)

	s.server.emails = append(s.server.emails, &ReceivedEmail{
		To:      s.to,
		From:    s.from,
		Subject: subject,
		Body:    string(body),
	})

	return nil
}

func (s *testSession) Reset() {}

func (s *testSession) Logout() error {
	return nil
}
