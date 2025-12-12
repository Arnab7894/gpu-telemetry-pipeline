package streamer

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/stretchr/testify/assert"
)

func TestConfig_Defaults(t *testing.T) {
	cfg := &Config{
		InstanceID:     "test-1",
		CSVPath:        "test.csv",
		StreamInterval: 1 * time.Second,
	}

	assert.NotEmpty(t, cfg.InstanceID)
	assert.NotEmpty(t, cfg.CSVPath)
	assert.NotZero(t, cfg.StreamInterval)
}

func TestNewBatchLock_InvalidRedisURL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err := NewBatchLock("invalid://url", "batch-1", logger)
	assert.Error(t, err)
}

func TestStreamerConstants(t *testing.T) {
	assert.Equal(t, "telemetry", TopicTelemetry)
	assert.Equal(t, "gpu", TopicGPU)
}

func TestStreamer_AtomicCounters(t *testing.T) {
	cfg := &Config{
		InstanceID:     "test-1",
		CSVPath:        "/nonexistent.csv",
		StreamInterval: 1 * time.Second,
	}

	queue := &mq.InMemoryQueue{}
	err := queue.Start(context.Background())
	assert.NoError(t, err)

	streamer := NewStreamer(cfg, queue, nil, nil, nil)
	assert.NotNil(t, streamer)

	// Test atomic counters
	streamer.rowsSent.Add(5)
	assert.Equal(t, int64(5), streamer.rowsSent.Load())

	streamer.errorCount.Add(2)
	assert.Equal(t, int64(2), streamer.errorCount.Load())
}

func TestMessage_Creation(t *testing.T) {
	payload := map[string]string{"test": "data"}
	msg, err := mq.NewMessage("test-topic", payload)

	assert.NoError(t, err)
	assert.NotNil(t, msg)
	assert.NotEmpty(t, msg.ID)
	assert.Equal(t, "test-topic", msg.Topic)
	assert.NotNil(t, msg.Payload)
}
