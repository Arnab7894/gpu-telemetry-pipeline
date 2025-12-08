package mq

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessage(t *testing.T) {
	topic := "test-topic"
	data := map[string]interface{}{"key": "value"}

	msg, err := NewMessage(topic, data)

	require.NoError(t, err)
	assert.NotEmpty(t, msg.ID)
	assert.Equal(t, topic, msg.Topic)
	assert.NotNil(t, msg.Payload)
	assert.False(t, msg.Timestamp.IsZero())
}

func TestNewMessageWithID(t *testing.T) {
	id := "custom-id-123"
	topic := "test-topic"
	data := map[string]interface{}{"key": "value"}

	msg, err := NewMessageWithID(id, topic, data)

	require.NoError(t, err)
	assert.Equal(t, id, msg.ID)
	assert.Equal(t, topic, msg.Topic)
	assert.NotNil(t, msg.Payload)
}

func TestMessage_Unmarshal(t *testing.T) {
	type TestPayload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	payload := TestPayload{Name: "test", Value: 42}
	msg, err := NewMessage("test", payload)
	require.NoError(t, err)

	var result TestPayload
	err = msg.Unmarshal(&result)

	assert.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 42, result.Value)
}

func TestMessage_WithHeader(t *testing.T) {
	msg, err := NewMessage("test", nil)
	require.NoError(t, err)

	msg = msg.WithHeader("X-Custom-Header", "custom-value")

	assert.NotNil(t, msg.Headers)
	assert.Equal(t, "custom-value", msg.Headers["X-Custom-Header"])
}

func TestMessage_WithMetadata(t *testing.T) {
	msg, err := NewMessage("test", nil)
	require.NoError(t, err)

	msg = msg.WithMetadata("retry-count", "3")

	assert.NotNil(t, msg.Metadata)
	assert.Equal(t, "3", msg.Metadata["retry-count"])
}

func TestMessage_GetHeader(t *testing.T) {
	msg, err := NewMessage("test", nil)
	require.NoError(t, err)
	msg.Headers = map[string]string{"Content-Type": "application/json"}

	value, ok := msg.GetHeader("Content-Type")
	assert.True(t, ok)
	assert.Equal(t, "application/json", value)

	missing, ok := msg.GetHeader("Missing-Header")
	assert.False(t, ok)
	assert.Empty(t, missing)
}

func TestMessage_GetMetadata(t *testing.T) {
	msg, err := NewMessage("test", nil)
	require.NoError(t, err)
	msg.Metadata = map[string]interface{}{"source": "streamer"}

	value, ok := msg.GetMetadata("source")
	assert.True(t, ok)
	assert.Equal(t, "streamer", value)

	missing, ok := msg.GetMetadata("missing")
	assert.False(t, ok)
	assert.Nil(t, missing)
}

func TestMessage_ChainedMethods(t *testing.T) {
	msg, err := NewMessage("test", map[string]string{"key": "value"})
	require.NoError(t, err)

	msg = msg.WithHeader("X-Request-ID", "req-123").
		WithMetadata("version", "1.0")

	value, ok := msg.GetHeader("X-Request-ID")
	assert.True(t, ok)
	assert.Equal(t, "req-123", value)

	metaValue, ok := msg.GetMetadata("version")
	assert.True(t, ok)
	assert.Equal(t, "1.0", metaValue)
}
