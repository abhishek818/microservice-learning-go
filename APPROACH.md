# Approach

## Goal

This project is designed to satisfy the following requirements:

- consume messages in order
- deliver them from `microservice1` to `microservice2` via HTTP `POST`
- retry every 10 seconds on failure
- retry forever until success
- avoid message loss


## Approach:

1. Keep the flow simple and explicit.
2. Make retry ownership unambiguous.
3. Commit queue progress only after downstream success.

I have kept `microservice1` as a single-message worker that blocks on the current message until delivery succeeds.

## Assumptions

- FIFO means preserving message order for the stream being consumed.
- Unlimited retry is a hard requirement, even if it causes head-of-line blocking.
- Preventing message loss is more important than maximizing throughput.
- Duplicate delivery is acceptable as long as persistence is idempotent.

## Main Design Choices

### 1. Queue progress is committed only after API success

This is the core reliability decision.

`microservice1` consumes a message, calls `microservice2`, and commits the Kafka offset only after a successful `2xx` response. If the HTTP call fails, the offset is not committed, so the message can be retried or re-consumed after restart.

Result:

- no lost messages due to premature acknowledgement
- at-least-once delivery semantics

### 2. Ordering is preserved by processing one message at a time

`microservice1` does not move to the next message while the current one is failing.

This introduces head-of-line blocking, but it keeps delivery order deterministic and makes the retry behavior consistent.

### 3. Idempotency is handled in `microservice2`

Even with correct offset handling, duplicates are still possible if:

- `microservice2` saves the message
- `microservice1` crashes before committing the offset

To handle that safely, `microservice2` persists messages with a unique `message_id`. Duplicate deliveries are treated as successful replays, not failures.

### 4. Keep service boundaries clear

The codebase is split so `main.go` mostly wires dependencies and the behavior lives in focused internal packages:

- `internal/consumer` for queue consumption and retry
- `internal/delivery` for HTTP delivery
- `internal/api` for the receiver API
- `internal/store` for PostgreSQL persistence
- `internal/config` and `internal/model` for shared contracts


## Data Model Choices

The receiver stores more than just the payload:

- `message_id`
- `payload`
- `kafka_topic`
- `kafka_partition`
- `kafka_offset`
- `producer_timestamp`
- `received_at`
- `last_seen_at`
- `duplicate_count`

This makes duplicate handling and operational debugging easier while keeping the schema simple.


## Tradeoffs

These tradeoffs are intentional:

- Throughput is lower because retries block later messages.
- Kafka is used as a practical stand-in for a FIFO queue; ordering is preserved by serialized consumption.
- The system provides at-least-once delivery, not exactly-once delivery.
- Idempotent persistence is used to make duplicate delivery safe.
