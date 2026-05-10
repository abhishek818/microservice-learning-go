# microservice-learning-go
reliable message delivery

# Reliable API Delivery with Unlimited Retry in Golang

## 1. Overview

This project implements a reliable message delivery system using Golang.

- Microservice-1 consumes messages from a FIFO queue.
- Microservice-1 sends each message to Microservice-2 using a POST API.
- If the API call fails, Microservice-1 retries every 10 seconds.
- Retry is unlimited.
- Messages should not be lost.
- Microservice-2 exposes a POST API to receive and persist messages.


## 2. Tech Stack

- Golang
- Kafka-compatible broker using Redpanda
- PostgreSQL
- Docker Compose
- kafka-go
- pgx PostgreSQL driver

## 3. Architecture

```text
Producer
   |
   v
Kafka / Redpanda Topic
   |
   v
Microservice-1
Queue Consumer + Retry Worker
   |
   | HTTP POST
   v
Microservice-2
Receiver API
   |
   v
PostgreSQL
```

## 4. Services

### Producer

Producer is a small utility service used to publish test messages to Kafka.

Example message:

```json
{
  "message_id": "msg-001",
  "payload": {
    "customer_id": 1,
    "amount": 100,
    "event_type": "payment_created"
  }
}
```

### Microservice-1

Microservice-1 is responsible for:

- Consuming messages from Kafka.
- Calling Microservice-2 POST API.
- Retrying failed API calls every 10 seconds.
- Retrying forever until delivery succeeds.
- Committing Kafka offset only after successful delivery.

### Microservice-2

Microservice-2 is responsible for:

- Exposing POST API.
- Receiving messages.
- Storing messages in PostgreSQL.
- Handling duplicate messages safely using `message_id`.

## 5. Important Reliability Design

The most important design decision is:

```text
Kafka offset is committed only after Microservice-2 successfully receives the message.
```

Flow:

```text
Fetch message from Kafka
        |
        v
POST message to Microservice-2
        |
        +-- Success
        |      |
        |      v
        |   Commit Kafka offset
        |
        +-- Failure
               |
               v
          Wait 10 seconds
               |
               v
          Retry same message
```

Because offset is not committed before successful delivery, the message is not lost even if Microservice-1 crashes.

## 6. Retry Policy

Retry policy:

```text
Retry interval: 10 seconds
Retry count: unlimited
```

Microservice-1 retries the same message until Microservice-2 returns a successful 2xx response.

If Microservice-2 is down, Microservice-1 keeps retrying.

If Microservice-2 comes back online, the next retry succeeds and the Kafka offset is committed.

## 7. Ordering Guarantee

Microservice-1 processes one message at a time.

It does not move to the next message until the current message has been delivered successfully.

This preserves FIFO-style processing.

Example:

```text
msg-001 fails
msg-001 is retried until success
msg-002 waits
msg-003 waits
```

This is intentional because continuing with later messages while an earlier message is failing can break ordering.

## 8. Idempotency

Kafka-based delivery with manual offset commit gives at-least-once delivery.

There is one possible duplicate scenario:

```text
Microservice-2 receives and saves the message.
Microservice-1 crashes before committing Kafka offset.
Microservice-1 restarts.
Same message is consumed again.
```

To handle this safely, Microservice-2 uses `message_id` as a unique key.

Database table:

```sql
CREATE TABLE IF NOT EXISTS received_messages (
    id BIGSERIAL PRIMARY KEY,
    message_id VARCHAR(100) NOT NULL UNIQUE,
    payload JSONB NOT NULL,
    kafka_topic TEXT NOT NULL DEFAULT '',
    kafka_partition INTEGER NOT NULL DEFAULT 0,
    kafka_offset BIGINT NOT NULL DEFAULT 0,
    producer_timestamp TIMESTAMPTZ,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    duplicate_count BIGINT NOT NULL DEFAULT 0
);
```

If the same `message_id` comes again, Microservice-2 does not insert duplicate data. It updates `last_seen_at`, increments `duplicate_count`, and returns success response.

This makes the system:

```text
At-least-once delivery
Effectively-once persistence
```

## 9. Project Structure

```text
reliable-delivery-go/
│
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── go.sum
│
├── db/
│   └── init.sql
│
├── internal/
│   ├── api/
│   ├── config/
│   ├── consumer/
│   ├── delivery/
│   ├── model/
│   ├── observability/
│   └── store/
│
└── cmd/
    ├── microservice1/
    │   └── main.go
    ├── microservice2/
    │   └── main.go
    └── producer/
        └── main.go
```

## 10. How to Run

### Step 1: Start infrastructure and services

```bash
docker compose up -d --build postgres kafka microservice2 microservice1
```

### Step 2: Check running containers

```bash
docker ps
```

Expected containers:

```text
postgres
kafka
microservice2
microservice1
```

### Step 3: Run producer

```bash
docker compose --profile test run --rm producer
```

Producer publishes test messages to Kafka.

### Step 4: Check Microservice-1 logs

```bash
docker compose logs -f microservice1
```

Expected logs:

```text
message consumed from kafka
delivery attempt started
delivery successful
message delivered and kafka offset committed successfully
```

### Step 5: Check Microservice-2 logs

```bash
docker compose logs -f microservice2
```

Expected logs:

```text
message saved successfully
```

## 11. Verify Data in PostgreSQL

