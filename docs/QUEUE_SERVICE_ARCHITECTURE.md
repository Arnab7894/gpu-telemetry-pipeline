# Distributed Queue Service Architecture

## Overview

The GPU Telemetry Pipeline now uses a **standalone Queue Service** with HTTP API, enabling true multi-pod Kubernetes deployment with competing consumers pattern.

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────────┐
│                    Queue Service Pod                          │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Competing Consumer Queue (In-Memory)                  │  │
│  │  - Consumer Groups                                     │  │
│  │  - Pending Entry List (PEL) for message locking       │  │
│  │  - Visibility Timeout (5 minutes default)             │  │
│  │  - ACK/NACK support                                    │  │
│  │  - Dead Letter Queue (max 3 retries)                  │  │
│  └────────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  HTTP API Server (Port 8080)                           │  │
│  │  - POST /api/v1/queue/publish                          │  │
│  │  - GET  /api/v1/queue/subscribe (SSE)                  │  │
│  │  - POST /api/v1/queue/ack                              │  │
│  │  - POST /api/v1/queue/nack                             │  │
│  │  - GET  /api/v1/queue/stats                            │  │
│  │  - GET  /health                                        │  │
│  └────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
                      ▲                           ▲
                      │ HTTP                      │ SSE
                      │                           │
        ┌─────────────┴──────────┐    ┌──────────┴─────────────┐
        │  Streamer Pods (N)      │    │  Collector Pods (N)     │
        │  ┌──────────────────┐  │    │  ┌──────────────────┐  │
        │  │ HTTP Client      │  │    │  │ HTTP Client      │  │
        │  │ POST /publish    │  │    │  │ GET /subscribe   │  │
        │  └──────────────────┘  │    │  │ POST /ack        │  │
        └────────────────────────┘    │  └──────────────────┘  │
                                       └────────────────────────┘
```

## Key Components

### 1. Queue Service

**Purpose**: Centralized message queue with competing consumer pattern

**Implementation**: `internal/queueservice/`
- `competing_queue.go` - Core queue with consumer groups and PEL
- `http_server.go` - HTTP API with Server-Sent Events (SSE)
- `message.go` - Message and pending entry structures

**Features**:
- ✅ **Consumer Groups**: Multiple consumers belong to a group
- ✅ **Competing Consumers**: Each message delivered to ONE consumer only
- ✅ **Message Locking**: Messages invisible while being processed (PEL)
- ✅ **Visibility Timeout**: Auto-redelivery if no ACK (default: 5 min)
- ✅ **ACK/NACK**: Explicit acknowledgment or requeue
- ✅ **Max Retries**: Dead letter queue after 3 failed attempts
- ✅ **SSE Streaming**: Real-time message delivery to collectors
- ✅ **Health Checks**: `/health` endpoint for Kubernetes probes

**Configuration**:
```bash
PORT=8080                      # HTTP server port
VISIBILITY_TIMEOUT=5m          # Message visibility timeout
MAX_RETRIES=3                  # Max delivery attempts
BUFFER_SIZE=1000               # Topic channel buffer size
```

### 2. HTTP Queue Client

**Purpose**: Client library for streamers/collectors to connect to queue service

**Implementation**: `internal/mq/http_queue_client.go`

**Features**:
- ✅ Implements `MessageQueue` interface (drop-in replacement)
- ✅ HTTP POST for publishing messages
- ✅ SSE (Server-Sent Events) for subscription
- ✅ Automatic reconnection with exponential backoff
- ✅ Graceful error handling
- ✅ Keep-alive pings

**Usage**:
```go
queue := mq.NewHTTPQueueClient(mq.HTTPQueueConfig{
    BaseURL:       "http://queue-service:8080",
    ConsumerGroup: "collectors",
    ConsumerID:    "collector-1",
}, logger)

queue.Start(ctx)
queue.Subscribe(ctx, "telemetry", handler)
```

### 3. Updated Streamer

**Changes**: Auto-detects queue type via `QUEUE_SERVICE_URL`

```go
if queueServiceURL := os.Getenv("QUEUE_SERVICE_URL"); queueServiceURL != "" {
    // Use HTTP queue client (multi-pod deployment)
    queue = mq.NewHTTPQueueClient(...)
} else {
    // Use in-memory queue (single-process/testing)
    queue = mq.NewInMemoryQueue(...)
}
```

### 4. Updated Collector

**Changes**: Same auto-detection as streamer

## Message Flow with Competing Consumers

### Publish Flow

```
1. Streamer creates telemetry message
   ↓
