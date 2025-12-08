package mq

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Message represents a message in the queue
// It wraps telemetry events and other data that flows through the system
type Message struct {
	// ID is a unique identifier for this message
	ID string `json:"id"`

	// Topic is the routing key for subscribers
	// Examples: "telemetry.gpu", "telemetry.metrics", "gpu.discovery"
	Topic string `json:"topic"`

	// Payload contains the actual message data (JSON-encoded)
	// Using json.RawMessage to preserve JSON structure during marshal/unmarshal
	Payload json.RawMessage `json:"payload"`

	// Headers for metadata (e.g., source, version, compression)
	Headers map[string]string `json:"headers,omitempty"`

	// Timestamp when the message was created
	Timestamp time.Time `json:"timestamp"`

	// Metadata for additional context (e.g., retry count, processing time)
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewMessage creates a new message with the given topic and payload
// The payload is automatically marshaled to JSON
func NewMessage(topic string, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Message{
		ID:        uuid.New().String(),
		Topic:     topic,
		Payload:   data,
		Headers:   make(map[string]string),
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}, nil
}

// NewMessageWithID creates a message with a specific ID (useful for testing)
func NewMessageWithID(id, topic string, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Message{
		ID:        id,
		Topic:     topic,
		Payload:   data,
		Headers:   make(map[string]string),
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}, nil
}

// Unmarshal unmarshals the payload into the given interface
func (m *Message) Unmarshal(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}

// WithHeader adds a header to the message (fluent API)
func (m *Message) WithHeader(key, value string) *Message {
	m.Headers[key] = value
	return m
}

// WithMetadata adds metadata to the message (fluent API)
func (m *Message) WithMetadata(key string, value interface{}) *Message {
	m.Metadata[key] = value
	return m
}

// GetHeader returns a header value
func (m *Message) GetHeader(key string) (string, bool) {
	val, ok := m.Headers[key]
	return val, ok
}

// GetMetadata returns a metadata value
func (m *Message) GetMetadata(key string) (interface{}, bool) {
	val, ok := m.Metadata[key]
	return val, ok
}
