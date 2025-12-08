package collector

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_HandleMessage_ValidTelemetry(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             1,
		MaxConcurrentHandlers: 10,
	}

	queue := mq.NewInMemoryQueue(mq.InMemoryQueueConfig{
		BufferSize: 100,
		MaxWorkers: 10,
	})
	telemetryRepo := inmemory.NewTelemetryRepository()
	gpuRepo := inmemory.NewGPURepository()

	collector := NewCollector(config, queue, telemetryRepo, gpuRepo, nil)

	// Create valid telemetry message
	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "GPU-123",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      "75",
		Timestamp:  time.Now(),
	}

	payload, _ := json.Marshal(telemetry)
	msg := &mq.Message{
		ID:        "msg-1",
		Topic:     "telemetry",
		Payload:   payload,
		Headers:   make(map[string]string),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"hostname": "host-1",
			"device":   "nvidia0",
		},
	}

	// Handle message
	err := collector.handleMessage(context.Background(), msg)
	require.NoError(t, err)

	// Verify statistics
	assert.Equal(t, int64(1), collector.messagesProcessed.Load())
	assert.Equal(t, int64(0), collector.messagesErrors.Load())
	assert.Equal(t, int64(1), collector.telemetryStored.Load())
	assert.Equal(t, int64(1), collector.gpusStored.Load())

	// Verify telemetry was stored
	stored, err := telemetryRepo.GetByGPU("GPU-123", storage.TimeFilter{})
	require.NoError(t, err)
	assert.Len(t, stored, 1)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", stored[0].MetricName)

	// Verify GPU was stored
	gpu, err := gpuRepo.GetByUUID("GPU-123")
	require.NoError(t, err)
	assert.Equal(t, "GPU-123", gpu.UUID)
	assert.Equal(t, "host-1", gpu.Hostname)
}

func TestCollector_HandleMessage_MalformedJSON(t *testing.T) {
	config := &Config{
		InstanceID: "test-collector",
	}

	queue := mq.NewInMemoryQueue(mq.InMemoryQueueConfig{BufferSize: 10})
	telemetryRepo := inmemory.NewTelemetryRepository()
	gpuRepo := inmemory.NewGPURepository()

	collector := NewCollector(config, queue, telemetryRepo, gpuRepo, nil)

	// Create message with malformed JSON
	msg := &mq.Message{
		ID:        "msg-1",
		Topic:     "telemetry",
		Payload:   []byte("{invalid json}"),
		Timestamp: time.Now(),
	}

	// Handle message - should not return error (malformed data is acknowledged)
	err := collector.handleMessage(context.Background(), msg)
	assert.NoError(t, err)

	// Verify error counted
	assert.Equal(t, int64(1), collector.messagesProcessed.Load())
	assert.Equal(t, int64(1), collector.messagesErrors.Load())
	assert.Equal(t, int64(0), collector.telemetryStored.Load())
}

func TestCollector_HandleMessage_MissingGPUUUID(t *testing.T) {
	config := &Config{InstanceID: "test-collector"}
	queue := mq.NewInMemoryQueue(mq.InMemoryQueueConfig{BufferSize: 10})
	telemetryRepo := inmemory.NewTelemetryRepository()
	gpuRepo := inmemory.NewGPURepository()

	collector := NewCollector(config, queue, telemetryRepo, gpuRepo, nil)

	// Create telemetry with missing UUID
	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "", // Missing
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      "75",
		Timestamp:  time.Now(),
	}

	payload, _ := json.Marshal(telemetry)
	msg := &mq.Message{
		ID:        "msg-1",
		Payload:   payload,
		Timestamp: time.Now(),
	}

	err := collector.handleMessage(context.Background(), msg)
	assert.NoError(t, err) // Invalid data is acknowledged, not requeued

	assert.Equal(t, int64(1), collector.messagesErrors.Load())
	assert.Equal(t, int64(0), collector.telemetryStored.Load())
}

func TestCollector_HandleMessage_MissingTimestamp(t *testing.T) {
	config := &Config{InstanceID: "test-collector"}
	queue := mq.NewInMemoryQueue(mq.InMemoryQueueConfig{BufferSize: 10})
	telemetryRepo := inmemory.NewTelemetryRepository()
	gpuRepo := inmemory.NewGPURepository()

	collector := NewCollector(config, queue, telemetryRepo, gpuRepo, nil)

	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "GPU-123",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      "75",
		// Timestamp missing (zero value)
	}

	payload, _ := json.Marshal(telemetry)
	msg := &mq.Message{
		ID:        "msg-1",
		Payload:   payload,
		Timestamp: time.Now(),
	}

	err := collector.handleMessage(context.Background(), msg)
	assert.NoError(t, err)

	assert.Equal(t, int64(1), collector.messagesErrors.Load())
	assert.Equal(t, int64(0), collector.telemetryStored.Load())
}

