# Telemetry Collector

The **Telemetry Collector** subscribes to the message queue, receives telemetry messages published by Streamers, validates and stores them in repositories.

## Features

- **Message Queue Subscription**: Subscribes to `telemetry` topic
- **JSON Deserialization**: Converts message payloads back to domain structs
- **Validation**: Checks for required fields (GPUUUID, MetricName, Timestamp)
- **Dual Storage**: Stores telemetry data and GPU metadata separately
- **Concurrent Processing**: Handles multiple messages in parallel
- **Error Handling**: Logs and counts malformed data without crashing
- **Statistics Tracking**: Real-time counters for messages, errors, and storage
- **Graceful Shutdown**: SIGINT/SIGTERM handling with final stats report

## Architecture

```
Message Queue → Collector → TelemetryRepository
                         ↓
                    GPURepository
```

### Message Flow

1. Collector subscribes to `telemetry` topic
2. Queue delivers messages to `handleMessage()`
3. Message payload unmarshaled to `TelemetryPoint`
4. Validation checks (GPUUUID, MetricName, Timestamp)
5. GPU metadata extracted from message metadata
6. Telemetry stored in `TelemetryRepository`
7. GPU info stored/updated in `GPURepository`
8. Statistics incremented (atomic counters)

## Configuration

### Environment Variables

- `INSTANCE_ID`: Unique identifier for this collector (default: hostname)
- `BATCH_SIZE`: Number of messages to process before commit (default: 1)
- `MAX_CONCURRENT_HANDLERS`: Max parallel message handlers (default: 10)
- `QUEUE_BUFFER_SIZE`: Message queue buffer size (default: 1000)
- `QUEUE_WORKERS`: Number of queue worker goroutines (default: 10)

### Command-Line Flags

```bash
./bin/collector \
  -instance-id=collector-1 \
  -batch-size=1 \
  -max-concurrent=10 \
  -queue-buffer=1000 \
  -queue-workers=10
```

Flags override environment variables.

## Usage

### Single Collector

```bash
# Build
go build -o bin/collector ./cmd/collector

# Run
./bin/collector -instance-id=collector-1
```

### Multiple Collectors (Same Process)

For true parallel collection with load balancing, run multiple collector instances **in the same process** sharing the message queue:

```go
package main

import (
    "context"
    "github.com/arnabghosh/gpu-metrics-streamer/internal/collector"
    "github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
    "github.com/arnabghosh/gpu-metrics-streamer/internal/storage/inmemory"
)

func main() {
    ctx := context.Background()
    
    // Shared infrastructure
    queue := mq.NewInMemoryQueue(mq.InMemoryQueueConfig{
        BufferSize: 1000,
        MaxWorkers: 20,
    })
    telemetryRepo := inmemory.NewTelemetryRepository()
    gpuRepo := inmemory.NewGPURepository()
    
    queue.Start(ctx)
    
    // Launch multiple collectors
    for i := 1; i <= 3; i++ {
        config := &collector.Config{
            InstanceID: fmt.Sprintf("collector-%d", i),
            BatchSize:  1,
            MaxConcurrentHandlers: 10,
        }
        
        c := collector.NewCollector(config, queue, telemetryRepo, gpuRepo, nil)
        go c.Start(ctx)
    }
    
    // Keep running...
}
```

This approach enables:
- **Load Balancing**: Queue distributes messages across all subscribed collectors
- **Concurrent Processing**: Multiple collectors process messages in parallel
- **Shared Storage**: All collectors write to the same repositories (thread-safe)

### Important: Architecture Limitation

The current `InMemoryQueue` implementation is **process-local**. This means:

✅ **Within Same Process**: Multiple collector goroutines can share the same queue instance and achieve true parallel processing with load balancing.

❌ **Across Processes**: Separate processes (e.g., `./bin/streamer` and `./bin/collector`) create their own isolated queue instances. Messages published by the streamer **cannot** reach the collector's queue.

**For multi-process deployment** (separate streamer and collector binaries), you need a **shared message queue**:

- **Redis Streams/Pub-Sub**
- **MongoDB Change Streams**
- **RabbitMQ / Kafka**
- **NATS / NSQ**

See [ARCHITECTURE.md](./ARCHITECTURE.md) for migration to shared queue.

## Message Handling

### Valid Message

```json
{
  "gpu_uuid": "GPU-abc123",
  "metric_name": "DCGM_FI_DEV_GPU_UTIL",
  "value": "75",
  "timestamp": "2025-01-18T12:34:56Z"
}
```

Message metadata (optional):
```json
{
  "hostname": "gpu-node-1",
  "device": "nvidia0"
}
```

### Validation Rules

