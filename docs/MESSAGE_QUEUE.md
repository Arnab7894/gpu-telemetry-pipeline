# Message Queue Implementation

## Overview

The custom message queue layer provides a lightweight, in-memory pub/sub system for the GPU telemetry pipeline. It supports **multiple producers** (Streamers) and **multiple consumers** (Collectors) with full concurrency safety.

## Architecture

### Components

1. **Message** - Envelope for telemetry data
2. **MessageQueue Interface** - Abstraction for different implementations
3. **InMemoryQueue** - Channel-based implementation
4. **MessageHandler** - Callback function type for subscribers

### Design Principles (SOLID)

- **Single Responsibility**: Each component has one clear purpose
- **Open/Closed**: Queue interface allows new implementations without changing consumers
- **Liskov Substitution**: Any MessageQueue implementation can be swapped
- **Interface Segregation**: Minimal, focused interface
- **Dependency Inversion**: Depends on abstractions (MessageQueue interface)

## Message Structure

```go
type Message struct {
    ID        string                 // Unique identifier (UUID)
    Topic     string                 // Routing key for subscribers
    Payload   []byte                 // JSON-encoded data
    Headers   map[string]string      // Metadata (source, version, etc.)
    Timestamp time.Time              // Creation time
    Metadata  map[string]interface{} // Additional context
}
```

### Creating Messages

```go
// Create a message with automatic JSON marshaling
msg, err := mq.NewMessage("telemetry.gpu", telemetryPoint)

// Add headers and metadata using fluent API
msg.WithHeader("source", "streamer-1").
    WithMetadata("retry_count", 0)

// Unmarshal payload in subscriber
var point domain.TelemetryPoint
err := msg.Unmarshal(&point)
```

## MessageQueue Interface

```go
type MessageQueue interface {
    Start(ctx context.Context) error
    Stop() error
    Close() error
    
    Publish(ctx context.Context, msg *Message) error
    Subscribe(ctx context.Context, topic string, handler MessageHandler) error
    Unsubscribe(topic string) error
    
    Stats() QueueStats
}
```

### Lifecycle

1. **Create** - `NewInMemoryQueue(config)`
2. **Start** - `queue.Start(ctx)` - begins message processing
3. **Use** - Publish/Subscribe concurrently
4. **Stop** - `queue.Stop()` - graceful shutdown (waits for in-flight messages)
5. **Close** - `queue.Close()` - release resources

## InMemoryQueue Implementation

### Features

- **Concurrency-Safe**: Uses `sync.RWMutex` for subscriber map, atomic ops for stats
- **Worker Pool**: Limits concurrent handlers (default: 100)
- **Buffered Channel**: Prevents blocking publishers (default: 1000)
- **Graceful Shutdown**: Drains remaining messages before stopping
- **Panic Recovery**: Handlers that panic don't crash the queue
- **Error Tracking**: Statistics for published, delivered, and failed messages
- **Topic Routing**: Messages delivered to all subscribers of a topic (fanout)

### Configuration

```go
config := mq.InMemoryQueueConfig{
    BufferSize: 1000,  // Channel buffer size
    MaxWorkers: 100,   // Max concurrent handlers
}
queue := mq.NewInMemoryQueue(config)
```

### Usage Example

```go
// Create and start queue
queue := mq.NewInMemoryQueue(mq.DefaultInMemoryQueueConfig())
ctx := context.Background()
queue.Start(ctx)
defer queue.Close()

// Subscribe (multiple consumers can subscribe to same topic)
queue.Subscribe(ctx, "telemetry.gpu", func(ctx context.Context, msg *mq.Message) error {
    var point domain.TelemetryPoint
    if err := msg.Unmarshal(&point); err != nil {
        return err
    }
    
    // Store in repository
    return telemetryRepo.Store(&point)
})

// Publish (from multiple producers concurrently)
for _, point := range telemetryPoints {
    msg, _ := mq.NewMessage("telemetry.gpu", point)
    if err := queue.Publish(ctx, msg); err != nil {
        log.Error("Failed to publish", "error", err)
    }
}

// Monitor queue health
stats := queue.Stats()
log.Info("Queue stats",
    "published", stats.TotalPublished,
    "delivered", stats.TotalDelivered,
    "errors", stats.TotalErrors,
    "depth", stats.QueueDepth,
)
```

