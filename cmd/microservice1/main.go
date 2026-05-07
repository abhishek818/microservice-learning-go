package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"
)

type QueueMessage struct {
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

func main() {
	logger := log.New(os.Stdout, "[microservice-1] ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	kafkaBroker := getEnv("KAFKA_BROKER", "localhost:9092")
	kafkaTopic := getEnv("KAFKA_TOPIC", "reliable-messages")
	kafkaGroupID := getEnv("KAFKA_GROUP_ID", "reliable-delivery-group")
	microservice2URL := getEnv("MICROSERVICE2_URL", "http://localhost:8082/api/v1/messages")

	retryInterval := time.Duration(getEnvAsInt("RETRY_INTERVAL_SECONDS", 10)) * time.Second
	httpTimeout := time.Duration(getEnvAsInt("HTTP_TIMEOUT_SECONDS", 5)) * time.Second

	logger.Printf("starting microservice-1")
	logger.Printf("kafka_broker=%s", kafkaBroker)
	logger.Printf("kafka_topic=%s", kafkaTopic)
	logger.Printf("kafka_group_id=%s", kafkaGroupID)
	logger.Printf("microservice2_url=%s", microservice2URL)
	logger.Printf("retry_interval=%s", retryInterval)
	logger.Printf("http_timeout=%s", httpTimeout)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{kafkaBroker},
		Topic:       kafkaTopic,
		GroupID:     kafkaGroupID,
		MinBytes:    1,
		MaxBytes:    10e6,
		StartOffset: kafka.FirstOffset,
	})

	defer func() {
		if err := reader.Close(); err != nil {
			logger.Printf("failed to close kafka reader: %v", err)
		}
	}()

	httpClient := &http.Client{
		Timeout: httpTimeout,
	}

	logger.Println("microservice-1 started successfully and waiting for messages")

	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				logger.Println("shutdown signal received, stopping consumer")
				return
			}

			logger.Printf("failed to fetch message from kafka: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		messageID := extractMessageID(message.Value)

		logger.Printf(
			"message consumed from kafka: topic=%s partition=%d offset=%d message_id=%s",
			message.Topic,
			message.Partition,
			message.Offset,
			messageID,
		)

		delivered := deliverWithUnlimitedRetry(
			ctx,
			httpClient,
			microservice2URL,
			message.Value,
			messageID,
			retryInterval,
			logger,
		)

		if !delivered {
			logger.Printf("message was not committed because service is shutting down: message_id=%s", messageID)
			return
		}

		if err := reader.CommitMessages(ctx, message); err != nil {
			logger.Printf("failed to commit kafka offset: message_id=%s error=%v", messageID, err)
			return
		}

		logger.Printf(
			"message delivered and kafka offset committed successfully: message_id=%s partition=%d offset=%d",
			messageID,
			message.Partition,
			message.Offset,
		)
	}
}

func deliverWithUnlimitedRetry(
	ctx context.Context,
	httpClient *http.Client,
	url string,
	messageBody []byte,
	messageID string,
	retryInterval time.Duration,
	logger *log.Logger,
) bool {
	attempt := 1

	for {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		logger.Printf("delivery attempt started: message_id=%s attempt=%d", messageID, attempt)

		err := postMessage(ctx, httpClient, url, messageBody)

		if err == nil {
			logger.Printf("delivery successful: message_id=%s attempt=%d", messageID, attempt)
			return true
		}

		logger.Printf(
			"delivery failed: message_id=%s attempt=%d error=%v retry_after=%s",
			messageID,
			attempt,
			err,
			retryInterval,
		)

		attempt++

		select {
		case <-ctx.Done():
			return false
		case <-time.After(retryInterval):
		}
	}
}

func postMessage(
	ctx context.Context,
	httpClient *http.Client,
	url string,
	messageBody []byte,
) error {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewReader(messageBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}

	defer response.Body.Close()

	responseBody, _ := io.ReadAll(response.Body)

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf(
			"microservice-2 returned non-success status: status_code=%d response=%s",
			response.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	return nil
}

func extractMessageID(messageBytes []byte) string {
	var queueMessage QueueMessage

	if err := json.Unmarshal(messageBytes, &queueMessage); err != nil {
		return "unknown"
	}

	queueMessage.MessageID = strings.TrimSpace(queueMessage.MessageID)

	if queueMessage.MessageID == "" {
		return "unknown"
	}

	return queueMessage.MessageID
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getEnvAsInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsedValue, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsedValue
}