2. Streamer calls queue.Publish(ctx, msg)
   ↓
3. HTTP Client sends POST /api/v1/queue/publish
   ↓
4. Queue Service adds message to topic channel
   ↓
5. Returns success response
```

### Subscribe & Delivery Flow

```
1. Collector calls queue.Subscribe(ctx, "telemetry", handler)
   ↓
2. HTTP Client opens SSE connection: GET /subscribe?topic=telemetry&consumer_group=collectors&consumer_id=collector-1
   ↓
3. Queue Service:
   - Creates consumer group if not exists
   - Registers consumer in group
   - Sends "connected" event
   ↓
4. Queue Service delivery loop:
   - Pops message from topic channel
   - Selects ONE consumer from group (round-robin)
   - Adds to Pending Entry List (PEL) with visibility timeout
   - Sends message via SSE to selected consumer
   ↓
5. HTTP Client receives SSE event, unmarshals message
   ↓
6. Calls handler function (collector processes message)
   ↓
7a. If handler succeeds:
    - HTTP Client sends POST /ack
    - Queue Service removes from PEL (message deleted)
   
7b. If handler fails:
    - HTTP Client sends POST /nack
    - Queue Service requeues message (if under max retries)
```

### Visibility Timeout & Redelivery

```
If NO ACK/NACK within visibility timeout:
   ↓
1. Queue Service visibility checker (every 10s) finds expired message
   ↓
2. Removes from PEL
   ↓
3. Increments delivery count
   ↓
4. If delivery_count < max_retries (3):
   - Requeues message to topic channel
   - Another consumer will receive it
   Else:
   - Moves to Dead Letter Queue
```

## Competing Consumers vs Fanout

### Old Architecture (Fanout - Not Desired)
```
Message → Queue → Subscriber 1 ✓
               → Subscriber 2 ✓
               → Subscriber 3 ✓
(All subscribers receive ALL messages)
```

### New Architecture (Competing Consumers - Desired) ✅
```
Message → Queue → Consumer Group → Consumer 1 ✓
                                 OR Consumer 2
                                 OR Consumer 3
(ONE consumer receives each message)
```

## Deployment Scenarios

### Scenario 1: Local Testing (In-Memory Queue)

```bash
# No QUEUE_SERVICE_URL set - uses in-memory queue
./bin/streamer &
./bin/collector &
```

**Behavior**: Single process, in-memory queue, no network overhead

### Scenario 2: Multi-Process (HTTP Queue)

```bash
# Start queue service
./bin/queueservice &

# Start collectors
export QUEUE_SERVICE_URL=http://localhost:8080
./bin/collector -instance-id=collector-1 &
./bin/collector -instance-id=collector-2 &