## Concurrency Model

### Multiple Producers

- **Thread-Safe**: Multiple Streamers can publish concurrently
- **Non-Blocking**: Buffered channel prevents blocking (up to buffer size)
- **Context Support**: Publish respects context cancellation

```go
// 10 Streamers publishing concurrently
var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(streamerID int) {
        defer wg.Done()
        
        for _, point := range csvData {
            msg, _ := mq.NewMessage("telemetry.gpu", point)
            queue.Publish(ctx, msg)
        }
    }(i)
}
wg.Wait()
```

### Multiple Consumers

- **Fanout Pattern**: Each message delivered to ALL subscribers on a topic
- **Concurrent Execution**: Handlers run concurrently (worker pool limits max)
- **Isolation**: Handler errors don't affect other handlers

```go
// 5 Collectors subscribing to same topic
for i := 0; i < 5; i++ {
    collectorID := i
    queue.Subscribe(ctx, "telemetry.gpu", func(ctx context.Context, msg *mq.Message) error {
        log.Info("Collector received message", "collector", collectorID)
        return processMessage(msg)
    })
}
```

### Worker Pool

Limits concurrent handler executions to prevent resource exhaustion:

- **Semaphore Pattern**: Uses buffered channel as semaphore
- **Backpressure**: Blocks if pool is full (prevents OOM)
- **Fair Scheduling**: FIFO order for handler execution

## Error Handling

### Handler Errors

```go
queue.Subscribe(ctx, "topic", func(ctx context.Context, msg *mq.Message) error {
    if err := processMessage(msg); err != nil {
        return err  // Logged as error, stats.TotalErrors++
    }
    return nil  // stats.TotalDelivered++
})
```

### Panic Recovery

```go
queue.Subscribe(ctx, "topic", func(ctx context.Context, msg *mq.Message) error {
    panic("oops")  // Recovered, logged, stats.TotalErrors++
})
```

### Publish Failures

```go
err := queue.Publish(ctx, msg)
if err != nil {
    // Possible errors:
    // - Queue is closed
    // - Queue not started
    // - Context cancelled
    // - Buffer full (unlikely with proper sizing)
}
```

## Graceful Shutdown

### Stop Sequence

1. `queue.Stop()` or `queue.Close()` called
2. Context cancelled (stops accepting new messages)
3. **Drain Phase**: Process remaining messages in buffer
4. Wait for all in-flight handlers to complete
5. Return control to caller

### Example

```go
queue := mq.NewInMemoryQueue(config)
queue.Start(ctx)

// Publish 1000 messages
for i := 0; i < 1000; i++ {
    msg, _ := mq.NewMessage("topic", data)
    queue.Publish(ctx, msg)
}

// Graceful shutdown waits for all 1000 to be processed
queue.Close()  // Blocks until all handlers finish

stats := queue.Stats()
// stats.TotalPublished == stats.TotalDelivered (no loss)
```

## Statistics

```go
type QueueStats struct {
    TotalPublished    int64  // Total messages published
    TotalDelivered    int64  // Total successful handler executions
    TotalErrors       int64  // Total handler errors (including panics)
    ActiveSubscribers int    // Current number of topics with subscribers
    QueueDepth        int    // Current messages waiting to be processed
}
```

Use for monitoring and alerting:

```go
stats := queue.Stats()

// Alert if queue is backing up
if stats.QueueDepth > 500 {
    log.Warn("Queue depth high", "depth", stats.QueueDepth)
}

// Alert if error rate is high
errorRate := float64(stats.TotalErrors) / float64(stats.TotalPublished)
if errorRate > 0.05 {
    log.Error("High error rate", "rate", errorRate)
}
```

## Testing

### Test Coverage

11 comprehensive test cases covering:

