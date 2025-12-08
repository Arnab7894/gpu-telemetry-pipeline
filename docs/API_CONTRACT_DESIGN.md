# API Contract and Handler Design (Prompt 9)

## Framework Choice: Gin

**Selected Framework**: `gin-gonic/gin`

**Justification**:
- **Performance**: Gin is one of the fastest HTTP frameworks for Go (uses httprouter)
- **Ease of Use**: Clean, intuitive API with excellent documentation
- **Middleware Support**: Built-in middleware for logging, recovery, CORS, etc.
- **JSON Handling**: Native support for JSON binding and validation
- **OpenAPI Integration**: Works seamlessly with `swaggo/swag` for Swagger generation
- **Production Ready**: Used by many production systems, well-maintained
- **Path Parameters**: Clean syntax for route parameters (`:uuid`)

## API Contract

### Base URL
```
http://localhost:8080/api/v1
```

## Endpoints

### 1. GET /api/v1/gpus

**Description**: List all GPUs for which telemetry data exists

**Path Parameters**: None

**Query Parameters**: None

**Request Example**:
```http
GET /api/v1/gpus HTTP/1.1
Host: localhost:8080
Accept: application/json
```

**Success Response** (200 OK):
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
    },
    {
      "uuid": "GPU-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "device_id": "nvidia1",
      "gpu_index": "1",
      "model_name": "NVIDIA H100 80GB HBM3",
      "hostname": "mtv5-dgx1-hgpu-031",
      "container": "gpu-workload",
      "pod": "training-pod-1",
      "namespace": "ml-team"
    }
  ],
  "total": 2
}
```

**Error Response** (500 Internal Server Error):
```json
{
  "error": "Failed to retrieve GPUs",
  "message": "Internal server error occurred while fetching GPU list",
  "timestamp": "2025-01-18T12:34:56Z"
}
```

---

### 2. GET /api/v1/gpus/{uuid}/telemetry

**Description**: Get telemetry data for a specific GPU, ordered by timestamp (newest first)

**Path Parameters**:
- `uuid` (string, required): GPU UUID (e.g., "GPU-5fd4f087-86f3-7a43-b711-4771313afc50")

**Query Parameters**:
- `start_time` (string, optional): Start time in RFC3339 format (e.g., "2025-01-18T00:00:00Z")
- `end_time` (string, optional): End time in RFC3339 format (e.g., "2025-01-18T23:59:59Z")

**Request Examples**:

1. All telemetry for GPU:
```http
GET /api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry HTTP/1.1
Host: localhost:8080
Accept: application/json
```

2. Filtered by time range:
```http
GET /api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry?start_time=2025-01-18T00:00:00Z&end_time=2025-01-18T12:00:00Z HTTP/1.1
Host: localhost:8080
Accept: application/json
```

**Success Response** (200 OK):
```json
{
  "metrics": [
    {
      "gpu_uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
      "metric_name": "DCGM_FI_DEV_GPU_UTIL",
      "value": "85",
      "timestamp": "2025-01-18T12:34:56.789Z",
      "labels_raw": "DCGM_FI_DRIVER_VERSION=\"535.129.03\",Hostname=\"mtv5-dgx1-hgpu-031\""
    },
    {
      "gpu_uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
      "metric_name": "DCGM_FI_DEV_POWER_USAGE",
      "value": "300.5",
      "timestamp": "2025-01-18T12:34:56.789Z",
      "labels_raw": "DCGM_FI_DRIVER_VERSION=\"535.129.03\",Hostname=\"mtv5-dgx1-hgpu-031\""
    }
  ],
  "total": 2,
  "gpu_uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
  "start_time": "2025-01-18T00:00:00Z",
  "end_time": "2025-01-18T12:00:00Z"
}
```

**Error Responses**:

404 Not Found (GPU doesn't exist):
```json
{
  "error": "GPU not found",
  "message": "No GPU found with UUID: GPU-invalid-uuid",
  "timestamp": "2025-01-18T12:34:56Z"
}
```

400 Bad Request (Invalid time format):
```json
{
  "error": "Invalid start_time format",
  "message": "Use RFC3339 format (e.g., 2023-01-01T00:00:00Z)",
  "timestamp": "2025-01-18T12:34:56Z"
}
```

500 Internal Server Error:
```json
{
  "error": "Failed to retrieve telemetry data",
  "message": "Internal server error occurred",
  "timestamp": "2025-01-18T12:34:56Z"
}
```

---

## Data Transfer Objects (DTOs)

### GPU Response Model

**File**: `internal/api/dto/gpu_dto.go`

```go
// GPUResponse represents a GPU in API responses
// Decouples internal domain.GPU from API contract
type GPUResponse struct {
    UUID      string `json:"uuid"`
    DeviceID  string `json:"device_id"`
    GPUIndex  string `json:"gpu_index"`
    ModelName string `json:"model_name"`
    Hostname  string `json:"hostname"`
    Container string `json:"container,omitempty"`
    Pod       string `json:"pod,omitempty"`
    Namespace string `json:"namespace,omitempty"`
}

