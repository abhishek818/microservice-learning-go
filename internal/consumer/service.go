package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"microservice-go/internal/config"
	"microservice-go/internal/delivery"
	"microservice-go/internal/model"

	"github.com/segmentio/kafka-go"
)

type kafkaReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, messages ...kafka.Message) error
	Close() error
}

type Service struct {
	reader         kafkaReader
	deliveryClient delivery.Client
	logger         *slog.Logger
	retryInterval  time.Duration
}

func NewService(serviceConfig config.Microservice1, logger *slog.Logger) *Service {
	return &Service{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:     []string{serviceConfig.KafkaBroker},
			Topic:       serviceConfig.KafkaTopic,
			GroupID:     serviceConfig.KafkaGroupID,
			MinBytes:    1,
			MaxBytes:    10e6,
			StartOffset: kafka.FirstOffset,
		}),
		deliveryClient: delivery.NewHTTPClient(serviceConfig.Microservice2URL, serviceConfig.HTTPTimeout),
		logger:         logger,
		retryInterval:  serviceConfig.RetryInterval,
	}
}

func (s *Service) Run(ctx context.Context) error {
	defer func() {
		if err := s.reader.Close(); err != nil {
			s.logger.Error("failed to close kafka reader", slog.Any("error", err))
		}
	}()

	s.logger.Info("consumer started")

	for {
		message, err := s.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				s.logger.Info("shutdown signal received, stopping consumer")
				return nil
			}

			s.logger.Error("failed to fetch message from kafka", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}

		messageID := model.ExtractMessageID(message.Value)
		metadata := model.DeliveryMetadata{
			Topic:             message.Topic,
			Partition:         message.Partition,
			Offset:            message.Offset,
			ProducerTimestamp: message.Time,
		}

		messageLogger := s.logger.With(
			"message_id", messageID,
			"topic", message.Topic,
			"partition", message.Partition,
			"offset", message.Offset,
		)
		messageLogger.Info("message consumed from kafka")

		delivered := s.deliverWithUnlimitedRetry(
			ctx,
			message.Value,
			metadata,
			messageLogger,
		)
		if !delivered {
			messageLogger.Info("message was not committed because service is shutting down")
			return nil
		}

		if err := s.reader.CommitMessages(ctx, message); err != nil {
			messageLogger.Error("failed to commit kafka offset", slog.Any("error", err))
			return err
		}

		messageLogger.Info("message delivered and kafka offset committed successfully")
	}
}

func (s *Service) deliverWithUnlimitedRetry(
	ctx context.Context,
	messageBody []byte,
	metadata model.DeliveryMetadata,
	logger *slog.Logger,
) bool {
	attempt := 1
	blockedStartedAt := time.Now()

	for {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		logger.Info("delivery attempt started", slog.Int("attempt", attempt))

		err := s.deliveryClient.Deliver(ctx, messageBody, metadata)
		if err == nil {
			logger.Info("delivery successful", slog.Int("attempt", attempt), slog.Duration("delivery_latency", time.Since(blockedStartedAt)))
			return true
		}

		logger.Warn(
			"delivery failed",
			slog.Int("attempt", attempt),
			slog.Duration("retry_after", s.retryInterval),
			slog.Any("error", err),
		)

		attempt++

		select {
		case <-ctx.Done():
			return false
		case <-time.After(s.retryInterval):
		}
	}
}
