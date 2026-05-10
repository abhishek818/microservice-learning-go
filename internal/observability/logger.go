package observability

import (
	"log/slog"
	"os"
)

func NewLogger(serviceName string) *slog.Logger {
	return slog.New(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}),
	).With("service", serviceName)
}