// GPUListResponse wraps a list of GPUs
type GPUListResponse struct {
    GPUs  []*GPUResponse `json:"gpus"`
    Total int            `json:"total"`
}
```

### Telemetry Response Model

**File**: `internal/api/dto/telemetry_dto.go`

```go
// TelemetryResponse represents a single telemetry metric in API responses
// Decouples internal domain.TelemetryPoint from API contract
type TelemetryResponse struct {
    GPUUUID    string    `json:"gpu_uuid"`
    MetricName string    `json:"metric_name"`
    Value      string    `json:"value"`
    Timestamp  time.Time `json:"timestamp"`
    LabelsRaw  string    `json:"labels_raw,omitempty"`
}

// TelemetryListResponse wraps a list of telemetry metrics
type TelemetryListResponse struct {
    Metrics   []*TelemetryResponse `json:"metrics"`
    Total     int                  `json:"total"`
    GPUUUID   string               `json:"gpu_uuid"`
    StartTime *time.Time           `json:"start_time,omitempty"`
    EndTime   *time.Time           `json:"end_time,omitempty"`
}
```

### Error Response Model

**File**: `internal/api/dto/error_dto.go`

```go
// ErrorResponse represents a standardized error response
type ErrorResponse struct {
    Error     string    `json:"error"`
    Message   string    `json:"message"`
    Timestamp time.Time `json:"timestamp"`
}
```

---

## Handler Interfaces

### GPU Handler

**File**: `internal/api/handlers/gpu_handler.go`

```go
type GPUHandler struct {
    gpuRepo storage.GPURepository
}

// ListGPUs handles GET /api/v1/gpus
// Returns all GPUs with telemetry data as DTOs
func (h *GPUHandler) ListGPUs(c *gin.Context) {
    // 1. Call repository to get domain.GPU objects
    // 2. Convert domain.GPU → dto.GPUResponse
    // 3. Wrap in dto.GPUListResponse
    // 4. Return JSON (200) or error (500)
}

// GetGPU handles GET /api/v1/gpus/:uuid
// Returns a specific GPU as DTO
func (h *GPUHandler) GetGPU(c *gin.Context) {
    // 1. Extract :uuid path parameter
    // 2. Call repository to get domain.GPU
    // 3. Convert domain.GPU → dto.GPUResponse
    // 4. Return JSON (200) or error (404, 500)
}
```

### Telemetry Handler

**File**: `internal/api/handlers/telemetry_handler.go`

```go
type TelemetryHandler struct {
    telemetryRepo storage.TelemetryRepository
    gpuRepo       storage.GPURepository
}

