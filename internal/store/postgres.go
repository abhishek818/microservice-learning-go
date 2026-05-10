package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"microservice-go/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SaveResult struct {
	ID             int64
	Inserted       bool
	DuplicateCount int64
}

type MessageStore interface {
	SaveMessage(ctx context.Context, record model.MessageRecord) (SaveResult, error)
	Ping(ctx context.Context) error
	Close()
}

type PostgresStore struct {
	dbPool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	dbPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create database pool: %w", err)
	}

	return &PostgresStore{
		dbPool: dbPool,
	}, nil
}

func (s *PostgresStore) SaveMessage(ctx context.Context, record model.MessageRecord) (SaveResult, error) {
	const query = `
		INSERT INTO received_messages (
			message_id,
			payload,
			kafka_topic,
			kafka_partition,
			kafka_offset,
			producer_timestamp,
			last_seen_at
		)
		VALUES ($1, $2::jsonb, $3, $4, $5, $6, NOW())
		ON CONFLICT (message_id) DO UPDATE
		SET
			last_seen_at = NOW(),
			duplicate_count = received_messages.duplicate_count + 1
		RETURNING id, duplicate_count = 0 AS inserted, duplicate_count;
	`

	var payloadValue string
	if record.Payload != nil {
		payloadValue = string(record.Payload)
	} else {
		payloadValue = string(json.RawMessage(`{}`))
	}

	var result SaveResult

	err := s.dbPool.QueryRow(
		ctx,
		query,
		record.MessageID,
		payloadValue,
		record.Metadata.Topic,
		record.Metadata.Partition,
		record.Metadata.Offset,
		nullableTimestamp(record.Metadata.ProducerTimestamp),
	).Scan(&result.ID, &result.Inserted, &result.DuplicateCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return SaveResult{}, fmt.Errorf("save message returned no rows")
	}
	if err != nil {
		return SaveResult{}, err
	}

	return result, nil
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.dbPool.Ping(ctx)
}

func (s *PostgresStore) Close() {
	s.dbPool.Close()
}

func nullableTimestamp(timestampValue time.Time) any {
	if timestampValue.IsZero() {
		return nil
	}

	return timestampValue.UTC()
}
