# GPU Telemetry Pipeline

An elastic, scalable telemetry pipeline for GPU monitoring in AI clusters, featuring a custom message queue implementation.

## Architecture

This system implements a distributed telemetry pipeline with the following components:

- **Telemetry Streamer**: Reads GPU metrics from CSV and streams them via the message queue
- **Custom Message Queue**: In-memory queue implementation supporting multiple producers/consumers
- **Telemetry Collector**: Consumes messages from the queue and persists data
- **API Gateway**: REST API for querying GPU telemetry data

## Project Structure

```
.
├── cmd/                    # Application entry points
│   ├── streamer/          # Telemetry Streamer service
│   ├── collector/         # Telemetry Collector service
│   └── api-gateway/       # API Gateway service
├── internal/              # Private application code
│   ├── domain/           # Domain models (GPU, TelemetryPoint)
│   ├── mq/               # Custom message queue implementation
│   ├── storage/          # Storage layer (Repository pattern)
│   ├── telemetry/        # Telemetry business logic
│   ├── api/              # API handlers, middleware, DTOs
│   └── config/           # Configuration management
├── pkg/                   # Public reusable libraries
├── deployments/           # Deployment configurations
├── api/                   # OpenAPI specifications
└── docs/                  # Documentation
```

## Getting Started

### Prerequisites

- Go 1.21 or later
- Make

### Build

```bash
# Build all services
make build

# Build individual services
go build -o bin/streamer ./cmd/streamer
go build -o bin/collector ./cmd/collector
go build -o bin/api-gateway ./cmd/api-gateway
```

### Run Services

```bash
# Run Telemetry Streamer
make run-streamer

# Run Telemetry Collector
make run-collector

# Run API Gateway
make run-api
```

### Testing

```bash
# Run all tests
make test

# Run tests with coverage
make cover
```

### Generate OpenAPI Specification

```bash
make openapi
```

## API Endpoints

- `GET /api/v1/gpus` - List all GPUs with telemetry data
- `GET /api/v1/gpus/{id}/telemetry` - Get telemetry for a specific GPU
  - Query parameters: `start_time`, `end_time` (RFC3339 format)
- `GET /health` - Health check
- `GET /metrics` - Service metrics

## Configuration

Configuration can be provided via environment variables or command-line flags:

- `INSTANCE_ID` - Instance identifier
- `LOG_LEVEL` - Log level (debug, info, warn, error)
- `CSV_FILE` - Path to CSV file (Streamer)
- `STREAM_INTERVAL` - Interval between streaming rows in ms (Streamer)
- `API_PORT` - API Gateway port
- `QUEUE_BUFFER_SIZE` - Message queue buffer size
- `STORAGE_TYPE` - Storage type (inmemory, mongodb)

## Design Patterns

This project implements several design patterns:

- **Repository Pattern**: Abstract storage layer for extensibility
- **Factory Pattern**: Queue and storage creation
- **Strategy Pattern**: Pluggable parsers and serializers
- **Publisher-Subscriber**: Message queue architecture
- **Adapter Pattern**: CSV to domain model conversion

## SOLID Principles

The codebase adheres to SOLID principles:

- **Single Responsibility**: Each component has a focused purpose
- **Open/Closed**: Extensible via interfaces
- **Liskov Substitution**: Implementations are interchangeable
- **Interface Segregation**: Small, focused interfaces
- **Dependency Inversion**: Depend on abstractions

## Docker Deployment

### Build Docker Images

```bash
# Build all images
make docker-build

# Or build individually
make docker-streamer
make docker-collector
make docker-api
make docker-queue
```

### Run with Docker

```bash
# Run individual services
docker run -d --name streamer \
  -v $(pwd)/data/dcgm_metrics_20250718_134233.csv:/app/data/metrics.csv \
  -e QUEUE_URL=http://queue-service:8081 \
  gpu-telemetry-streamer:latest

docker run -d --name collector \
  -e QUEUE_URL=http://queue-service:8081 \
  gpu-telemetry-collector:latest

docker run -d --name api-gateway -p 8080:8080 \
  gpu-telemetry-api:latest

docker run -d --name queue-service -p 8081:8081 \
  gpu-telemetry-queue:latest
```

## Kubernetes Deployment

### Prerequisites

- Docker
- kind (Kubernetes in Docker)
- kubectl
- Helm v3.x

### Quick Start with kind

```bash
# 1. Create kind cluster
kind create cluster --name gpu-telemetry

# 2. Build and load Docker images
make docker-build
kind load docker-image gpu-telemetry-streamer:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-collector:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-api:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-queue:latest --name gpu-telemetry

# 3. Create CSV ConfigMap
kubectl create configmap gpu-telemetry-csv-data \
  --from-file=metrics.csv=data/dcgm_metrics_20250718_134233.csv

# 4. Install Helm chart
helm install gpu-telemetry ./charts/gpu-telemetry

# 5. Wait for pods to be ready
kubectl get pods -w

# 6. Access the API
kubectl port-forward service/gpu-telemetry-api-gateway 8080:8080

# 7. Test the API
curl http://localhost:8080/api/v1/gpus
```

For detailed deployment instructions, see [DEPLOYMENT.md](DEPLOYMENT.md).

### Scaling

```bash
# Scale streamers and collectors
helm upgrade gpu-telemetry ./charts/gpu-telemetry \
  --set streamer.replicaCount=5 \
  --set collector.replicaCount=5

# Or use kubectl
kubectl scale deployment gpu-telemetry-streamer --replicas=5
kubectl scale deployment gpu-telemetry-collector --replicas=5
```

## End-to-End Testing

### Manual Testing

Follow the step-by-step guide in [E2E_TEST.md](E2E_TEST.md) to manually test the entire pipeline.

### Automated System Test

Run the automated Go system test:

```bash
# Option 1: Use the test runner script
./test/e2e/run_test.sh

# Option 2: Run directly (requires port-forward to be running)
kubectl port-forward service/gpu-telemetry-api-gateway 8080:8080 &
cd test/e2e
go run system_test.go
```

The system test verifies:
- ✅ API health and availability
- ✅ Data ingestion from CSV
- ✅ Message flow through the queue
- ✅ Telemetry storage and retrieval
- ✅ Time-range filtering
- ✅ Data ordering (newest first)
- ✅ Data freshness
- ✅ Swagger documentation

## Troubleshooting

### Common Issues

**Pods not starting:**
```bash
kubectl describe pod <pod-name>
kubectl logs <pod-name>
```

**Images not found in kind:**
```bash
kind load docker-image <image-name>:latest --name gpu-telemetry
```

**CSV data missing:**
```bash
kubectl get configmap gpu-telemetry-csv-data
kubectl describe configmap gpu-telemetry-csv-data
```

**API not responding:**
```bash
kubectl logs -l app.kubernetes.io/component=api-gateway
kubectl get svc
kubectl get endpoints
```

For more troubleshooting tips, see [DEPLOYMENT.md](DEPLOYMENT.md).

## Documentation

- [E2E Testing Guide](E2E_TEST.md) - Complete end-to-end testing instructions
- [Deployment Guide](DEPLOYMENT.md) - Detailed Kubernetes deployment guide
- [AI Prompts Log](README_AI_PROMPTS.md) - Development workflow and prompts

## License

MIT

## AI Assistance

This project was developed with AI assistance. See `README_AI_PROMPTS.md` for the detailed prompt log and development workflow.
