package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MessageRequest struct {
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

type APIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type messageStore interface {
	SaveMessage(ctx context.Context, messageID string, payload json.RawMessage) (storeResult, error)
}

type storeResult struct {
	ID       int64
	Inserted bool
}

type postgresMessageStore struct {
	dbPool *pgxpool.Pool
}

func main() {
	logger := log.New(os.Stdout, "[microservice-2] ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)
	httpPort := getEnv("HTTP_PORT", "8082")

	databaseURL := getEnv("DATABASE_URL", "postgres://appuser:apppassword@localhost:5432/reliable_delivery?sslmode=disable")
	ctx := context.Background()

	dbPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		logger.Fatalf("failed to create database pool: %v", err)
	}
	defer dbPool.Close()

	if err := dbPool.Ping(ctx); err != nil {
		logger.Fatalf("failed to connect to database: %v", err)
	}

	logger.Println("connected to database successfully")

	store := postgresMessageStore{dbPool: dbPool}
	mux := newMux(store, logger)

	server := &http.Server{
		Addr:         ":" + httpPort,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Printf("microservice-2 started on port %s", httpPort)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	waitForShutdown(server, logger)
}

func newMux(store messageStore, logger *log.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, APIResponse{
			Status:  "success",
			Message: "microservice-2 is healthy",
		})
	})

	mux.HandleFunc("POST /api/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		handleReceiveMessage(w, r, store, logger)
	})

	return mux
}

func handleReceiveMessage(w http.ResponseWriter, r *http.Request, store messageStore, logger *log.Logger) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, APIResponse{
			Status:  "error",
			Message: "method not allowed",
		})
		return
	}

	defer r.Body.Close()

	var request MessageRequest

	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&request); err != nil {
		logger.Printf("invalid request body: %v", err)

		writeJSON(w, http.StatusBadRequest, APIResponse{
			Status:  "error",
			Message: "invalid JSON request body",
		})
		return
	}

	request.MessageID = strings.TrimSpace(request.MessageID)

	if request.MessageID == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Status:  "error",
			Message: "message_id is required",
		})
		return
	}

	if len(request.Payload) == 0 {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Status:  "error",
			Message: "payload is required",
		})
		return
	}

	result, err := store.SaveMessage(r.Context(), request.MessageID, request.Payload)
	if err != nil {
		logger.Printf("failed to save message: message_id=%s error=%v", request.MessageID, err)

		writeJSON(w, http.StatusInternalServerError, APIResponse{
			Status:  "error",
			Message: "failed to save message",
		})
		return
	}

	if !result.Inserted {
		logger.Printf("duplicate message ignored safely: message_id=%s", request.MessageID)

		writeJSON(w, http.StatusOK, APIResponse{
			Status:  "success",
			Message: "message already processed",
		})
		return
	}

	logger.Printf("message saved successfully: id=%d message_id=%s", result.ID, request.MessageID)

	writeJSON(w, http.StatusOK, APIResponse{
		Status:  "success",
		Message: "message received successfully",
	})
}

func (s postgresMessageStore) SaveMessage(ctx context.Context, messageID string, payload json.RawMessage) (storeResult, error) {
	const query = `
		INSERT INTO received_messages (message_id, payload)
		VALUES ($1, $2::jsonb)
		ON CONFLICT (message_id) DO NOTHING
		RETURNING id;
	`

	var savedMessageID int64

	err := s.dbPool.QueryRow(
		ctx,
		query,
		messageID,
		string(payload),
	).Scan(&savedMessageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return storeResult{Inserted: false}, nil
	}
	if err != nil {
		return storeResult{}, err
	}

	return storeResult{
		ID:       savedMessageID,
		Inserted: true,
	}, nil
}

func writeJSON(w http.ResponseWriter, statusCode int, response APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to write JSON response: %v", err)
	}
}

func waitForShutdown(server *http.Server, logger *log.Logger) {
	stop := make(chan os.Signal, 1)

	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop

	logger.Println("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Printf("server shutdown failed: %v", err)
		return
	}

	logger.Println("server stopped gracefully")
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}
