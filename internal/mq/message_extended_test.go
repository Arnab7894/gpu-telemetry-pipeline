package mq

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMessage_Success(t *testing.T) {
	payload := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	}

	msg, err := NewMessage("test-topic", payload)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.NotEmpty(t, msg.ID)
	assert.Equal(t, "test-topic", msg.Topic)
	assert.NotNil(t, msg.Payload)
	assert.NotNil(t, msg.Headers)
	assert.NotNil(t, msg.Metadata)
	assert.WithinDuration(t, time.Now(), msg.Timestamp, 1*time.Second)
}

func TestNewMessage_WithComplexPayload(t *testing.T) {
	payload := struct {
		Name   string
		Age    int
		Active bool
		Tags   []string
		Nested map[string]interface{}
	}{
		Name:   "Test User",
		Age:    30,
		Active: true,
		Tags:   []string{"tag1", "tag2", "tag3"},
		Nested: map[string]interface{}{
			"key1": "value1",
			"key2": 456,
		},
	}

	msg, err := NewMessage("complex-topic", payload)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.NotEmpty(t, msg.Payload)
}

func TestNewMessage_InvalidPayload(t *testing.T) {
	// Channel cannot be marshaled to JSON
	payload := make(chan int)

	msg, err := NewMessage("test-topic", payload)

	assert.Error(t, err)
	assert.Nil(t, msg)
}

func TestNewMessageWithID_Extended(t *testing.T) {
	customID := "custom-message-id-12345"
	payload := map[string]string{"key": "value"}

	msg, err := NewMessageWithID(customID, "test-topic", payload)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, customID, msg.ID)
	assert.Equal(t, "test-topic", msg.Topic)
}

func TestNewMessageWithID_EmptyID(t *testing.T) {
	payload := map[string]string{"key": "value"}

	msg, err := NewMessageWithID("", "test-topic", payload)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Empty(t, msg.ID)
}

func TestMessage_Unmarshal_StructPayload(t *testing.T) {
	type TestPayload struct {
		Name  string
		Count int
	}

	original := TestPayload{
		Name:  "Test",
		Count: 42,
	}

	msg, err := NewMessage("test", original)
	assert.NoError(t, err)

	var decoded TestPayload
	err = msg.Unmarshal(&decoded)

	assert.NoError(t, err)
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Count, decoded.Count)
}

func TestMessage_Headers(t *testing.T) {
	payload := map[string]string{"data": "test"}
	msg, err := NewMessage("test", payload)
	assert.NoError(t, err)

	// Add headers
	msg.Headers["X-Source"] = "test-service"
	msg.Headers["X-Version"] = "1.0"
	msg.Headers["X-Priority"] = "high"

	assert.Equal(t, "test-service", msg.Headers["X-Source"])
	assert.Equal(t, "1.0", msg.Headers["X-Version"])
	assert.Equal(t, "high", msg.Headers["X-Priority"])
	assert.Len(t, msg.Headers, 3)
}

func TestMessage_Metadata(t *testing.T) {
	payload := map[string]string{"data": "test"}
	msg, err := NewMessage("test", payload)
	assert.NoError(t, err)

	// Add metadata
	msg.Metadata["retry_count"] = 3
	msg.Metadata["processing_time_ms"] = 150.5
	msg.Metadata["source_service"] = "streamer"

	assert.Equal(t, 3, msg.Metadata["retry_count"])
	assert.Equal(t, 150.5, msg.Metadata["processing_time_ms"])
	assert.Equal(t, "streamer", msg.Metadata["source_service"])
	assert.Len(t, msg.Metadata, 3)
}

func TestMessage_JSONRoundtrip(t *testing.T) {
	payload := map[string]interface{}{
		"name":  "test",
		"value": 123,
	}

	msg, err := NewMessage("test-topic", payload)
	assert.NoError(t, err)

	msg.Headers["X-Test"] = "header-value"
	msg.Metadata["meta-key"] = "meta-value"

	// Marshal to JSON
	jsonData, err := json.Marshal(msg)
	assert.NoError(t, err)

	// Unmarshal back
	var decoded Message
	err = json.Unmarshal(jsonData, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, msg.ID, decoded.ID)
	assert.Equal(t, msg.Topic, decoded.Topic)
	assert.Equal(t, msg.Headers["X-Test"], decoded.Headers["X-Test"])
}

func TestMessage_EmptyPayload(t *testing.T) {
	msg, err := NewMessage("test-topic", nil)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "test-topic", msg.Topic)
}

func TestMessage_TopicVariations(t *testing.T) {
	topics := []string{
		"telemetry",
		"telemetry.gpu",
		"telemetry.metrics.v1",
		"gpu.discovery",
		"events.stream.data",
	}

	for _, topic := range topics {
		msg, err := NewMessage(topic, map[string]string{"test": "data"})
		assert.NoError(t, err)
		assert.Equal(t, topic, msg.Topic)
	}
}

func TestMessage_TimestampPrecision(t *testing.T) {
	before := time.Now()
	msg, err := NewMessage("test", map[string]string{"data": "test"})
	after := time.Now()

	assert.NoError(t, err)
	assert.True(t, msg.Timestamp.After(before) || msg.Timestamp.Equal(before))
	assert.True(t, msg.Timestamp.Before(after) || msg.Timestamp.Equal(after))
}

func TestMessage_PayloadPreservation(t *testing.T) {
	payload := map[string]interface{}{
		"string": "test",
		"int":    123,
		"float":  45.67,
		"bool":   true,
		"null":   nil,
		"array":  []interface{}{1, 2, 3},
		"object": map[string]interface{}{"nested": "value"},
	}

	msg, err := NewMessage("test", payload)
	assert.NoError(t, err)

	var decoded map[string]interface{}
	err = msg.Unmarshal(&decoded)
	assert.NoError(t, err)

	assert.Equal(t, "test", decoded["string"])
	// JSON numbers are float64
	assert.Equal(t, float64(123), decoded["int"])
	assert.Equal(t, 45.67, decoded["float"])
	assert.Equal(t, true, decoded["bool"])
	assert.Nil(t, decoded["null"])
	assert.Len(t, decoded["array"], 3)
	assert.NotNil(t, decoded["object"])
}

func BenchmarkNewMessage(b *testing.B) {
	payload := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewMessage("test-topic", payload)
	}
}

func BenchmarkMessage_UnmarshalPayload(b *testing.B) {
	type TestPayload struct {
		Name  string
		Count int
	}

	msg, _ := NewMessage("test", TestPayload{Name: "Test", Count: 42})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decoded TestPayload
		msg.Unmarshal(&decoded)
	}
}
