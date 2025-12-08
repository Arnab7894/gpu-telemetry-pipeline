package queueservice

import (
	"encoding/json"
	"time"
)

// Message represents a message in the queue
type Message struct {
	ID            string                 `json:"id"`
	Topic         string                 `json:"topic"`
	Payload       json.RawMessage        `json:"payload"`
	Headers       map[string]string      `json:"headers"`
	Metadata      map[string]interface{} `json:"metadata"`
	Timestamp     time.Time              `json:"timestamp"`
	DeliveryCount int                    `json:"delivery_count"`
}

// PendingEntry represents a message being processed
type PendingEntry struct {
	Message         *Message
	ConsumerID      string
	ConsumerGroup   string
	DeliveryTime    time.Time
	VisibilityUntil time.Time
	DeliveryCount   int
}
