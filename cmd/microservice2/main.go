package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"microservice-go/internal/api"
	"microservice-go/internal/config"
	"microservice-go/internal/observability"
	"microservice-go/internal/store"
)

func main() {
	logger := observability.NewLogger("microservice-2")

	serviceConfig := config.LoadMicroservice2()
	logger.Info(
		"starting microservice-2",
		slog.String("http_port", serviceConfig.HTTPPort),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	messageStore, err := store.NewPostgresStore(ctx, serviceConfig.DatabaseURL)
	if err != nil {
		logger.Error("failed to create message store", slog.Any("error", err))
		os.Exit(1)
	}
	defer messageStore.Close()

	if err := messageStore.Ping(ctx); err != nil {
		logger.Error("failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}

	messageServer := api.NewMessageServer(messageStore, logger).HTTPServer(serviceConfig.HTTPPort)

	if err := api.RunHTTPServer(ctx, messageServer, logger, "microservice-2-api"); err != nil {
		logger.Error("microservice-2 stopped with error", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("microservice-2 stopped gracefully")
}