1. **GPUUUID**: Must be non-empty
2. **MetricName**: Must be non-empty
3. **Timestamp**: Must not be zero value
4. **JSON**: Must be valid JSON

Invalid messages are logged, counted, and acknowledged (not requeued).

### Error Handling

- **Malformed JSON**: Logged as warning, error counter incremented, message acknowledged
- **Missing Required Fields**: Logged with details, error counter incremented, message acknowledged
- **Storage Errors**: Logged, not counted as successfully stored
- **No Crash on Errors**: Collector continues processing next messages

## Statistics

The collector tracks:

- `messages_processed`: Total messages handled
- `messages_errors`: Messages with validation/parse errors
- `gpus_stored`: GPU records created/updated
- `telemetry_stored`: Telemetry points successfully stored
- `queue_delivered`: Messages delivered by queue to this collector

Statistics reported every 10 seconds and at shutdown.

Example log output:
```json
{
  "time": "2025-01-18T12:34:56Z",
  "level": "INFO",
  "msg": "Collector statistics",
  "component": "collector",
  "instance_id": "collector-1",
  "messages_processed": 1250,
  "messages_errors": 3,
  "gpus_stored": 42,
  "telemetry_stored": 1247,
  "queue_delivered": 1250
}
```

## Testing

### Unit Tests

```bash
# Run collector tests
go test ./internal/collector/... -v

# With coverage
go test ./internal/collector/... -cover
```

Test coverage:
- ✅ Valid telemetry handling
- ✅ Malformed JSON handling
- ✅ Missing GPUUUID validation
- ✅ Missing Timestamp validation
- ✅ Missing MetricName validation
- ✅ No GPU metadata handling
- ✅ Multiple messages processing
- ✅ Statistics tracking
- ✅ Validation logic

### Integration Testing (Single Process)

To test the full pipeline in a single process:

```go
// test_integration.go
package main

import (
    "context"
    "time"
    "github.com/arnabghosh/gpu-metrics-streamer/internal/collector"
    "github.com/arnabghosh/gpu-metrics-streamer/internal/streamer"
    "github.com/arnabghosh/gpu-metrics-streamer/internal/mq"
    "github.com/arnabghosh/gpu-metrics-streamer/internal/storage/inmemory"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // Shared queue
    queue := mq.NewInMemoryQueue(mq.InMemoryQueueConfig{
        BufferSize: 1000,
        MaxWorkers: 10,
    })
    queue.Start(ctx)
    
    // Repositories
    telemetryRepo := inmemory.NewTelemetryRepository()
    gpuRepo := inmemory.NewGPURepository()
    
    // Start collector first
    collectorCfg := &collector.Config{
        InstanceID: "test-collector",
        BatchSize:  1,
    }
    c := collector.NewCollector(collectorCfg, queue, telemetryRepo, gpuRepo, nil)
    go c.Start(ctx)
    
    time.Sleep(100 * time.Millisecond) // Let collector subscribe
    
    // Start streamer
    streamerCfg := &streamer.Config{
        CSVPath:    "test_sample.csv",
        InstanceID: "test-streamer",
        Interval:   10 * time.Millisecond,
        LoopMode:   false,
    }
    s := streamer.NewStreamer(streamerCfg, queue, nil)
    s.Start(ctx)
    
    time.Sleep(2 * time.Second) // Let messages process
    
    // Check results
    log.Printf("Collector stats: %+v", c.Stats())
    log.Printf("Queue stats: %+v", queue.Stats())
}
```

## Performance Considerations

1. **Batch Size**: Currently set to 1 (immediate processing). Increase for higher throughput at cost of latency.

2. **Concurrent Handlers**: Default 10. Increase if CPU underutilized, decrease if too many goroutines.

3. **Queue Buffer**: Default 1000 messages. Increase if backpressure observed.

4. **Queue Workers**: Default 10. Should match or exceed number of collectors.

5. **Statistics Reporting**: Every 10 seconds. Adjust `reportStats()` interval if too frequent.

## Future Enhancements

- [ ] Support for external message queues (Redis, Kafka, RabbitMQ)
- [ ] Configurable batch commit intervals
- [ ] Dead letter queue for permanently failed messages
- [ ] Metrics export (Prometheus)
- [ ] Health check endpoint
- [ ] Message compression
- [ ] Idempotency checks (duplicate detection)
- [ ] Backpressure handling with exponential backoff
- [ ] Dynamic scaling based on queue depth

## See Also

- [Message Queue Documentation](./MESSAGE_QUEUE.md)
- [Storage Documentation](./STORAGE.md)
- [Streamer Documentation](./STREAMER.md)
- [Architecture Overview](./ARCHITECTURE.md)