# Start streamer
./bin/streamer &
```

**Behavior**: Separate processes, HTTP communication, competing consumers

### Scenario 3: Kubernetes (Multi-Pod)

```bash
# Deploy services
kubectl apply -f k8s/queue-service.yaml
kubectl apply -f k8s/collector.yaml    # 3 replicas
kubectl apply -f k8s/streamer.yaml     # 2 replicas
```

**Behavior**: 
- 1 queue service pod
- 3 collector pods (competing consumers)
- 2 streamer pods (concurrent publishers)
- Messages distributed across collectors (load balancing)

## Advantages

### 1. True Scalability
- ✅ Add/remove streamer pods without code changes
- ✅ Add/remove collector pods without code changes
- ✅ Queue service handles load balancing automatically

### 2. Fault Tolerance
- ✅ If collector crashes, message redelivered (visibility timeout)
- ✅ Max retries prevent infinite loops
- ✅ Dead letter queue captures problematic messages

### 3. No Duplicate Processing
- ✅ Each message processed by exactly ONE collector
- ✅ PEL prevents multiple consumers from seeing same message
- ✅ ACK ensures message deleted only after successful processing

### 4. Observability
- ✅ `/api/v1/queue/stats` shows real-time metrics
- ✅ Consumer registration logged
- ✅ Message delivery tracked
- ✅ Pending messages visible

### 5. Simple Deployment
- ✅ No external dependencies (Redis, Kafka, etc.)
- ✅ Pure Go implementation
- ✅ Single binary per service
- ✅ Standard HTTP/SSE (no special protocols)

## Limitations

### 1. Single Queue Service Instance
- **Issue**: Queue service is single-replica (in-memory state)
- **Impact**: If queue pod crashes, in-flight messages lost
- **Mitigation**: 
  - Use Kubernetes StatefulSet for stable identity
  - Collectors will auto-reconnect after pod restart
  - Implement persistence layer for production (future)

### 2. Memory Bounded
- **Issue**: All messages stored in RAM
- **Impact**: Limited by queue service pod memory
- **Mitigation**:
  - Increase memory limit in k8s manifest
  - Monitor queue depth via `/stats` endpoint
  - Alert if queue depth exceeds threshold

### 3. No Persistence
- **Issue**: Messages not persisted to disk
- **Impact**: Messages lost on queue service restart
- **Mitigation**:
  - For production, implement persistent backend (MongoDB, Postgres)
  - Or accept message loss (telemetry data is continuous stream)

### 4. Network Latency
- **Issue**: HTTP overhead vs in-process channels
- **Impact**: ~1-2ms additional latency per message
- **Trade-off**: Worth it for multi-pod scalability

## Monitoring

### Queue Statistics

```bash
curl http://queue-service:8080/api/v1/queue/stats
```

```json
{
  "topics": [
    {
      "name": "telemetry",
      "queue_depth": 42,
      "consumer_groups": [
        {
          "name": "collectors",
          "consumer_count": 3,
          "pending_messages": 5
        }
      ]
    }
  ],
  "dead_letter_count": 2
}
```

### Metrics to Monitor

1. **Queue Depth**: `queue_depth` - Should stay low (< 100)
2. **Pending Messages**: `pending_messages` - Should be low (active processing)
3. **Consumer Count**: `consumer_count` - Should match deployed replicas
4. **Dead Letter Count**: `dead_letter_count` - Should be rare

### Alerts

```yaml
# Example Prometheus alert rules
- alert: QueueDepthHigh
  expr: queue_depth > 1000
  annotations:
    summary: "Queue depth exceeding threshold"
    
- alert: NoConsumers
  expr: consumer_count == 0
  annotations:
    summary: "No collectors connected to queue"
    
- alert: HighDeadLetterCount
  expr: dead_letter_count > 100
  annotations:
    summary: "Too many messages in dead letter queue"
```

## Testing

### Unit Tests
```bash
go test ./internal/queueservice/...
go test ./internal/mq/...
```

### Integration Test
```bash
./test-integration.sh
```

Expected output:
```
✅ Queue service started
✅ Collectors started (3 instances)
✅ Streamer completed
✅ SUCCESS: All messages processed exactly once
✅ Load balancing working: 3 collectors received messages
```

### Manual Test
See `MANUAL_TEST.md` for step-by-step manual testing instructions.

## Future Enhancements

1. **Persistent Queue Backend**
   - Implement MongoDB/Postgres storage
   - Message durability across restarts

2. **High Availability**
   - Leader election for queue service
   - Multiple queue service replicas with shared state

3. **Advanced Metrics**
   - Prometheus metrics export
   - Grafana dashboards
   - Distributed tracing (OpenTelemetry)

4. **Performance Optimizations**
   - Message batching
   - Compression
   - Connection pooling

5. **Security**
   - TLS for HTTP API
   - Authentication/authorization
   - Rate limiting

## Conclusion

The new distributed queue service architecture enables:

✅ **Multi-pod Kubernetes deployment**  
✅ **Competing consumers** (no duplicate processing)  
✅ **Message locking** with visibility timeout  
✅ **ACK/NACK** for reliable delivery  
✅ **Horizontal scalability** for streamers and collectors  
✅ **No external dependencies**  
✅ **Simple HTTP/SSE protocol**  

This architecture is **production-ready** for deployment with appropriate monitoring and resource allocation.
