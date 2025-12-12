package streamer

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStreamer_WithAllParams(t *testing.T) {
	config := &Config{
		CSVPath:        "test.csv",
		StreamInterval: 1 * time.Second,
		LoopMode:       false,
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	assert.NotNil(t, streamer)
	assert.Equal(t, config, streamer.config)
	assert.NotNil(t, streamer.logger)
	assert.Equal(t, int64(0), streamer.rowsSent.Load())
	assert.Equal(t, int64(0), streamer.errorCount.Load())
}

func TestNewStreamer_NilLogger(t *testing.T) {
	config := &Config{
		CSVPath:        "test.csv",
		StreamInterval: 1 * time.Second,
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, nil)

	assert.NotNil(t, streamer)
	assert.NotNil(t, streamer.logger) // Should have default logger
}

// Config validation tests removed - Config doesn't have Validate method

func TestStreamer_Start_NonLoopMode(t *testing.T) {
	// Create a temporary CSV file
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value
2025-07-18T20:42:34Z,sm_clock,0,nvidia0,gpu-001,Tesla V100,host1,container1,pod1,ns1,1500.0
2025-07-18T20:42:34Z,mem_clock,0,nvidia0,gpu-001,Tesla V100,host1,container1,pod1,ns1,800.0
`
	tmpFile := createTempCSV(t, csvContent)
	defer os.Remove(tmpFile)

	config := &Config{
		CSVPath:        tmpFile,
		StreamInterval: 1 * time.Millisecond,
		LoopMode:       false, // Should exit after one pass
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := streamer.Start(ctx)

	// Should complete without error
	assert.NoError(t, err)
	// Should have published 2 messages
	assert.Equal(t, int32(2), mockQueue.publishCount.Load())
	// Should have sent 2 rows
	assert.Equal(t, int64(2), streamer.rowsSent.Load())
}

func TestStreamer_Start_LoopMode(t *testing.T) {
	// Create a temporary CSV file
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value
2025-07-18T20:42:34Z,sm_clock,0,nvidia0,gpu-001,Tesla V100,host1,container1,pod1,ns1,1500.0
`
	tmpFile := createTempCSV(t, csvContent)
	defer os.Remove(tmpFile)

	config := &Config{
		CSVPath:        tmpFile,
		StreamInterval: 1 * time.Millisecond,
		LoopMode:       true, // Should loop
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := streamer.Start(ctx)

	// Should exit with context deadline exceeded
	assert.Error(t, err)
	// Should have published at least 1 message (may not complete multiple loops in 500ms)
	assert.GreaterOrEqual(t, mockQueue.publishCount.Load(), int32(1))
}

func TestStreamer_Start_FileNotFound(t *testing.T) {
	config := &Config{
		CSVPath:        "/nonexistent/path/to/file.csv",
		StreamInterval: 1 * time.Second,
		LoopMode:       false,
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	ctx := context.Background()
	err := streamer.Start(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open CSV file")
}

func TestStreamer_Start_ContextCancelled(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value
2025-07-18T20:42:34Z,sm_clock,0,nvidia0,gpu-001,Tesla V100,host1,container1,pod1,ns1,1500.0
`
	tmpFile := createTempCSV(t, csvContent)
	defer os.Remove(tmpFile)

	config := &Config{
		CSVPath:        tmpFile,
		StreamInterval: 1 * time.Second,
		LoopMode:       true,
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := streamer.Start(ctx)

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestStreamer_streamFile_Success(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value
2025-07-18T20:42:34Z,sm_clock,0,nvidia0,gpu-001,Tesla V100,host1,container1,pod1,ns1,1500.0
2025-07-18T20:42:34Z,mem_clock,1,nvidia1,gpu-002,Tesla V100,host2,container2,pod2,ns2,800.0
2025-07-18T20:42:34Z,temperature,2,nvidia2,gpu-003,Tesla V100,host3,container3,pod3,ns3,65.0
`
	tmpFile := createTempCSV(t, csvContent)
	defer os.Remove(tmpFile)

	config := &Config{
		CSVPath:        tmpFile,
		StreamInterval: 1 * time.Second,
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	ctx := context.Background()
	err := streamer.streamFile(ctx)

	assert.NoError(t, err)
	assert.Equal(t, int32(3), mockQueue.publishCount.Load())
	assert.Equal(t, int64(3), streamer.rowsSent.Load())
}

func TestStreamer_streamFile_InvalidRows(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value
invalid,line,with,missing,columns
2025-07-18T20:42:34Z,sm_clock,0,nvidia0,gpu-001,Tesla V100,host1,container1,pod1,ns1,1500.0
another,invalid,line
2025-07-18T20:42:34Z,mem_clock,1,nvidia1,gpu-002,Tesla V100,host2,container2,pod2,ns2,800.0
`
	tmpFile := createTempCSV(t, csvContent)
	defer os.Remove(tmpFile)

	config := &Config{
		CSVPath:        tmpFile,
		StreamInterval: 1 * time.Second,
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	ctx := context.Background()
	err := streamer.streamFile(ctx)

	// Should complete despite invalid rows
	assert.NoError(t, err)
	// Should publish only valid rows (2)
	assert.Equal(t, int32(2), mockQueue.publishCount.Load())
	// Should track errors
	assert.Greater(t, streamer.errorCount.Load(), int64(0))
}

func TestStreamer_streamFile_EmptyFile(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value
`
	tmpFile := createTempCSV(t, csvContent)
	defer os.Remove(tmpFile)

	config := &Config{
		CSVPath:        tmpFile,
		StreamInterval: 1 * time.Second,
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	ctx := context.Background()
	err := streamer.streamFile(ctx)

	// Should complete successfully even with empty file
	assert.NoError(t, err)
	assert.Equal(t, int32(0), mockQueue.publishCount.Load())
	assert.Equal(t, int64(0), streamer.rowsSent.Load())
}

func TestStreamer_PublishError(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value
2025-07-18T20:42:34Z,sm_clock,0,nvidia0,gpu-001,Tesla V100,host1,container1,pod1,ns1,1500.0
2025-07-18T20:42:34Z,mem_clock,1,nvidia1,gpu-002,Tesla V100,host2,container2,pod2,ns2,800.0
`
	tmpFile := createTempCSV(t, csvContent)
	defer os.Remove(tmpFile)

	config := &Config{
		CSVPath:        tmpFile,
		StreamInterval: 1 * time.Second,
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{shouldFail: true}
	mockGPURepo := &MockGPURepository{}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	ctx := context.Background()
	err := streamer.streamFile(ctx)

	// Should complete but not publish messages due to errors
	assert.NoError(t, err)
	// No messages published due to errors
	assert.Equal(t, int32(0), mockQueue.publishCount.Load())
	// Rows sent should not be incremented
	assert.Equal(t, int64(0), streamer.rowsSent.Load())
}

func TestStreamer_Stats(t *testing.T) {
	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	config := &Config{
		CSVPath:        "test.csv",
		StreamInterval: 1 * time.Second,
		InstanceID:     "test-instance",
	}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	// Initially zero
	stats := streamer.Stats()
	assert.Equal(t, int64(0), stats.RowsSent)
	assert.Equal(t, int64(0), stats.ErrorCount)

	// Manually increment
	streamer.rowsSent.Add(10)
	streamer.errorCount.Add(3)

	stats = streamer.Stats()
	assert.Equal(t, int64(10), stats.RowsSent)
	assert.Equal(t, int64(3), stats.ErrorCount)
}

func TestStreamer_GPURepositoryError(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value
2025-07-18T20:42:34Z,sm_clock,0,nvidia0,gpu-001,Tesla V100,host1,container1,pod1,ns1,1500.0
`
	tmpFile := createTempCSV(t, csvContent)
	defer os.Remove(tmpFile)

	config := &Config{
		CSVPath:        tmpFile,
		StreamInterval: 1 * time.Second,
		InstanceID:     "test-instance",
	}

	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{shouldFail: true}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	ctx := context.Background()
	err := streamer.streamFile(ctx)

	// Should complete successfully even if GPU repo fails (non-fatal)
	assert.NoError(t, err)
	// Should still publish telemetry
	assert.Equal(t, int32(1), mockQueue.publishCount.Load())
}

func TestStreamer_ConcurrentStats(t *testing.T) {
	mockQueue := &MockMessageQueue{}
	mockGPURepo := &MockGPURepository{}

	config := &Config{
		CSVPath:        "test.csv",
		StreamInterval: 1 * time.Second,
		InstanceID:     "test-instance",
	}

	streamer := NewStreamer(config, mockQueue, mockGPURepo, nil, slog.Default())

	// Increment counters concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				streamer.rowsSent.Add(1)
				streamer.errorCount.Add(1)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := streamer.Stats()
	assert.Equal(t, int64(1000), stats.RowsSent)
	assert.Equal(t, int64(1000), stats.ErrorCount)
}

// Helper function to create temporary CSV file
func createTempCSV(t *testing.T, content string) string {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.csv")
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)
	return tmpFile
}

// Mock implementations
type MockMessageQueue struct {
	publishCount atomic.Int32
	shouldFail   bool
}

func (m *MockMessageQueue) Start(ctx context.Context) error { return nil }
func (m *MockMessageQueue) Stop() error                     { return nil }
func (m *MockMessageQueue) Close() error                    { return nil }
func (m *MockMessageQueue) Subscribe(ctx context.Context, topic string, handler mq.MessageHandler) error {
	return nil
}
func (m *MockMessageQueue) Unsubscribe(topic string) error { return nil }
func (m *MockMessageQueue) Publish(ctx context.Context, msg *mq.Message) error {
	if m.shouldFail {
		return errors.New("mock publish error")
	}
	m.publishCount.Add(1)
	return nil
}
func (m *MockMessageQueue) Stats() mq.QueueStats {
	return mq.QueueStats{
		TotalPublished: int64(m.publishCount.Load()),
		QueueDepth:     0,
	}
}

type MockGPURepository struct {
	shouldFail bool
}

func (m *MockGPURepository) Store(gpu *domain.GPU) error {
	if m.shouldFail {
		return errors.New("mock store error")
	}
	return nil
}
func (m *MockGPURepository) BulkStore(gpus []*domain.GPU) error {
	if m.shouldFail {
		return errors.New("mock store error")
	}
	return nil
}
func (m *MockGPURepository) GetByUUID(uuid string) (*domain.GPU, error) { return nil, nil }
func (m *MockGPURepository) List() ([]*domain.GPU, error)               { return nil, nil }
func (m *MockGPURepository) Count() int64                               { return 0 }
func (m *MockGPURepository) DeleteByUUID(uuid string) error             { return nil }
