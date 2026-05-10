package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"microservice-go/internal/config"
	"microservice-go/internal/model"

	"github.com/segmentio/kafka-go"
)

func main() {
	logger := log.New(os.Stdout, "[producer] ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	producerConfig, err := config.LoadProducer()
	if err != nil {
		logger.Fatalf("invalid configuration: %v", err)
	}

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{producerConfig.KafkaBroker},
		Topic:    producerConfig.KafkaTopic,
		Balancer: &kafka.Hash{},
	})

	defer func() {
		if err := writer.Close(); err != nil {
			logger.Printf("failed to close kafka writer: %v", err)
		}
	}()

	logger.Printf("producer started. broker=%s topic=%s message_count=%d", producerConfig.KafkaBroker, producerConfig.KafkaTopic, producerConfig.MessageCount)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 1; i <= producerConfig.MessageCount; i++ {
		messageID := fmt.Sprintf("msg-%03d", i)

		payloadBytes, err := json.Marshal(map[string]interface{}{
			"customer_id": i,
			"amount":      i * 100,
			"event_type":  "payment_created",
			"created_at":  time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			logger.Fatalf("failed to marshal payload for message_id=%s error=%v", messageID, err)
		}

		queueMessage := model.QueueMessage{
			MessageID: messageID,
			Payload:   payloadBytes,
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
