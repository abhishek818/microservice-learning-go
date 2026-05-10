package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Microservice1 struct {
	KafkaBroker      string
	KafkaTopic       string
	KafkaGroupID     string
	Microservice2URL string
	RetryInterval    time.Duration
	HTTPTimeout      time.Duration
}

type Microservice2 struct {
	HTTPPort    string
	DatabaseURL string
}

type Producer struct {
	KafkaBroker  string
	KafkaTopic   string
	MessageCount int
}

func LoadMicroservice1() (Microservice1, error) {
	retryIntervalSeconds, err := intFromEnv("RETRY_INTERVAL_SECONDS", 10, 1)
	if err != nil {
		return Microservice1{}, err
	}

	httpTimeoutSeconds, err := intFromEnv("HTTP_TIMEOUT_SECONDS", 5, 1)
	if err != nil {
		return Microservice1{}, err
	}

	return Microservice1{
		KafkaBroker:      stringFromEnv("KAFKA_BROKER", "localhost:9092"),
		KafkaTopic:       stringFromEnv("KAFKA_TOPIC", "reliable-messages"),
		KafkaGroupID:     stringFromEnv("KAFKA_GROUP_ID", "reliable-delivery-group"),
		Microservice2URL: stringFromEnv("MICROSERVICE2_URL", "http://localhost:8082/api/v1/messages"),
		RetryInterval:    time.Duration(retryIntervalSeconds) * time.Second,
		HTTPTimeout:      time.Duration(httpTimeoutSeconds) * time.Second,
	}, nil
}

func LoadMicroservice2() Microservice2 {
	return Microservice2{
		HTTPPort:    stringFromEnv("HTTP_PORT", "8082"),
		DatabaseURL: stringFromEnv("DATABASE_URL", "postgres://appuser:apppassword@localhost:5432/reliable_delivery?sslmode=disable"),
	}
}

func LoadProducer() (Producer, error) {
	messageCount, err := intFromEnv("MESSAGE_COUNT", 5, 1)
	if err != nil {
		return Producer{}, err
	}

	return Producer{
		KafkaBroker:  stringFromEnv("KAFKA_BROKER", "localhost:9092"),
		KafkaTopic:   stringFromEnv("KAFKA_TOPIC", "reliable-messages"),
		MessageCount: messageCount,
	}, nil
}

func stringFromEnv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func intFromEnv(key string, fallback int, min int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsedValue, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}

	if parsedValue < min {
		return 0, fmt.Errorf("%s must be >= %d", key, min)
	}

	return parsedValue, nil
}
