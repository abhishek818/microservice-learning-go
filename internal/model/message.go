package model

import "encoding/json"

type QueueMessage struct {
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}