// GetGPUTelemetry handles GET /api/v1/gpus/:uuid/telemetry
// Returns telemetry data for a GPU with optional time filtering
func (h *TelemetryHandler) GetGPUTelemetry(c *gin.Context) {
    // 1. Extract :uuid path parameter
    // 2. Verify GPU exists (gpuRepo.GetByUUID)
    // 3. Parse optional query parameters (start_time, end_time)
    // 4. Validate time format (RFC3339)
    // 5. Call repository with filters
    // 6. Convert []domain.TelemetryPoint → []dto.TelemetryResponse
    // 7. Wrap in dto.TelemetryListResponse
    // 8. Return JSON (200) or error (400, 404, 500)
}
```

---

## Routing Configuration

**File**: `internal/api/router.go`

```go
func (r *Router) setupRoutes() {
    // Health check
    r.engine.GET("/health", healthCheckHandler)
    
    // API v1 group
    v1 := r.engine.Group("/api/v1")
    {
        // GPU endpoints
        gpus := v1.Group("/gpus")
        {
            gpus.GET("", r.gpuHandler.ListGPUs)              // List all GPUs
            gpus.GET("/:uuid", r.gpuHandler.GetGPU)          // Get specific GPU
            gpus.GET("/:uuid/telemetry", r.telemetryHandler.GetGPUTelemetry)  // Get GPU telemetry
        }
    }
}
```

---

## Middleware

### 1. Logging Middleware

**File**: `internal/api/middleware/logging.go`

```go
func LoggingMiddleware() gin.HandlerFunc {
    // Log: method, path, status, duration, client IP
}
```

### 2. Error Handler Middleware

**File**: `internal/api/middleware/error_handler.go`

```go
func ErrorHandlerMiddleware() gin.HandlerFunc {
    // Catch errors from handlers
    // Convert to standardized dto.ErrorResponse
    // Add timestamp
}
```

---

## Key Design Decisions

### 1. UUID as GPU Identifier ✅
- Path parameter: `/:uuid` (e.g., `/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50`)
- Matches the unique key defined in `domain.GPU`
- Stable across restarts (hardware-based)

### 2. DTOs for API Isolation ✅
- **Principle**: "Do not expose internal domain structs directly" (Prompt 9)
- **Benefit**: API contract is decoupled from internal domain models
- **Future-proof**: Can change domain models without breaking API
- **Conversion**: Handlers convert `domain.*` → `dto.*Response`

### 3. Time Filtering ✅
- Use RFC3339 format for `start_time` and `end_time`
- Industry standard, human-readable
- Native Go parsing: `time.Parse(time.RFC3339, str)`

### 4. Error Response Standardization ✅
- Consistent structure across all endpoints
- Includes: error code, user-friendly message, timestamp
- Makes client error handling predictable

### 5. Telemetry Ordering ✅
- Repository sorts by `Timestamp DESC` (newest first)
- Clients see most recent data first
- Supports real-time monitoring use cases

---

## Handler Responsibilities

### ✅ What Handlers DO:
1. **Extract parameters** (path, query)
2. **Validate input** (time format, required fields)
3. **Call repositories** (using storage interfaces)
4. **Convert domain → DTO** (maintain API isolation)
5. **Return JSON responses** (success or error)
6. **Log operations** (via middleware)

### ❌ What Handlers DON'T DO:
1. ❌ Business logic (that's in domain/services)
2. ❌ Direct database access (use repositories)
3. ❌ Expose domain structs (use DTOs)
4. ❌ Complex transformations (keep handlers thin)

---

## Repository Interface Usage

Handlers depend on **interfaces**, not concrete implementations:

```go
// Storage interfaces (defined in internal/storage/)
type GPURepository interface {
    List() ([]*domain.GPU, error)
    GetByUUID(uuid string) (*domain.GPU, error)
    Save(gpu *domain.GPU) error
}

