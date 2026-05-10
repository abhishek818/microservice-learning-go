package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"microservice-go/internal/delivery"
	"microservice-go/internal/model"
	"microservice-go/internal/store"
)

type MessageServer struct {
	store  store.MessageStore
	logger *slog.Logger
}

func NewMessageServer(store store.MessageStore, logger *slog.Logger) *MessageServer {
	return &MessageServer{
		store:  store,
		logger: logger,
	}
}

func (s *MessageServer) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "microservice-2 is healthy",
		})
	})

	mux.HandleFunc("POST /api/v1/messages", s.handleReceiveMessage)

	return mux
}

func (s *MessageServer) HTTPServer(httpPort string) *http.Server {
	return &http.Server{
		Addr:         ":" + httpPort,
		Handler:      s.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func (s *MessageServer) handleReceiveMessage(w http.ResponseWriter, r *http.Request) {
	requestLogger := s.logger

	defer r.Body.Close()

	var request model.QueueMessage
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		requestLogger.Warn("invalid request body", slog.Any("error", err))

		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "invalid JSON request body",
		})
		return
	}

	request.MessageID = strings.TrimSpace(request.MessageID)
	if request.MessageID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "message_id is required",
		})
		return
	}

	if len(request.Payload) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "payload is required",
		})
		return
	}

	metadata, err := delivery.MetadataFromHeaders(r.Header)
	if err != nil {
		requestLogger.Warn("invalid delivery metadata headers", slog.String("message_id", request.MessageID), slog.Any("error", err))

		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "invalid delivery metadata headers",
		})
		return
	}

	result, err := s.store.SaveMessage(r.Context(), model.MessageRecord{
		MessageID: request.MessageID,
		Payload:   request.Payload,
		Metadata:  metadata,
	})
	if err != nil {
		requestLogger.Error("failed to save message", slog.String("message_id", request.MessageID), slog.Any("error", err))

		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"status":  "error",
			"message": "failed to save message",
		})
		return
	}

	if !result.Inserted {
		requestLogger.Info(
			"duplicate message observed",
			slog.String("message_id", request.MessageID),
			slog.Int64("duplicate_count", result.DuplicateCount),
			slog.String("topic", metadata.Topic),
			slog.Int("partition", metadata.Partition),
			slog.Int64("offset", metadata.Offset),
		)

		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "message already processed",
		})
		return
	}

	requestLogger.Info(
		"message saved successfully",
		slog.Int64("id", result.ID),
		slog.String("message_id", request.MessageID),
		slog.String("topic", metadata.Topic),
		slog.Int("partition", metadata.Partition),
		slog.Int64("offset", metadata.Offset),
	)

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "message received successfully",
	})
}

func RunHTTPServer(ctx context.Context, server *http.Server, logger *slog.Logger, serverName string) error {
	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("http server starting", slog.String("server", serverName), slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			return
		}

		serverErrors <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		logger.Info("http server shutting down", slog.String("server", serverName))
		return server.Shutdown(shutdownCtx)
	case err := <-serverErrors:
		return err
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("failed to write JSON response", slog.Any("error", err))
	}
}
