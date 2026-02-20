// pkg/common/kafka/kafka.go
package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/Medex81/ProjectorBackend/pkg/common/metrics"
	"github.com/Medex81/ProjectorBackend/pkg/common/tracing"
	"github.com/segmentio/kafka-go"
)

type Message struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
	TraceID   string          `json:"trace_id"`
}

type Producer struct {
	writer *kafka.Writer
}

func NewProducer(brokers []string) *Producer {
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}

	return &Producer{writer: w}
}

func (p *Producer) Publish(ctx context.Context, topic string, key string, msg Message) error {
	ctx, span := tracing.StartSpan(ctx, "kafka.Publish")
	defer span.End()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	err = p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
		Time:  msg.Timestamp,
	})

	if err == nil {
		metrics.KafkaMessagesProduced.WithLabelValues(topic).Inc()
		logger.Info().Str("topic", topic).Str("key", key).Msg("Message published to Kafka")
	}

	return err
}

func (p *Producer) Close() error {
	return p.writer.Close()
}

type Consumer struct {
	reader *kafka.Reader
}

func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})

	return &Consumer{reader: r}
}

func (c *Consumer) Consume(ctx context.Context, handler func(context.Context, Message) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msg, err := c.reader.ReadMessage(ctx)
			if err != nil {
				logger.Error().Err(err).Msg("Error reading message from Kafka")
				continue
			}

			ctx, span := tracing.StartSpan(ctx, "kafka.Consume")

			var kafkaMsg Message
			if err := json.Unmarshal(msg.Value, &kafkaMsg); err != nil {
				logger.Error().Err(err).Msg("Error unmarshaling Kafka message")
				span.End()
				continue
			}

			metrics.KafkaMessagesConsumed.WithLabelValues(msg.Topic).Inc()
			logger.Info().Str("topic", msg.Topic).Str("partition", string(rune(msg.Partition))).Msg("Message consumed from Kafka")

			if err := handler(ctx, kafkaMsg); err != nil {
				logger.Error().Err(err).Msg("Error handling Kafka message")
			}

			span.End()
		}
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
