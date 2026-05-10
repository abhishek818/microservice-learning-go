package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"microservice-go/internal/config"
	"microservice-go/internal/consumer"
	"microservice-go/internal/observability"
)

func main() {
	logger := observability.NewLogger("microservice-1")

	serviceConfig, err := config.LoadMicroservice1()
	if err != nil {
		logger.Error("invalid configuration", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info(
		"starting microservice-1",
		slog.String("kafka_broker", serviceConfig.KafkaBroker),
		slog.String("kafka_topic", serviceConfig.KafkaTopic),
		slog.String("kafka_group_id", serviceConfig.KafkaGroupID),
		slog.String("microservice2_url", serviceConfig.Microservice2URL),
		slog.Duration("retry_interval", serviceConfig.RetryInterval),
		slog.Duration("http_timeout", serviceConfig.HTTPTimeout),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	consumerService := consumer.NewService(serviceConfig, logger)

	if err := consumerService.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("microservice-1 stopped with error", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("microservice-1 stopped gracefully")
}
