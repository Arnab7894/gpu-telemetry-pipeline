package collector

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCollector(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	mockQueue := &MockMessageQueue{}
	mockTelemetryRepo := &MockTelemetryRepository{}
	mockGPURepo := &MockGPURepository{}

	collector := NewCollector(config, mockQueue, mockTelemetryRepo, mockGPURepo, slog.Default())

	assert.NotNil(t, collector)
	assert.Equal(t, config, collector.config)
	assert.NotNil(t, collector.logger)
	assert.NotNil(t, collector.workQueue)
	assert.NotNil(t, collector.workersDone)
}

func TestNewCollector_NilLogger(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	mockQueue := &MockMessageQueue{}
	mockTelemetryRepo := &MockTelemetryRepository{}
	mockGPURepo := &MockGPURepository{}

	collector := NewCollector(config, mockQueue, mockTelemetryRepo, mockGPURepo, nil)

	assert.NotNil(t, collector)
	assert.NotNil(t, collector.logger) // Should have default logger
}

func TestCollector_processMessage_Success(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	mockQueue := &MockMessageQueue{}
	mockTelemetryRepo := &MockTelemetryRepository{}
	mockGPURepo := &MockGPURepository{}

	collector := NewCollector(config, mockQueue, mockTelemetryRepo, mockGPURepo, slog.Default())

	// Create a valid telemetry message
	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "gpu-123",
		MetricName: "temperature",
		Value:      "65.5",
		Timestamp:  time.Now(),
		BatchID:    "batch-1",
	}

	payload, err := json.Marshal(telemetry)
	require.NoError(t, err)

	msg := &mq.Message{
		ID:      "msg-1",
		Topic:   "telemetry",
		Payload: payload,
		Headers: map[string]string{
			"gpu_uuid": "gpu-123",
		},
		Metadata: map[string]interface{}{
			"hostname":   "host1",
			"model_name": "Tesla V100",
			"device":     "nvidia0",
			"gpu_index":  0,
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	collector.processMessage(ctx, msg)

	// Verify statistics
	assert.Equal(t, int64(1), collector.messagesProcessed.Load())
	assert.Equal(t, int64(0), collector.messagesErrors.Load())
	assert.Equal(t, int64(1), collector.telemetryStored.Load())
	assert.Equal(t, int64(1), collector.gpusStored.Load())

	// Verify data was stored
	assert.Equal(t, int32(1), mockTelemetryRepo.storeCount.Load())
	assert.Equal(t, int32(1), mockGPURepo.storeCount.Load())
}

func TestCollector_processMessage_InvalidJSON(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	mockQueue := &MockMessageQueue{}
	mockTelemetryRepo := &MockTelemetryRepository{}
	mockGPURepo := &MockGPURepository{}

	collector := NewCollector(config, mockQueue, mockTelemetryRepo, mockGPURepo, slog.Default())

	msg := &mq.Message{
		ID:        "msg-1",
		Topic:     "telemetry",
		Payload:   []byte("invalid json"),
		Headers:   map[string]string{},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	collector.processMessage(ctx, msg)

	// Verify error was tracked
	assert.Equal(t, int64(1), collector.messagesProcessed.Load())
	assert.Equal(t, int64(1), collector.messagesErrors.Load())
	assert.Equal(t, int64(0), collector.telemetryStored.Load())
}

func TestCollector_processMessage_MissingGPUUUID(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	mockQueue := &MockMessageQueue{}
	mockTelemetryRepo := &MockTelemetryRepository{}
	mockGPURepo := &MockGPURepository{}

	collector := NewCollector(config, mockQueue, mockTelemetryRepo, mockGPURepo, slog.Default())

	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "", // Missing
		MetricName: "temperature",
		Value:      "65.5",
		Timestamp:  time.Now(),
	}

	payload, _ := json.Marshal(telemetry)
	msg := &mq.Message{
		ID:        "msg-1",
		Topic:     "telemetry",
		Payload:   payload,
		Headers:   map[string]string{},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	collector.processMessage(ctx, msg)

	// Verify validation error
	assert.Equal(t, int64(1), collector.messagesProcessed.Load())
	assert.Equal(t, int64(1), collector.messagesErrors.Load())
	assert.Equal(t, int64(0), collector.telemetryStored.Load())
}

func TestCollector_processMessage_MissingMetricName(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	mockQueue := &MockMessageQueue{}
	mockTelemetryRepo := &MockTelemetryRepository{}
	mockGPURepo := &MockGPURepository{}

	collector := NewCollector(config, mockQueue, mockTelemetryRepo, mockGPURepo, slog.Default())

	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "gpu-123",
		MetricName: "", // Missing
		Value:      "65.5",
		Timestamp:  time.Now(),
	}

	payload, _ := json.Marshal(telemetry)
	msg := &mq.Message{
		ID:        "msg-1",
		Topic:     "telemetry",
		Payload:   payload,
		Headers:   map[string]string{},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	collector.processMessage(ctx, msg)

	// Verify validation error
	assert.Equal(t, int64(1), collector.messagesErrors.Load())
}

func TestCollector_processMessage_TelemetryStoreError(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	mockQueue := &MockMessageQueue{}
	mockTelemetryRepo := &MockTelemetryRepository{shouldFail: true}
	mockGPURepo := &MockGPURepository{}

	collector := NewCollector(config, mockQueue, mockTelemetryRepo, mockGPURepo, slog.Default())

	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "gpu-123",
		MetricName: "temperature",
		Value:      "65.5",
		Timestamp:  time.Now(),
	}

	payload, _ := json.Marshal(telemetry)
	msg := &mq.Message{
		ID:        "msg-1",
		Topic:     "telemetry",
		Payload:   payload,
		Headers:   map[string]string{},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	collector.processMessage(ctx, msg)

	// Verify error was tracked
	assert.Equal(t, int64(1), collector.messagesProcessed.Load())
	assert.Equal(t, int64(1), collector.messagesErrors.Load())
	assert.Equal(t, int64(0), collector.telemetryStored.Load())
}

func TestCollector_processMessage_GPUStoreErrorNonFatal(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	mockQueue := &MockMessageQueue{}
	mockTelemetryRepo := &MockTelemetryRepository{}
	mockGPURepo := &MockGPURepository{shouldFail: true}

	collector := NewCollector(config, mockQueue, mockTelemetryRepo, mockGPURepo, slog.Default())

	telemetry := &domain.TelemetryPoint{
		GPUUUID:    "gpu-123",
		MetricName: "temperature",
		Value:      "65.5",
		Timestamp:  time.Now(),
	}

	payload, _ := json.Marshal(telemetry)
	msg := &mq.Message{
		ID:      "msg-1",
		Topic:   "telemetry",
		Payload: payload,
		Headers: map[string]string{},
		Metadata: map[string]interface{}{
			"hostname": "host1",
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	collector.processMessage(ctx, msg)

	// Telemetry should still be stored even if GPU store fails
	assert.Equal(t, int64(1), collector.messagesProcessed.Load())
	assert.Equal(t, int64(0), collector.messagesErrors.Load())
	assert.Equal(t, int64(1), collector.telemetryStored.Load())
	assert.Equal(t, int64(0), collector.gpusStored.Load())
}

func TestCollector_validateTelemetry(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	collector := NewCollector(config, nil, nil, nil, slog.Default())

	tests := []struct {
		name      string
		telemetry *domain.TelemetryPoint
		wantErr   bool
	}{
		{
			name: "valid telemetry",
			telemetry: &domain.TelemetryPoint{
				GPUUUID:    "gpu-123",
				MetricName: "temperature",
				Timestamp:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing GPU UUID",
			telemetry: &domain.TelemetryPoint{
				GPUUUID:    "",
				MetricName: "temperature",
				Timestamp:  time.Now(),
			},
			wantErr: true,
		},
		{
			name: "missing metric name",
			telemetry: &domain.TelemetryPoint{
				GPUUUID:    "gpu-123",
				MetricName: "",
				Timestamp:  time.Now(),
			},
			wantErr: true,
		},
		{
			name: "missing timestamp",
			telemetry: &domain.TelemetryPoint{
				GPUUUID:    "gpu-123",
				MetricName: "temperature",
				Timestamp:  time.Time{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := collector.validateTelemetry(tt.telemetry)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCollector_extractGPUFromMessage(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	collector := NewCollector(config, nil, nil, nil, slog.Default())

	telemetry := &domain.TelemetryPoint{
		GPUUUID: "gpu-123",
	}

	msg := &mq.Message{
		ID:    "msg-1",
		Topic: "telemetry",
		Headers: map[string]string{
			"gpu_uuid": "gpu-123",
		},
		Metadata: map[string]interface{}{
			"hostname":   "host1",
			"model_name": "Tesla V100",
			"device":     "nvidia0",
			"gpu_index":  float64(0), // JSON unmarshals numbers as float64
		},
	}

	gpu := collector.extractGPUFromMessage(msg, telemetry)

	assert.NotNil(t, gpu)
	assert.Equal(t, "gpu-123", gpu.UUID)
	assert.Equal(t, "host1", gpu.Hostname)
	assert.Equal(t, "Tesla V100", gpu.ModelName)
}

func TestCollector_extractGPUFromMessage_NoMetadata(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	collector := NewCollector(config, nil, nil, nil, slog.Default())

	telemetry := &domain.TelemetryPoint{
		GPUUUID: "gpu-123",
	}

	msg := &mq.Message{
		ID:       "msg-1",
		Topic:    "telemetry",
		Headers:  map[string]string{},
		Metadata: nil,
	}

	gpu := collector.extractGPUFromMessage(msg, telemetry)

	assert.NotNil(t, gpu)
	assert.Equal(t, "gpu-123", gpu.UUID)
}

func TestCollector_Stats_Detailed(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	collector := NewCollector(config, nil, nil, nil, slog.Default())

	// Initially zero
	stats := collector.Stats()
	assert.Equal(t, int64(0), stats.MessagesProcessed)
	assert.Equal(t, int64(0), stats.MessagesErrors)

	// Increment counters
	collector.messagesProcessed.Add(100)
	collector.messagesErrors.Add(5)
	collector.gpusStored.Add(20)
	collector.telemetryStored.Add(95)

	stats = collector.Stats()
	assert.Equal(t, int64(100), stats.MessagesProcessed)
	assert.Equal(t, int64(5), stats.MessagesErrors)
	assert.Equal(t, int64(20), stats.GPUsStored)
	assert.Equal(t, int64(95), stats.TelemetryStored)
}

func TestCollector_handleMessage_EnqueueSuccess(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	mockQueue := &MockMessageQueue{}
	mockTelemetryRepo := &MockTelemetryRepository{}
	mockGPURepo := &MockGPURepository{}

	collector := NewCollector(config, mockQueue, mockTelemetryRepo, mockGPURepo, slog.Default())

	msg := &mq.Message{
		ID:    "msg-1",
		Topic: "telemetry",
	}

	ctx := context.Background()
	err := collector.handleMessage(ctx, msg)

	assert.NoError(t, err)
	// Message should be in work queue
	assert.Equal(t, 1, len(collector.workQueue))
}

func TestCollector_ConcurrentProcessing(t *testing.T) {
	config := &Config{
		InstanceID:            "test-collector",
		BatchSize:             100,
		MaxConcurrentHandlers: 10,
	}

	mockQueue := &MockMessageQueue{}
	mockTelemetryRepo := &MockTelemetryRepository{}
	mockGPURepo := &MockGPURepository{}

	collector := NewCollector(config, mockQueue, mockTelemetryRepo, mockGPURepo, slog.Default())

	// Process multiple messages concurrently
	numMessages := 50
	for i := 0; i < numMessages; i++ {
		telemetry := &domain.TelemetryPoint{
			GPUUUID:    "gpu-123",
			MetricName: "temperature",
			Value:      "65.5",
			Timestamp:  time.Now(),
		}

		payload, _ := json.Marshal(telemetry)
		msg := &mq.Message{
			ID:        "msg-" + string(rune(i)),
			Topic:     "telemetry",
			Payload:   payload,
			Headers:   map[string]string{},
			Timestamp: time.Now(),
		}

		collector.processMessage(context.Background(), msg)
	}

	// Give some time for processing
	time.Sleep(100 * time.Millisecond)

	// Verify all messages were processed
	assert.Equal(t, int64(numMessages), collector.messagesProcessed.Load())
	assert.Equal(t, int64(0), collector.messagesErrors.Load())
	assert.Equal(t, int64(numMessages), collector.telemetryStored.Load())
}

// Mock implementations
type MockMessageQueue struct {
	subscribeCount atomic.Int32
}

func (m *MockMessageQueue) Start(ctx context.Context) error { return nil }
func (m *MockMessageQueue) Stop() error                     { return nil }
func (m *MockMessageQueue) Close() error                    { return nil }
func (m *MockMessageQueue) Subscribe(ctx context.Context, topic string, handler mq.MessageHandler) error {
	m.subscribeCount.Add(1)
	return nil
}
func (m *MockMessageQueue) Unsubscribe(topic string) error { return nil }
func (m *MockMessageQueue) Publish(ctx context.Context, msg *mq.Message) error {
	return nil
}
func (m *MockMessageQueue) Stats() mq.QueueStats {
	return mq.QueueStats{}
}

type MockTelemetryRepository struct {
	storeCount atomic.Int32
	shouldFail bool
}

func (m *MockTelemetryRepository) Store(t *domain.TelemetryPoint) error {
	if m.shouldFail {
		return errors.New("mock store error")
	}
	m.storeCount.Add(1)
	return nil
}

func (m *MockTelemetryRepository) BulkStore(points []*domain.TelemetryPoint) error {
	if m.shouldFail {
		return errors.New("mock bulk store error")
	}
	m.storeCount.Add(int32(len(points)))
	return nil
}

func (m *MockTelemetryRepository) GetByGPUUUID(uuid string, start, end time.Time) ([]*domain.TelemetryPoint, error) {
	return nil, nil
}

func (m *MockTelemetryRepository) GetByGPUUUIDAndMetric(uuid, metricName string, start, end time.Time) ([]*domain.TelemetryPoint, error) {
	return nil, nil
}

func (m *MockTelemetryRepository) DeleteOlderThan(cutoff time.Time) error {
	return nil
}

func (m *MockTelemetryRepository) Count() int64 {
	return int64(m.storeCount.Load())
}

func (m *MockTelemetryRepository) GetByGPU(gpuUUID string, filter storage.TimeFilter) ([]*domain.TelemetryPoint, error) {
	return nil, nil
}

type MockGPURepository struct {
	storeCount atomic.Int32
	shouldFail bool
}

func (m *MockGPURepository) Store(gpu *domain.GPU) error {
	if m.shouldFail {
		return errors.New("mock store error")
	}
	m.storeCount.Add(1)
	return nil
}

func (m *MockGPURepository) BulkStore(gpus []*domain.GPU) error {
	if m.shouldFail {
		return errors.New("mock bulk store error")
	}
	m.storeCount.Add(int32(len(gpus)))
	return nil
}

func (m *MockGPURepository) GetByUUID(uuid string) (*domain.GPU, error) {
	return nil, nil
}

func (m *MockGPURepository) List() ([]*domain.GPU, error) {
	return nil, nil
}

func (m *MockGPURepository) Count() int64 {
	return 0
}

func (m *MockGPURepository) DeleteByUUID(uuid string) error {
	return nil
}
