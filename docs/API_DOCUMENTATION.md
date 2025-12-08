# API Documentation

## Overview

The GPU Telemetry Pipeline API provides REST endpoints to query GPU telemetry data collected from DCGM metrics.

## Base URL

```
http://localhost:8080/api/v1
```

## Generating OpenAPI Specification

### Prerequisites

Install `swag` tool:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
```

### Generate Swagger Documentation

```bash
make swagger
# or
make openapi
```

This will generate:
- `docs/swagger/swagger.json` - OpenAPI 2.0 specification in JSON format
- `docs/swagger/swagger.yaml` - OpenAPI 2.0 specification in YAML format  
- `docs/swagger/docs.go` - Go documentation file

## Viewing API Documentation

### Option 1: Swagger UI (Built-in)

1. Start the API Gateway:
   ```bash
   make run-api
   ```

2. Open your browser to:
   ```
   http://localhost:8080/swagger/index.html
   ```

### Option 2: Swagger Editor Online

1. Generate the spec:
   ```bash
   make swagger
   ```

2. Copy the contents of `docs/swagger/swagger.yaml`

3. Go to [Swagger Editor](https://editor.swagger.io/)

4. Paste the YAML content

### Option 3: VS Code Extension

1. Install the "OpenAPI (Swagger) Editor" extension

2. Open `docs/swagger/swagger.yaml` in VS Code

3. Right-click and select "Preview Swagger"

## API Endpoints

### 1. List All GPUs

**Endpoint**: `GET /api/v1/gpus`

**Description**: Returns all GPUs for which telemetry data exists

**Response**: 200 OK
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

**Example**:
```bash
curl http://localhost:8080/api/v1/gpus
```

---

### 2. Get GPU Telemetry

**Endpoint**: `GET /api/v1/gpus/{uuid}/telemetry`

**Description**: Returns telemetry data for a specific GPU, ordered by timestamp (newest first)

**Path Parameters**:
- `uuid` (string, required): GPU UUID

**Query Parameters**:
- `start_time` (string, optional): Start time in RFC3339 format (e.g., `2025-01-18T00:00:00Z`)
- `end_time` (string, optional): End time in RFC3339 format (e.g., `2025-01-18T23:59:59Z`)

**Response**: 200 OK
```json
{
  "metrics": [
    {
      "gpu_uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
      "metric_name": "DCGM_FI_DEV_GPU_UTIL",
      "value": "85",
      "timestamp": "2025-01-18T12:34:56.789Z",
      "labels_raw": "DCGM_FI_DRIVER_VERSION=\"535.129.03\",Hostname=\"mtv5-dgx1-hgpu-031\""
    }
  ],
  "total": 1,
  "gpu_uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
  "start_time": "2025-01-18T00:00:00Z",
  "end_time": "2025-01-18T23:59:59Z"
}
```

**Examples**:

Get all telemetry for a GPU:
```bash
curl http://localhost:8080/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry
```

Get telemetry with time filtering:
```bash
curl "http://localhost:8080/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry?start_time=2025-01-18T00:00:00Z&end_time=2025-01-18T12:00:00Z"
```

---

## Error Responses

All errors return a standardized error response:

**Format**:
```json
{
  "error": "Error type",
  "message": "Detailed error message",
  "timestamp": "2025-01-18T12:34:56Z"
}
```

**Status Codes**:
- `400 Bad Request` - Invalid input parameters (malformed UUID, invalid time format)
- `404 Not Found` - GPU not found
- `500 Internal Server Error` - Server-side error

**Examples**:

GPU Not Found (404):
```json
{
  "error": "GPU not found",
  "message": "No GPU found with UUID: GPU-invalid-uuid",
  "timestamp": "2025-01-18T12:34:56Z"
}
```

Invalid Time Format (400):
```json
{
  "error": "Invalid start_time format",
  "message": "Use RFC3339 format (e.g., 2023-01-01T00:00:00Z). Got: invalid-time",
  "timestamp": "2025-01-18T12:34:56Z"
}
```

---

## Testing the API

### Manual Testing

1. Start the queue service:
   ```bash
   go run cmd/queueservice/main.go
   ```

2. Start a collector:
   ```bash
   export QUEUE_SERVICE_URL=http://localhost:8080
   go run cmd/collector/main.go -instance-id=collector-1
   ```

3. Start a streamer:
   ```bash
   export QUEUE_SERVICE_URL=http://localhost:8080
   go run cmd/streamer/main.go -csv-path=test_sample.csv -loop=false
   ```

4. Start the API Gateway:
   ```bash
   go run cmd/api-gateway/main.go
   ```

5. Query the API:
   ```bash
   # List GPUs
   curl http://localhost:8080/api/v1/gpus | jq .
   
   # Get telemetry
   curl "http://localhost:8080/api/v1/gpus/GPU-<uuid>/telemetry" | jq .
   ```

### Integration Testing

Run the integration test script:
```bash
./test-integration.sh
```

---

## Architecture

```
┌─────────────┐
│   Client    │
│  (Browser/  │
│   curl)     │
└──────┬──────┘
       │ HTTP
       ▼
┌─────────────────────────────────┐
│      API Gateway                │
│  (cmd/api-gateway/main.go)      │
│                                 │
│  ┌──────────────────────────┐  │
│  │  Gin HTTP Router         │  │
│  │  - Middleware            │  │
│  │  - Route handlers        │  │
│  └───────────┬──────────────┘  │
│              │                  │
│  ┌───────────▼──────────────┐  │
│  │  Handlers                │  │
│  │  - GPUHandler            │  │
│  │  - TelemetryHandler      │  │
│  └───────────┬──────────────┘  │
│              │                  │
│  ┌───────────▼──────────────┐  │
│  │  DTOs (Data Transfer     │  │
│  │  Objects)                │  │
│  │  - Domain → DTO          │  │
│  └──────────────────────────┘  │
└────────────┬────────────────────┘
             │
    ┌────────▼────────┐
    │  Storage        │
    │  Repositories   │
    │  (In-Memory)    │
    └─────────────────┘
```

---

## Development

### Adding New Endpoints

1. Define DTOs in `internal/api/dto/`
2. Add handler methods in `internal/api/handlers/`
3. Add Swagger annotations (godoc comments)
4. Register routes in `internal/api/router.go`
5. Regenerate Swagger docs: `make swagger`

### Swagger Annotation Format

```go
// HandlerName godoc
// @Summary Short description
// @Description Detailed description
// @Tags tag-name
// @Accept json
// @Produce json
// @Param param_name path/query type required "description"
// @Success 200 {object} dto.ResponseType
// @Failure 400 {object} dto.ErrorResponse
// @Router /api/v1/path [method]
func (h *Handler) HandlerName(c *gin.Context) {
    // Implementation
}
```

---

## References

- [Swagger/OpenAPI 2.0 Specification](https://swagger.io/specification/v2/)
- [swaggo/swag Documentation](https://github.com/swaggo/swag)
- [Gin Web Framework](https://github.com/gin-gonic/gin)
- [RFC3339 Time Format](https://www.rfc-editor.org/rfc/rfc3339)