Open PostgreSQL shell:

```bash
docker exec -it reliable_postgres psql -U appuser -d reliable_delivery
```

Run:

```sql
SELECT id, message_id, kafka_topic, kafka_partition, kafka_offset, duplicate_count, received_at, last_seen_at
FROM received_messages
ORDER BY id;
```

Count:

```sql
SELECT COUNT(*) FROM received_messages;
```

Expected count after one producer run:

```text
5
```

## 12. Retry Test

### Step 1: Stop Microservice-2

```bash
docker compose stop microservice2
```

### Step 2: Run producer

```bash
docker compose --profile test run --rm producer
```

### Step 3: Watch Microservice-1 logs

```bash
docker compose logs -f microservice1
```

Expected logs:

```text
delivery attempt started: message_id=msg-001 attempt=1
delivery failed: message_id=msg-001 attempt=1
delivery attempt started: message_id=msg-001 attempt=2
delivery failed: message_id=msg-001 attempt=2
```

Microservice-1 will retry every 10 seconds.

### Step 4: Start Microservice-2 again

```bash
docker compose start microservice2
```

Expected Microservice-1 logs:

```text
delivery successful
message delivered and kafka offset committed successfully
```

This proves that messages are not lost when Microservice-2 is down.

## 13. Duplicate Message Test

Run producer twice:

```bash
docker compose --profile test run --rm producer
docker compose --profile test run --rm producer
```

Since producer uses same message IDs:

```text
msg-001
msg-002
msg-003
msg-004
msg-005
```

Microservice-2 will ignore duplicates safely.

Check DB count:

```sql
SELECT COUNT(*) FROM received_messages;
```

Expected:

```text
5
```

This proves idempotency.

## 14. Environment Variables

### Microservice-1

| Variable | Description | Default |
|---|---|---|
| KAFKA_BROKER | Kafka broker address | kafka:9092 |
| KAFKA_TOPIC | Kafka topic name | reliable-messages |
| KAFKA_GROUP_ID | Kafka consumer group ID | reliable-delivery-group |
| MICROSERVICE2_URL | Microservice-2 API URL | http://microservice2:8082/api/v1/messages |
| RETRY_INTERVAL_SECONDS | Retry interval | 10 |
| HTTP_TIMEOUT_SECONDS | HTTP client timeout | 5 |

### Microservice-2

| Variable | Description | Default |
|---|---|---|
| HTTP_PORT | API server port | 8082 |
| DATABASE_URL | PostgreSQL connection URL | postgres://appuser:apppassword@postgres:5432/reliable_delivery?sslmode=disable |

### Producer

| Variable | Description | Default |
|---|---|---|
| KAFKA_BROKER | Kafka broker address | kafka:9092 |
| KAFKA_TOPIC | Kafka topic name | reliable-messages |
| MESSAGE_COUNT | Number of test messages | 5 |

Integer-based settings are validated at startup.

- `RETRY_INTERVAL_SECONDS` must be a positive integer.
- `HTTP_TIMEOUT_SECONDS` must be a positive integer.
- `MESSAGE_COUNT` must be a positive integer.

If any of these values are invalid, the service exits immediately with a configuration error instead of silently changing behavior.

## 15. API Details

### Health Check

```http
GET /health
```

Response:

```json
{
  "status": "success",
  "message": "microservice-2 is healthy"
}
```

### Receive Message

```http
POST /api/v1/messages
Content-Type: application/json
```

Request:

```json
{
  "message_id": "msg-001",
  "payload": {
    "customer_id": 1,
    "amount": 100
  }
}
```

Success response:

```json
{
  "status": "success",
  "message": "message received successfully"
}
```

Duplicate response:

```json
{
  "status": "success",
  "message": "message already processed"
}
```

## 16. Failure Handling

Microservice-1 retries when:

- Microservice-2 is down.
- Network connection fails.
- HTTP request times out.
- Microservice-2 returns non-2xx status code.

Microservice-1 does not commit Kafka offset until the message is successfully delivered.

Microservice-1 is a background worker and does not expose a separate admin HTTP port.

## 17. Delivery Semantics

This system provides:

```text
At-least-once delivery from Kafka
Effectively-once persistence in PostgreSQL
```

At-least-once delivery means the same message may be delivered more than once in crash scenarios.

Effectively-once persistence is achieved because Microservice-2 stores messages using unique `message_id`.

## 18. Why Not Commit Offset Before API Success?

If offset is committed before API success, this can happen:

```text
Message consumed from Kafka
Offset committed
Microservice-2 API fails
Microservice-1 crashes
Message is lost
```

Therefore, offset is committed only after Microservice-2 successfully receives the message.

## 19. Future Improvements

For production usage, the following can be added:

- Dead-letter queue for permanently invalid messages.
- Exponential backoff.
- Authentication between services.
- Schema validation.
- More partitions for higher throughput.


## 20. Summary

This project implements reliable API delivery using Golang.

Key points:

- Microservice-1 consumes messages from Kafka.
- Microservice-1 calls Microservice-2 using POST API.
- Failed API calls are retried every 10 seconds.
- Retry is unlimited.
- Kafka offset is committed only after successful API delivery.
- Microservice-2 is idempotent using `message_id`.
- PostgreSQL stores successfully received messages.
- The system prevents message loss and safely handles duplicates.
