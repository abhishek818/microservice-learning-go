package model

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	UnknownMessageID = "unknown"
)

type QueueMessage struct {
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

type DeliveryMetadata struct {
	Topic             string
	Partition         int
	Offset            int64
	ProducerTimestamp time.Time
}

type MessageRecord struct {
	MessageID string
	Payload   json.RawMessage
	Metadata  DeliveryMetadata
}

func DecodeQueueMessage(messageBytes []byte) (QueueMessage, error) {
	var queueMessage QueueMessage
	if err := json.Unmarshal(messageBytes, &queueMessage); err != nil {
		return QueueMessage{}, err
	}

	queueMessage.MessageID = normalizeMessageID(queueMessage.MessageID)

	return queueMessage, nil
}

func ExtractMessageID(messageBytes []byte) string {
	queueMessage, err := DecodeQueueMessage(messageBytes)
	if err != nil {
		return UnknownMessageID
	}

	return queueMessage.MessageID
}

func normalizeMessageID(messageID string) string {
	normalizedMessageID := strings.TrimSpace(messageID)
	if normalizedMessageID == "" {
		return UnknownMessageID
	}

	return normalizedMessageID
}
