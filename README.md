# GPU Telemetry Pipeline

A scalable, distributed telemetry pipeline for monitoring GPU metrics in Kubernetes clusters. The system streams GPU telemetry data from CSV files, processes it through a distributed message queue, persists it to MongoDB, and exposes REST APIs for querying historical telemetry data.

---

## Table of Contents

- [System Overview](#system-overview)
- [Architecture](#architecture)
- [Design Considerations](#design-considerations)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
  - [Option 1: Local Deployment (Recommended)](#option-1-local-deployment-recommended)
  - [Option 2: Existing Kubernetes Cluster](#option-2-existing-kubernetes-cluster)
- [API Documentation](#api-documentation)
- [User Workflow](#user-workflow)
- [Project Structure](#project-structure)
- [Configuration](#configuration)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)
- [AI Assistance](#ai-assistance)

---

## System Overview

The GPU Telemetry Pipeline is designed to handle high-volume GPU metrics collection and querying in containerized AI/ML workloads. It provides:

- **Real-time streaming** of GPU metrics from CSV data sources
- **Distributed message queue** with competing consumers pattern (Redis-backed)
- **Persistent storage** in MongoDB for historical telemetry data
- **REST API** for querying GPU information and telemetry history
- **Horizontal scalability** - independently scale streamers, collectors, and API instances
- **Kubernetes-native** - Helm charts for easy deployment and management

---

## Architecture

### High-Level Component Diagram

```
┌──────────────────────────────────────────────────────────┐
│                   Kubernetes Cluster                     │
│                                                          │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐  │
│  │  Streamer    │   │ Queue Service│   │  Collector   │  │
│  │  Pods (N)    │──▶│ (Redis)      │◀──│  Pods (N)    │  │
│  │              │   │              │   │              │  │
│  │ • Read CSV   │   │ • Redis List │   │ • Consume    │  │
│  │ • Parse      │   │ • ACK/NACK   │   │ • Process    │  │
│  │ • Publish    │   │ • Competing  │   │ • Store      │  │
│  └──────────────┘   │   Consumers  │   └──────┬───────┘  │
│                     └──────────────┘          │          │
│                                               │          │
│                                               ▼          │
│                                       ┌──────────────┐   │
│                                       │   MongoDB    │   │
│  ┌──────────────┐                     │              │   │
│  │ API Gateway  │                     │ • Telemetry  │   │
│  │  Pods (N)    │◀────────────────────│   Storage    │   │
│  │              │                     │ • GPU Info   │   │
│  │ • REST API   │                     └──────────────┘   │
│  │ • Query      │                                        │
│  │ • Swagger UI │                                        │
│  └──────┬───────┘                                        │
│         │                                                │
└─────────┼────────────────────────────────────────────────┘
          │
          ▼
    External Users
 (http://localhost:8080)
```

### Component Responsibilities

1. **Streamer Service**
   - Reads GPU metrics from CSV files
   - Parses DCGM metrics format
   - Publishes messages to Redis queue
   - Supports loop mode for continuous streaming
   - Horizontally scalable (multiple replicas)

2. **Queue Service (Redis)**
   - Distributed message queue using Redis Lists
   - Competing consumers pattern (each message to ONE collector)
   - Message acknowledgment (ACK/NACK)
   - Visibility timeout for message redelivery
   - Dead letter queue for failed messages

3. **Collector Service**
   - Subscribes to telemetry messages from queue
   - Processes messages in batches
   - Stores telemetry data in MongoDB
   - Worker pool for concurrent processing
   - Horizontally scalable (multiple replicas)

4. **API Gateway Service**
   - REST API for querying GPU data
   - Lists all GPUs with metadata
   - Retrieves telemetry history with time filters
   - Built-in Swagger UI for API exploration
   - Health checks for Kubernetes probes
   - Horizontally scalable (multiple replicas)

5. **MongoDB**
   - Persistent storage for telemetry data
   - Collections: `gpus`, `telemetry_points`
   - Indexes for efficient time-range queries
   - Composite key: (GPU UUID + timestamp)

---

## Design Considerations

### 1. **Scalability**
- **Independent scaling**: Each component (streamer, collector, API) can scale independently
- **Competing consumers**: Multiple collectors process messages in parallel without duplication
- **Batch processing**: Collectors process telemetry in configurable batches for efficiency
- **Connection pooling**: MongoDB connections pooled across API instances

### 2. **Reliability**
- **Message acknowledgment**: ACK/NACK ensures no message loss
- **Visibility timeout**: Unprocessed messages automatically redelivered
- **Dead letter queue**: Failed messages moved to DLQ after max retries
- **Health checks**: Kubernetes liveness/readiness probes for all services

### 3. **Extensibility**
- **Repository pattern**: Storage layer abstracted for future extensions
- **Interface-driven**: Components depend on abstractions, not implementations
- **Configuration-driven**: Environment variables for all tunable parameters
- **Helm charts**: Templated Kubernetes manifests for easy customization

### 4. **Design Patterns**
- **Repository Pattern**: Abstract storage layer (`GPURepository`, `TelemetryRepository`)
- **Publisher-Subscriber**: Message queue architecture
- **Factory Pattern**: Queue and storage instantiation
- **Competing Consumers**: Load distribution across collector instances
- **Adapter Pattern**: CSV to domain model conversion

### 5. **SOLID Principles**
- **Single Responsibility**: Each component has focused purpose
- **Open/Closed**: Extensible via interfaces without modifying existing code
- **Liskov Substitution**: Interface implementations are interchangeable
- **Interface Segregation**: Small, focused interfaces (e.g., `MessageQueue`, `Repository`)
- **Dependency Inversion**: Depend on abstractions, not concretions

---

## Prerequisites

Before you begin, ensure you have the following installed:

- **Docker** (v20.10+) - [Install Docker](https://docs.docker.com/get-docker/)
- **kubectl** (v1.19+) - [Install kubectl](https://kubernetes.io/docs/tasks/tools/)
- **Helm** (v3.x) - [Install Helm](https://helm.sh/docs/intro/install/)

**For local deployment:**
- **kind** (Kubernetes in Docker) - [Install kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)

**For development:**
- **Go** (v1.21+) - [Install Go](https://go.dev/doc/install)
- **Make** - Usually pre-installed on macOS/Linux

---

## Quick Start

Choose one of the following deployment options:

### Option 1: Local Deployment (Recommended)

This option creates a local Kubernetes cluster using `kind`, builds Docker images, loads them into the cluster, and deploys all services.

```bash
# Run the automated local deployment
make local-deploy
```

**What this does:**
1. ✅ Checks prerequisites (Docker, kind, kubectl, Helm)
2. ✅ Builds all Docker images
3. ✅ Creates a kind cluster named `gpu-telemetry-cluster`
4. ✅ Loads Docker images into the kind cluster
5. ✅ Installs MongoDB, Redis, and all application services via Helm
6. ✅ Waits for all pods to be ready
7. ✅ Sets up port forwarding to `localhost:8080`

**Expected output:**
```
🎉 Deployment Complete!

Verification Commands:
  1. Check pods status:
     kubectl get pods -n default

  2. Test API Gateway health:
     curl http://localhost:8080/health

  3. List all GPUs:
     curl http://localhost:8080/api/v1/gpus | jq
```

**To clean up:**
```bash
make local-cleanup
```

---

### Option 2: Existing Kubernetes Cluster

Use this option if you already have a Kubernetes cluster (EKS, GKE, AKS, etc.) and want to deploy there.

**Step 1: Connect to your cluster**
```bash
# Verify you're connected to the correct cluster
kubectl cluster-info
kubectl config current-context

# If needed, switch context
kubectl config use-context <your-context>
```

**Step 2: Push Docker images to a registry**

For remote clusters, images must be in a container registry accessible by your cluster.

```bash
# Option A: Docker Hub
make k8s-deploy REGISTRY=yourusername TAG=v1.0.0

# Option B: Google Container Registry
make k8s-deploy REGISTRY=gcr.io/your-project-id TAG=v1.0.0

# Option C: AWS Elastic Container Registry
make k8s-deploy REGISTRY=123456789.dkr.ecr.us-east-1.amazonaws.com TAG=v1.0.0
```

**Step 3: Deploy to Kubernetes**
```bash
make k8s-deploy REGISTRY=<your-registry> TAG=<tag>
```

**What this does:**
1. ✅ Validates Docker is running
2. ✅ Checks Kubernetes cluster connection
3. ✅ Detects if using kind (recommends `local-deploy` instead)
4. ✅ Builds Docker images
5. ✅ Pushes images to your registry
6. ✅ Deploys MongoDB, Redis, and all services via Helm
7. ✅ Sets up port forwarding to `localhost:8080`

---

## API Documentation

### Generate Swagger Documentation

The project includes OpenAPI/Swagger specifications for the REST API.

```bash
# Generate Swagger docs
make openapi
```

**Generated files:**
- `docs/swagger/swagger.json` - OpenAPI 2.0 in JSON
- `docs/swagger/swagger.yaml` - OpenAPI 2.0 in YAML
- `docs/swagger/docs.go` - Go documentation

### Access Swagger UI

After deployment, access the interactive Swagger UI:

```bash
# If port-forwarding is not active, start it:
kubectl port-forward service/api-gateway 8080:8080

# Open in browser:
# http://localhost:8080/swagger/index.html
```

### API Endpoints

#### 1. Health Check
```bash
GET /health

curl http://localhost:8080/health
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2025-12-12T10:30:00Z"
}
```

#### 2. List All GPUs
```bash
GET /api/v1/gpus

curl http://localhost:8080/api/v1/gpus | jq
```

**Response:**
```json
{
  "gpus": [
    {
      "uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
      "device_id": "nvidia0",
      "gpu_index": "0",
      "model_name": "NVIDIA H100 80GB HBM3",
      "hostname": "mtv5-dgx1-hgpu-031",
      "container": "gpu-workload",
      "pod": "training-pod-1",
      "namespace": "ml-team"
    }
  ],
  "total": 1
}
```

#### 3. Get GPU Telemetry
```bash
GET /api/v1/gpus/{uuid}/telemetry?start_time={RFC3339}&end_time={RFC3339}

# Example: Get telemetry for specific GPU
GPU_UUID="GPU-5fd4f087-86f3-7a43-b711-4771313afc50"
curl "http://localhost:8080/api/v1/gpus/${GPU_UUID}/telemetry" | jq

# Example: With time filters
curl "http://localhost:8080/api/v1/gpus/${GPU_UUID}/telemetry?start_time=2025-12-12T04:00:00Z&end_time=2025-12-12T05:00:00Z" | jq
```

**Response:**
```json
{
  "gpu_uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
  "telemetry": [
    {
      "metric_name": "DCGM_FI_DEV_GPU_TEMP",
      "value": 65.0,
      "timestamp": "2025-12-12T04:15:32Z"
    },
    {
      "metric_name": "DCGM_FI_DEV_POWER_USAGE",
      "value": 320.5,
      "timestamp": "2025-12-12T04:15:32Z"
    }
  ],
  "count": 2
}
```

---

## User Workflow

### Step-by-Step Guide for New Users

**1. Deploy the system**

```bash
# For local testing with kind
make local-deploy

# Wait for deployment to complete (~2-3 minutes)
```

**2. Verify deployment**

```bash
# Check all pods are running
kubectl get pods

# Expected output:
# NAME                          READY   STATUS    RESTARTS   AGE
# api-gateway-xxx               1/1     Running   0          2m
# collector-xxx                 1/1     Running   0          2m
# mongodb-0                     1/1     Running   0          2m
# queue-service-0               1/1     Running   0          2m
# redis-xxx                     1/1     Running   0          2m
# streamer-xxx                  1/1     Running   0          2m
```

**3. Test API health**

```bash
curl http://localhost:8080/health

# Expected: {"status":"healthy","timestamp":"..."}
```

**4. Explore Swagger UI**

Open your browser to: `http://localhost:8080/swagger/index.html`

- Try the `/api/v1/gpus` endpoint
- Click "Try it out" → "Execute"
- View the response with all GPU information

**5. Query GPU telemetry**

```bash
# List all GPUs and get a UUID
curl http://localhost:8080/api/v1/gpus | jq '.gpus[0].uuid'

# Get telemetry for that GPU
GPU_UUID="<paste-uuid-here>"
curl "http://localhost:8080/api/v1/gpus/${GPU_UUID}/telemetry" | jq

# Filter by time range
curl "http://localhost:8080/api/v1/gpus/${GPU_UUID}/telemetry?start_time=2025-12-12T00:00:00Z&end_time=2025-12-12T23:59:59Z" | jq
```

**6. Monitor logs** (optional)

```bash
# View streamer logs
kubectl logs -l app=streamer --tail=50 -f

# View collector logs
kubectl logs -l app=collector --tail=50 -f

# View API logs
kubectl logs -l app=api-gateway --tail=50 -f
```

**7. Clean up**

```bash
# When done, clean up local deployment
make local-cleanup
```

---

## Project Structure

```
.
├── cmd/                          # Application entry points
│   ├── api-gateway/              # REST API service
│   ├── collector/                # Telemetry collector service
│   ├── queueservice/             # Queue service (if using custom queue)
│   └── streamer/                 # CSV streamer service
├── internal/                     # Private application code
│   ├── api/                      # API handlers, middleware, DTOs
│   │   ├── handlers/             # HTTP request handlers
│   │   ├── middleware/           # Request logging, CORS, etc.
│   │   └── router.go             # Route definitions
│   ├── collector/                # Collector business logic
│   ├── config/                   # Configuration management
│   ├── domain/                   # Domain models (GPU, TelemetryPoint)
│   ├── mq/                       # Message queue implementations
│   │   ├── http_queue_client.go  # HTTP-based queue client
│   │   ├── inmemory_queue.go     # In-memory queue (for testing)
│   │   └── redis_queue.go        # Redis queue implementation
│   ├── parser/                   # CSV parsing logic
│   ├── queueservice/             # Custom queue service (optional)
│   ├── storage/                  # Storage layer (Repository pattern)
│   │   └── mongodb/              # MongoDB repository
│   ├── streamer/                 # Streamer business logic
│   └── telemetry/                # Telemetry domain logic
├── charts/                       # Helm charts
│   ├── api-gateway/              # API Gateway Helm chart
│   ├── collector/                # Collector Helm chart
│   ├── mongodb/                  # MongoDB Helm chart
│   ├── queue-service/            # Queue Service Helm chart
│   ├── redis/                    # Redis Helm chart
│   └── streamer/                 # Streamer Helm chart
├── data/                         # Sample CSV data
│   └── dcgm_metrics_20250718_134233.csv
├── docs/                         # Documentation
│   └── swagger/                  # Generated Swagger/OpenAPI specs
├── test/                         # Integration tests
├── Makefile                      # Build and deployment automation
├── README.md                     # This file
└── README_AI_PROMPTS.md          # AI assistance documentation
```

---

## Configuration

All services are configured via environment variables. Default values are provided in Helm chart `values.yaml` files.

### Streamer Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `CSV_PATH` | `/data/dcgm_metrics.csv` | Path to CSV file |
| `QUEUE_SERVICE_URL` | `http://queue-service:8080` | Redis/Queue service URL |
| `STREAM_INTERVAL` | `100ms` | Delay between streaming rows |
| `LOOP_MODE` | `true` | Loop CSV file continuously |
| `BATCH_SIZE` | `100` | Number of rows per batch |

### Collector Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `QUEUE_SERVICE_URL` | `http://queue-service:8080` | Redis/Queue service URL |
| `MONGODB_URI` | `mongodb://mongodb:27017` | MongoDB connection string |
| `MONGODB_DATABASE` | `gpu_telemetry` | Database name |
| `BATCH_SIZE` | `1` | Telemetry processing batch size |
| `MAX_CONCURRENT_HANDLERS` | `10` | Worker pool size |

### API Gateway Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `API_PORT` | `8080` | HTTP server port |
| `MONGODB_URI` | `mongodb://mongodb:27017` | MongoDB connection string |
| `MONGODB_DATABASE` | `gpu_telemetry` | Database name |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |

### Queue Service (Redis) Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `redis:6379` | Redis server address |
| `REDIS_PASSWORD` | `` | Redis password (if any) |
| `REDIS_DB` | `0` | Redis database number |

---

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run tests with coverage report
make cover
```

Coverage report is generated at `coverage.html`. Open it in a browser to see detailed coverage.

### System Tests

System tests validate the **complete end-to-end pipeline** from CSV ingestion to API queries. They test the entire system as deployed on Kubernetes.

**Prerequisites:**
- A Kubernetes cluster with GPU telemetry services deployed (kind, minikube, GKE, EKS, etc.)
- `kubectl` configured to access the cluster
- API Gateway accessible at `localhost:8080` (use port-forwarding if needed)
- `kubectl`, `curl`, and `jq` installed

**Run system tests:**

```bash
# For local kind cluster (deployed with make local-deploy):
make system-test

# For remote or non-kind clusters:
# First, ensure API Gateway is accessible locally
kubectl port-forward service/api-gateway 8080:8080 -n default &

# Then run the tests
make system-test
# or
./system-test.sh

# Customize namespace and port if needed:
NAMESPACE=my-namespace API_PORT=9090 ./system-test.sh
```

**What the system tests verify:**

1. ✅ **API Gateway Health** - Ensures the API is responding
2. ✅ **Data Ingestion** - Verifies telemetry data flows from CSV → Streamer → Queue → Collector → MongoDB
3. ✅ **GPU Listing** - Tests `GET /api/v1/gpus` endpoint
4. ✅ **GPU Details** - Tests `GET /api/v1/gpus/{uuid}` endpoint
5. ✅ **Telemetry Retrieval** - Tests `GET /api/v1/gpus/{uuid}/telemetry` endpoint
6. ✅ **Telemetry Ordering** - Confirms results are sorted by timestamp (newest first)
7. ✅ **Time Range Filtering** - Tests `start_time` and `end_time` query parameters
8. ✅ **Data Structure Validation** - Ensures all required fields are present
9. ✅ **Error Handling** - Validates 400/404 responses for invalid requests
10. ✅ **Swagger Documentation** - Checks Swagger UI accessibility
11. ✅ **Component Health** - Reviews logs for errors across all services

**Expected output:**

```
==========================================
           TEST SUMMARY
==========================================
[SUCCESS] All system tests passed! ✓

Test Results:
  ✓ API Gateway Health Check
  ✓ Data Ingestion (found 8 GPU(s))
  ✓ List GPUs endpoint
  ✓ Get GPU details endpoint
  ✓ Get telemetry endpoint
  ✓ Telemetry ordering (newest first)
  ✓ Time range filtering
  ✓ Data structure validation
  ✓ Error handling (400 for invalid UUID)
  ✓ Error handling (404 for non-existent GPU)
  ✓ Swagger documentation
  ✓ Component health check

[SUCCESS] GPU Telemetry Pipeline is functioning correctly!
==========================================
```

**Troubleshooting:**

If system tests fail:
1. Check pod status: `kubectl get pods -n default`
2. View component logs: `kubectl logs -l app=<component> -n default`
3. Verify port forwarding: `lsof -i :8080`
4. Restart deployment if needed: `kubectl rollout restart deployment/<component> -n default`

### Manual API Testing

See [User Workflow](#user-workflow) section for step-by-step API testing examples.

---

## Troubleshooting

### Pods Not Starting

**Check pod status:**
```bash
kubectl get pods
kubectl describe pod <pod-name>
kubectl logs <pod-name>
```

**Common causes:**
- Image pull errors: Images not loaded into kind cluster
  ```bash
  make local-deploy  # Re-run to reload images
  ```
- Resource constraints: Insufficient CPU/memory
  ```bash
  kubectl top nodes
  ```

### API Not Responding

**Check API Gateway logs:**
```bash
kubectl logs -l app=api-gateway --tail=100
```

**Verify port forwarding:**
```bash
# Kill existing port-forward
pkill -f "port-forward.*api-gateway"

# Restart port forwarding
kubectl port-forward service/api-gateway 8080:8080
```

**Test health endpoint:**
```bash
curl http://localhost:8080/health
```

### MongoDB Connection Issues

**Check MongoDB pod:**
```bash
kubectl get pods -l app=mongodb
kubectl logs mongodb-0
```

**Verify MongoDB service:**
```bash
kubectl get svc mongodb
```

**Check MongoDB resource limits:**
If MongoDB pod is OOMKilled, increase memory limits in `charts/mongodb/values.yaml`:
```yaml
resources:
  limits:
    memory: 2Gi  # Increase if needed
```

### No Telemetry Data

**Check streamer logs:**
```bash
kubectl logs -l app=streamer --tail=100
```

**Check collector logs:**
```bash
kubectl logs -l app=collector --tail=100
```

**Verify queue service:**
```bash
kubectl logs -l app=queue-service --tail=100
# or for Redis
kubectl logs -l app=redis --tail=100
```

**Check if CSV data is mounted:**
```bash
kubectl exec -it <streamer-pod> -- ls -la /data/
```

### Images Not Found (ImagePullBackOff)

**For kind clusters:**
```bash
# Rebuild and reload images
make docker-build
kind load docker-image gpu-telemetry-streamer:latest --name gpu-telemetry-cluster
kind load docker-image gpu-telemetry-collector:latest --name gpu-telemetry-cluster
kind load docker-image gpu-telemetry-api:latest --name gpu-telemetry-cluster
kind load docker-image gpu-telemetry-queueservice:latest --name gpu-telemetry-cluster
```

**For remote clusters:**
Ensure images are pushed to a registry accessible by your cluster:
```bash
make k8s-deploy REGISTRY=yourusername TAG=v1.0.0
```

---

## AI Assistance

This project was developed with extensive AI assistance from Claude (Anthropic). For a detailed log of prompts, development workflow, and how AI was used throughout the project, please refer to:

**[docs/README_AI_PROMPTS.md](docs/README_AI_PROMPTS.md)**

The AI assistance covered:
- System architecture design
- Implementation of all microservices
- Kubernetes/Helm chart creation
- Test suite development
- Documentation generation
- Troubleshooting and debugging

---

## License

MIT