func TestCollector_HandleMessage_NoGPUMetadata(t *testing.T) {
	config := &Config{InstanceID: "test-collector"}
	queue := mq.NewInMemoryQueue(mq.InMemoryQueueConfig{BufferSize: 10})
	telemetryRepo := inmemory.NewTelemetryRepository()
	gpuRepo := inmemory.NewGPURepository()

	collector := NewCollector(config, queue, telemetryRepo, gpuRepo, nil)

	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "GPU-123",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      "75",
		Timestamp:  time.Now(),
	}

	payload, _ := json.Marshal(telemetry)
	msg := &mq.Message{
		ID:        "msg-1",
		Payload:   payload,
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{}, // No GPU metadata
	}

	err := collector.handleMessage(context.Background(), msg)
	require.NoError(t, err)

	// Telemetry should be stored even without GPU metadata
	assert.Equal(t, int64(1), collector.telemetryStored.Load())
	// GPU should not be stored (no metadata)
	assert.Equal(t, int64(0), collector.gpusStored.Load())
}

func TestCollector_MultipleMessages(t *testing.T) {
	config := &Config{InstanceID: "test-collector"}
	queue := mq.NewInMemoryQueue(mq.InMemoryQueueConfig{BufferSize: 100})
	telemetryRepo := inmemory.NewTelemetryRepository()
	gpuRepo := inmemory.NewGPURepository()

	collector := NewCollector(config, queue, telemetryRepo, gpuRepo, nil)

	// Process multiple messages
	for i := 0; i < 10; i++ {
		telemetry := &domain.TelemetryPoint{
			GPUUUID:    "GPU-123",
			MetricName: "DCGM_FI_DEV_GPU_UTIL",
			Value:      string(rune('0' + i)),
			Timestamp:  time.Now().Add(time.Duration(i) * time.Second),
		}

		payload, _ := json.Marshal(telemetry)
		msg := &mq.Message{
			ID:        string(rune('a' + i)),
			Payload:   payload,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"hostname": "host-1",
				"device":   "nvidia0",
			},
		}

		err := collector.handleMessage(context.Background(), msg)
		require.NoError(t, err)
	}

	// Verify all processed
	assert.Equal(t, int64(10), collector.messagesProcessed.Load())
	assert.Equal(t, int64(10), collector.telemetryStored.Load())
	assert.Equal(t, int64(0), collector.messagesErrors.Load())

	// Verify all stored in repository
	stored, err := telemetryRepo.GetByGPU("GPU-123", storage.TimeFilter{})
	require.NoError(t, err)
	assert.Len(t, stored, 10)
}

func TestCollector_Stats(t *testing.T) {
	config := &Config{InstanceID: "test-collector"}
	queue := mq.NewInMemoryQueue(mq.InMemoryQueueConfig{BufferSize: 10})
	telemetryRepo := inmemory.NewTelemetryRepository()
	gpuRepo := inmemory.NewGPURepository()

	collector := NewCollector(config, queue, telemetryRepo, gpuRepo, nil)

	// Initially zero
	stats := collector.Stats()
	assert.Equal(t, int64(0), stats.MessagesProcessed)
	assert.Equal(t, int64(0), stats.MessagesErrors)

	// Process a message
	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "GPU-123",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      "75",
		Timestamp:  time.Now(),
	}

	payload, _ := json.Marshal(telemetry)
	msg := &mq.Message{
		ID:        "msg-1",
		Payload:   payload,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"hostname": "host-1",
		},
	}

	collector.handleMessage(context.Background(), msg)

	// Check stats updated
	stats = collector.Stats()
	assert.Equal(t, int64(1), stats.MessagesProcessed)
	assert.Equal(t, int64(1), stats.TelemetryStored)
}

func TestCollector_ValidateTelemetry(t *testing.T) {
	collector := &Collector{}

	tests := []struct {
		name    string
		point   *domain.TelemetryPoint
		wantErr bool
	}{
		{
			name: "valid telemetry",
			point: &domain.TelemetryPoint{
				GPUUUID:    "GPU-123",
				MetricName: "DCGM_FI_DEV_GPU_UTIL",
				Timestamp:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing GPU UUID",
			point: &domain.TelemetryPoint{
				MetricName: "DCGM_FI_DEV_GPU_UTIL",
				Timestamp:  time.Now(),
			},
			wantErr: true,
		},
		{
			name: "missing metric name",
			point: &domain.TelemetryPoint{
				GPUUUID:   "GPU-123",
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "missing timestamp",
			point: &domain.TelemetryPoint{
				GPUUUID:    "GPU-123",
				MetricName: "DCGM_FI_DEV_GPU_UTIL",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := collector.validateTelemetry(tt.point)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
