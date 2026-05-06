package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

type QueueMessage struct {
	MessageID string      `json:"message_id"`
	Payload   interface{} `json:"payload"`
}

func main() {
	logger := log.New(os.Stdout, "[producer] ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	kafkaBroker := getEnv("KAFKA_BROKER", "localhost:9092")
	kafkaTopic := getEnv("KAFKA_TOPIC", "reliable-messages")
	messageCount := getEnvAsInt("MESSAGE_COUNT", 5)

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{kafkaBroker},
		Topic:    kafkaTopic,
		Balancer: &kafka.Hash{},
	})

	defer func() {
		if err := writer.Close(); err != nil {
			logger.Printf("failed to close kafka writer: %v", err)
		}
	}()

	logger.Printf("producer started. broker=%s topic=%s message_count=%d", kafkaBroker, kafkaTopic, messageCount)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 1; i <= messageCount; i++ {
		messageID := fmt.Sprintf("msg-%03d", i)

		queueMessage := QueueMessage{
			MessageID: messageID,
			Payload: map[string]interface{}{
				"customer_id": i,
				"amount":      i * 100,
				"event_type":  "payment_created",
				"created_at":  time.Now().UTC().Format(time.RFC3339),
			},
		}

		messageBytes, err := json.Marshal(queueMessage)
		if err != nil {
			logger.Fatalf("failed to marshal message_id=%s error=%v", messageID, err)
		}

		kafkaMessage := kafka.Message{
			Key:   []byte(messageID),
			Value: messageBytes,
			Time:  time.Now(),
		}

		if err := writer.WriteMessages(ctx, kafkaMessage); err != nil {
			logger.Fatalf("failed to write message_id=%s to kafka: %v", messageID, err)
		}

		logger.Printf("message published successfully: message_id=%s payload=%s", messageID, string(messageBytes))
	}

	logger.Println("all messages published successfully")
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
