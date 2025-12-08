# Distributed Message Queue Design for Multi-Pod Deployment

## Current Limitations

The existing `InMemoryQueue` implementation has the following limitations for Kubernetes deployment:

### ❌ What Doesn't Work
1. **Process Isolation**: Queue exists only in single process memory
2. **No Pod-to-Pod Communication**: Separate pods cannot share the queue
3. **Fanout Pattern**: All subscribers receive all messages (not competing consumers)
4. **No Persistence**: Messages lost on pod restart
5. **No Message Locking**: No visibility timeout for concurrent processing
6. **No ACK/NACK**: No acknowledgment mechanism for message completion

### ✅ What's Needed for Multi-Pod Kubernetes Deployment
1. **Shared Queue**: All streamer/collector pods connect to same queue instance
2. **Competing Consumers**: Each message delivered to exactly ONE collector
3. **Message Locking**: Message invisible to others while being processed
4. **Visibility Timeout**: Message redelivered if not ACKed within timeout
5. **ACK/NACK**: Explicit acknowledgment to delete or requeue messages
6. **Persistence**: Messages survive pod restarts
7. **Scalability**: Handle multiple streamer and collector replicas

## Solution Options

### Option 1: Redis Streams (Recommended) ⭐

**Pros:**
- ✅ Consumer groups (competing consumers)
- ✅ Message acknowledgment (XACK)
- ✅ Persistence with AOF/RDB
- ✅ Pending messages tracking
- ✅ Dead letter queue via maxretries
- ✅ Fast (in-memory with persistence)
- ✅ Simple deployment (single Redis pod)
- ✅ Low operational overhead

**Implementation:**
```go
// Redis Streams with Consumer Groups
XGROUP CREATE telemetry-stream collectors $ MKSTREAM
XADD telemetry-stream * data <json>           // Streamer publishes
XREADGROUP GROUP collectors consumer-1 STREAMS telemetry-stream > // Collector reads
XACK telemetry-stream collectors <msg-id>     // Collector ACKs
```

**Architecture:**
```
Streamer Pod 1 ──┐
Streamer Pod 2 ──┼──► Redis Stream ──┬──► Collector Pod 1
Streamer Pod 3 ──┘    (telemetry)    ├──► Collector Pod 2
                                      └──► Collector Pod 3
                      Consumer Group: "collectors"
```

### Option 2: MongoDB Change Streams

**Pros:**
- ✅ Already using MongoDB for other purposes
- ✅ Change streams for pub/sub
- ✅ Persistent by design
- ✅ Queries on message metadata

**Cons:**
- ❌ More complex consumer group implementation
- ❌ Manual visibility timeout handling
- ❌ Higher latency than Redis

### Option 3: RabbitMQ

**Pros:**
- ✅ Battle-tested message queue
- ✅ Competing consumers built-in
- ✅ ACK/NACK support
- ✅ Dead letter exchanges

**Cons:**
- ❌ Additional service to deploy
- ❌ More complex than Redis
- ❌ Higher resource usage

### Option 4: Apache Kafka

**Pros:**
- ✅ High throughput
- ✅ Consumer groups
- ✅ Persistent log

**Cons:**
- ❌ Overkill for this use case
- ❌ Complex deployment (Zookeeper/KRaft)
- ❌ Higher operational overhead

## Recommended Implementation: Redis Streams

### Architecture

```yaml
# Kubernetes Deployment
apiVersion: v1
kind: Service
metadata:
  name: redis
spec:
  selector:
    app: redis
  ports:
  - port: 6379
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        args: ["--appendonly", "yes"]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: streamer
spec:
  replicas: 3  # Multiple streamer pods
  template:
    spec:
      containers:
      - name: streamer
        env:
        - name: REDIS_URL
          value: "redis:6379"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: collector
spec:
  replicas: 5  # Multiple collector pods
  template:
    spec:
      containers:
      - name: collector
        env:
        - name: REDIS_URL
          value: "redis:6379"
```

### Message Flow

