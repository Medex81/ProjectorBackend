// pkg/common/kafka/kafka_test.go
package kafka

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/IBM/sarama/mocks"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestKafkaProducer_Publish(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true

	mockProducer := mocks.NewSyncProducer(t, config)
	mockProducer.ExpectSendMessageAndSucceed()

	producer := &Producer{
		syncProducer: mockProducer,
	}

	ctx := context.Background()
	msg := Message{
		ID:        uuid.New().String(),
		Type:      "test-event",
		Source:    "test-service",
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"key":"value"}`),
	}

	t.Run("Publish successfully", func(t *testing.T) {
		err := producer.Publish(ctx, "test-topic", "key", msg)
		assert.NoError(t, err)
	})

	t.Run("Publish with tracing", func(t *testing.T) {
		ctx, span := tracing.StartSpan(ctx, "test")
		defer span.End()

		err := producer.Publish(ctx, "test-topic", "key", msg)
		assert.NoError(t, err)
	})
}

func TestKafkaProducer_PublishErrors(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true

	t.Run("Broker unavailable", func(t *testing.T) {
		producer := NewProducer([]string{"localhost:9999"})
		defer producer.Close()

		ctx := context.Background()
		msg := Message{
			ID:   uuid.New().String(),
			Type: "test",
		}

		err := producer.Publish(ctx, "test-topic", "key", msg)
		assert.Error(t, err)
	})

	t.Run("Invalid message", func(t *testing.T) {
		mockProducer := mocks.NewSyncProducer(t, sarama.NewConfig())
		producer := &Producer{syncProducer: mockProducer}

		ctx := context.Background()
		msg := Message{
			ID:   uuid.New().String(),
			Type: "test",
			Data: json.RawMessage(`invalid json`),
		}

		err := producer.Publish(ctx, "test-topic", "key", msg)
		assert.Error(t, err)
	})
}

func TestKafkaConsumer_Consume(t *testing.T) {
	mockConsumer := mocks.NewConsumer(t, nil)
	mockPartitionConsumer := mockConsumer.ExpectConsumePartition("test-topic", 0, sarama.OffsetNewest)

	// Mock messages
	msgData, _ := json.Marshal(Message{
		ID:        uuid.New().String(),
		Type:      "test",
		Timestamp: time.Now(),
	})

	mockPartitionConsumer.ExpectMessagesDrainedOnClose()
	mockPartitionConsumer.YieldMessage(&sarama.ConsumerMessage{
		Topic: "test-topic",
		Value: msgData,
	})

	consumer := &Consumer{
		consumer: mockConsumer,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	t.Run("Consume message", func(t *testing.T) {
		handlerCalled := false
		handler := func(ctx context.Context, msg Message) error {
			handlerCalled = true
			return nil
		}

		err := consumer.Consume(ctx, handler)
		assert.Error(t, err) // Context timeout
		assert.True(t, handlerCalled)
	})
}

func TestKafkaConsumer_HandlerError(t *testing.T) {
	mockConsumer := mocks.NewConsumer(t, nil)
	mockPartitionConsumer := mockConsumer.ExpectConsumePartition("test-topic", 0, sarama.OffsetNewest)

	msgData, _ := json.Marshal(Message{
		ID: uuid.New().String(),
	})

	mockPartitionConsumer.YieldMessage(&sarama.ConsumerMessage{
		Topic: "test-topic",
		Value: msgData,
	})

	consumer := &Consumer{
		consumer: mockConsumer,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	handler := func(ctx context.Context, msg Message) error {
		return assert.AnError // Simulate handler error
	}

	err := consumer.Consume(ctx, handler)
	assert.Error(t, err)
}