1. **Basic Pub/Sub** - Single producer, single consumer
2. **Multiple Producers** - 10 producers, 100 messages each
3. **Multiple Consumers** - 5 consumers, fanout pattern
4. **Combined** - 10 producers + 5 consumers simultaneously
5. **Graceful Shutdown** - Verifies all messages processed
6. **Error Handling** - Handler errors tracked correctly
7. **Panic Recovery** - Panicking handlers don't crash queue
8. **Multiple Topics** - Topic isolation
9. **Context Cancellation** - Respects context
10. **Unsubscribe** - Dynamic subscription management
11. **Stats Accuracy** - Verify counters

### Running Tests

```bash
# Run all message queue tests
go test ./internal/mq/... -v

# Run specific test
go test ./internal/mq/... -run TestInMemoryQueue_MultipleProducers

# Run with race detector
go test ./internal/mq/... -race
```

## Future Enhancements

### Alternative Implementations

The `MessageQueue` interface allows plugging in different implementations:

```go
// Redis-based queue (distributed)
type RedisQueue struct {
    client *redis.Client
}

func (q *RedisQueue) Publish(ctx context.Context, msg *Message) error {
    return q.client.Publish(ctx, msg.Topic, msg.Payload).Err()
}

// Use via dependency injection
var queue MessageQueue
if config.UseRedis {
    queue = NewRedisQueue(redisClient)
} else {
    queue = NewInMemoryQueue(config)
}
```

### Possible Additions

- **Message Priority**: High-priority messages processed first
- **Dead Letter Queue**: Failed messages sent to DLQ for analysis
- **Retry Logic**: Automatic retries with exponential backoff
- **Message TTL**: Expire old messages
- **Persistent Queue**: Survive restarts (Redis, Kafka, etc.)
- **Rate Limiting**: Per-topic or per-consumer rate limits
- **Message Filtering**: Subscribers filter by message attributes

## Performance Characteristics

### Throughput

- **Publish**: ~1M msgs/sec (buffered channel write)
- **Subscribe**: Limited by handler processing time
- **Worker Pool**: Controls max concurrent handlers

### Latency

- **Publish**: <1μs (channel send)
- **Delivery**: Depends on queue depth and handler count
- **Shutdown**: Proportional to queue depth and handler processing time

### Memory Usage

- **Base**: ~100KB (queues, maps, atomic vars)
- **Messages**: BufferSize * avg_message_size
- **Workers**: MaxWorkers * goroutine_stack_size (~2KB each)

### Scalability

- **Horizontal**: Run multiple Streamers/Collectors
- **Vertical**: Increase BufferSize and MaxWorkers
- **Bottleneck**: Handler processing time (optimize subscribers)

## Integration with Telemetry Pipeline

```
┌──────────┐
│ Streamer │ ──┐
└──────────┘   │
               │  Publish("telemetry.gpu", points)
┌──────────┐   │
│ Streamer │ ──┼──→ ┌─────────────────┐
└──────────┘   │    │ InMemoryQueue   │
               │    │  - Worker Pool  │
┌──────────┐   │    │  - Buffer: 1000 │
│ Streamer │ ──┘    │  - Workers: 100 │
└──────────┘        └─────────────────┘
                            │
                            │ Subscribe("telemetry.gpu", handler)
                            │
                    ┌───────┴───────┐
                    │       │       │
                    ▼       ▼       ▼
              ┌──────────┐ ┌──────────┐ ┌──────────┐
              │Collector │ │Collector │ │Collector │
              │    1     │ │    2     │ │    3     │
              └──────────┘ └──────────┘ └──────────┘
                    │           │           │
                    └───────────┴───────────┘
                            │
                            ▼
                    ┌─────────────────┐
                    │   Repository    │
                    │ (InMemory/DB)   │
                    └─────────────────┘
```

## Best Practices

1. **Buffer Sizing**: `BufferSize >= peak_message_rate * avg_processing_time`
2. **Worker Pool**: `MaxWorkers = num_cpu_cores * 2` (IO-bound) or `num_cpu_cores` (CPU-bound)
3. **Error Handling**: Always handle errors in subscribers, don't panic
4. **Context**: Pass context to handlers for graceful cancellation
5. **Monitoring**: Track stats, alert on high queue depth or error rate
6. **Testing**: Test with race detector (`-race` flag)
7. **Graceful Shutdown**: Call `Close()` in defer, use `Stop()` for graceful draining