```
1. Streamer publishes to Redis Stream
   XADD telemetry-stream * payload <json>
   
2. Collector reads from consumer group (competing consumers)
   XREADGROUP GROUP collectors consumer-1 COUNT 1 BLOCK 1000 STREAMS telemetry-stream >
   
3. Message locked (invisible to other collectors)
   - Pending Entry List (PEL) tracks it
   - Other collectors won't see it
   
4. Collector processes message
   - Validate, store to repositories
   
5. Collector ACKs message
   XACK telemetry-stream collectors <msg-id>
   
6. Message removed from PEL (deleted effectively)

If collector crashes before ACK:
- Message stays in PEL
- After visibility timeout, can be claimed by another collector
- XCLAIM/XAUTOCLAIM for stale messages
```

### Key Features

**Competing Consumers:**
```go
// Each collector joins the same consumer group
// Redis ensures each message goes to only ONE consumer
XREADGROUP GROUP collectors consumer-1 ...
XREADGROUP GROUP collectors consumer-2 ...
XREADGROUP GROUP collectors consumer-3 ...
```

**Message Locking:**
```go
// When consumer-1 reads a message, it's added to PEL
// Other consumers won't see it in XREADGROUP
// Message is "locked" until ACKed or timeout
```

**Acknowledgment:**
```go
// Success: Delete from PEL
XACK telemetry-stream collectors <msg-id>

// Failure: Message stays in PEL for retry
// Or explicitly XDEL to discard
```

**Visibility Timeout:**
```go
// Claim stale messages (not ACKed after timeout)
XAUTOCLAIM telemetry-stream collectors consumer-1 300000 ...
// 300000ms = 5 minutes
```

## Implementation Plan

### Phase 1: Redis Queue Interface
1. Define `MessageQueue` interface (already exists)
2. Implement `RedisQueue` implementing `MessageQueue`
3. Use `go-redis/redis/v8` client
4. Consumer groups setup
5. XADD for Publish
6. XREADGROUP for Subscribe
7. XACK for acknowledgment

### Phase 2: Update Streamer
1. Replace `InMemoryQueue` with `RedisQueue`
2. No code changes needed (implements same interface)
3. Configure Redis URL via env var

### Phase 3: Update Collector
1. Replace `InMemoryQueue` with `RedisQueue`
2. Add explicit ACK after successful processing
3. Add NACK/retry logic for failures
4. Handle visibility timeout claims

### Phase 4: Testing
1. Unit tests with miniredis (in-memory Redis for tests)
2. Integration tests with real Redis
3. Multi-pod simulation (docker-compose)
4. Kubernetes deployment tests

### Phase 5: Migration
1. Feature flag to switch between InMemory and Redis
2. Backward compatibility for single-process mode
3. Documentation updates

## Code Structure

```
internal/mq/
├── queue.go              # MessageQueue interface (unchanged)
├── inmemory_queue.go     # Existing implementation (keep for tests)
└── redis_queue.go        # New: Redis Streams implementation

cmd/streamer/main.go      # Use Redis queue when REDIS_URL set
cmd/collector/main.go     # Use Redis queue when REDIS_URL set
```

## Configuration

```bash
# Environment Variables
REDIS_URL=redis:6379           # Redis server
REDIS_STREAM=telemetry-stream  # Stream name
REDIS_GROUP=collectors         # Consumer group
REDIS_CONSUMER_ID=collector-1  # Unique per pod
VISIBILITY_TIMEOUT=300000      # 5 minutes in ms
MAX_RETRIES=3                  # Dead letter after 3 failures
```

## Benefits

1. **True Horizontal Scaling**: Add more pods without code changes
2. **Load Balancing**: Redis distributes messages across consumers
3. **Fault Tolerance**: Messages survive pod crashes
4. **No Duplicate Processing**: Each message processed exactly once
5. **Backpressure Handling**: Collectors pull at their own rate
6. **Monitoring**: Redis INFO, XPENDING for observability
7. **Simple Deployment**: One Redis pod supports all streamers/collectors

## Next Steps

Would you like me to implement the Redis Streams message queue? This will enable:
- ✅ Multi-pod deployment
- ✅ Competing consumers (one message → one collector)
- ✅ Message locking with visibility timeout
- ✅ ACK/NACK for reliable processing
- ✅ Kubernetes-ready architecture