type TelemetryRepository interface {
    GetByGPU(gpuUUID string, filter TimeFilter) ([]*domain.TelemetryPoint, error)
    Save(point *domain.TelemetryPoint) error
}
```

**Benefits**:
- Testable (mock repositories in tests)
- Flexible (swap in-memory → MongoDB later)
- SOLID: Dependency Inversion Principle

---

## Example Request/Response Flows

### Flow 1: List GPUs

```
Client Request:
  GET /api/v1/gpus

↓
Router → GPUHandler.ListGPUs()
↓
1. gpuRepo.List() → []domain.GPU
2. Convert each domain.GPU → dto.GPUResponse
3. Wrap in dto.GPUListResponse{GPUs: [...], Total: N}
4. c.JSON(200, response)
↓
Client Response:
  200 OK
  {
    "gpus": [...],
    "total": N
  }
```

### Flow 2: Get Telemetry with Time Filter

```
Client Request:
  GET /api/v1/gpus/{uuid}/telemetry?start_time=2025-01-18T00:00:00Z

↓
Router → TelemetryHandler.GetGPUTelemetry()
↓
1. Extract uuid from path: c.Param("uuid")
2. Verify GPU exists: gpuRepo.GetByUUID(uuid)
   - If not found → 404 error
3. Parse start_time query parameter
   - If invalid format → 400 error
4. telemetryRepo.GetByGPU(uuid, filter) → []domain.TelemetryPoint
5. Convert each domain.TelemetryPoint → dto.TelemetryResponse
6. Wrap in dto.TelemetryListResponse{Metrics: [...], Total: N, StartTime: ...}
7. c.JSON(200, response)
↓
Client Response:
  200 OK
  {
    "metrics": [...],
    "total": N,
    "gpu_uuid": "GPU-...",
    "start_time": "2025-01-18T00:00:00Z"
  }
```

---

## OpenAPI/Swagger Annotations

Handlers will include Swagger comments for automatic API documentation generation:

```go
// ListGPUs godoc
// @Summary List all GPUs
// @Description Get list of all GPU devices with telemetry data
// @Tags gpus
// @Accept json
// @Produce json
// @Success 200 {object} dto.GPUListResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/gpus [get]
func (h *GPUHandler) ListGPUs(c *gin.Context) {
    // Implementation
}
```

Generate with: `make swagger` (Prompt 10)

---

## Testing Strategy

### Unit Tests (Handler Level):
```go
func TestListGPUs_Success(t *testing.T) {
    // 1. Create mock repository
    mockRepo := &MockGPURepository{
        ListFunc: func() ([]*domain.GPU, error) {
            return []*domain.GPU{{UUID: "GPU-123"}}, nil
        },
    }
    
    // 2. Create handler with mock
    handler := handlers.NewGPUHandler(mockRepo)
    
    // 3. Create test request
    req := httptest.NewRequest("GET", "/api/v1/gpus", nil)
    w := httptest.NewRecorder()
    
    // 4. Call handler
    handler.ListGPUs(ginContext)
    
    // 5. Assert response
    assert.Equal(t, 200, w.Code)
    // Assert JSON structure matches dto.GPUListResponse
}
```

---

## Summary

✅ **Framework**: Gin (fast, production-ready, OpenAPI support)  
✅ **Routes**: `/api/v1/gpus` and `/api/v1/gpus/:uuid/telemetry`  
✅ **Path Parameters**: `:uuid` (GPU unique identifier)  
✅ **Query Parameters**: `start_time`, `end_time` (RFC3339 format)  
✅ **DTOs**: Separate response models (don't expose domain structs)  
✅ **Error Format**: Standardized `ErrorResponse` structure  
✅ **Middleware**: Logging, error handling, recovery  
✅ **Repository Pattern**: Handlers use interfaces (testable, swappable)  
✅ **SOLID Compliance**: Dependency inversion, single responsibility  

**Next Step**: Prompt 10 - Full implementation with OpenAPI generation
