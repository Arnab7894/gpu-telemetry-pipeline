# Domain Models & CSV Mapping

This document explains the domain model design and how CSV columns map to Go structs.

## Overview

The system has two primary domain entities:
1. **GPU** - Represents a physical GPU device
2. **TelemetryPoint** - Represents a single telemetry measurement

## CSV Format

The input CSV has the following structure:

```csv
timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
```

### CSV Columns Explained

| Column | Description | Example |
|--------|-------------|---------|
| `timestamp` | Original timestamp from CSV | `"2025-07-18T20:42:34Z"` |
| `metric_name` | Type of metric | `"DCGM_FI_DEV_GPU_UTIL"` |
| `gpu_id` | Logical GPU index on host | `"0"`, `"1"`, `"2"` |
| `device` | Device identifier | `"nvidia0"`, `"nvidia1"` |
| `uuid` | Unique GPU hardware UUID | `"GPU-5fd4f087-86f3-7a43-b711-4771313afc50"` |
| `modelName` | GPU model | `"NVIDIA H100 80GB HBM3"` |
| `Hostname` | Host machine name | `"mtv5-dgx1-hgpu-031"` |
| `container` | Container name (optional) | `""` or `"my-container"` |
| `pod` | Kubernetes pod name (optional) | `""` or `"my-pod"` |
| `namespace` | Kubernetes namespace (optional) | `""` or `"default"` |
| `value` | Metric value | `"0"`, `"100"`, `"98.5"` |
| `labels_raw` | Prometheus-style labels | `'DCGM_FI_DRIVER_VERSION="535.129.03",...'` |

## GPU Entity

### Go Struct

```go
type GPU struct {
    UUID      string `json:"uuid"`           // PRIMARY KEY
    DeviceID  string `json:"device_id"`
    GPUIndex  string `json:"gpu_index"`
    ModelName string `json:"model_name"`
    Hostname  string `json:"hostname"`
    Container string `json:"container,omitempty"`
    Pod       string `json:"pod,omitempty"`
    Namespace string `json:"namespace,omitempty"`
}
```

### CSV Column Mapping

| Go Field | CSV Column | Description |
|----------|------------|-------------|
| `UUID` | `uuid` | **Primary key** for GPU identification |
| `DeviceID` | `device` | Device name (nvidia0, nvidia1, etc.) |
| `GPUIndex` | `gpu_id` | Logical index on the host |
| `ModelName` | `modelName` | GPU model string |
| `Hostname` | `Hostname` | Host machine identifier |
| `Container` | `container` | Container name (empty in current data) |
| `Pod` | `pod` | Pod name (empty in current data) |
| `Namespace` | `namespace` | Namespace (empty in current data) |

### Key Design Decisions

**Primary Key: `UUID`**
- Each GPU has a globally unique hardware UUID
- Used in API endpoint: `/api/v1/gpus/{uuid}/telemetry`
- Example: `GPU-5fd4f087-86f3-7a43-b711-4771313afc50`

**Alternative Composite Key: `Hostname:DeviceID`**
- Also uniquely identifies a GPU
- Useful for human-readable references
- Example: `mtv5-dgx1-hgpu-031:nvidia0`

## TelemetryPoint Entity

### Go Struct

```go
type TelemetryPoint struct {
    GPUUUID    string    `json:"gpu_uuid"`
    MetricName string    `json:"metric_name"`
    Value      string    `json:"value"`
    Timestamp  time.Time `json:"timestamp"`
    LabelsRaw  string    `json:"labels_raw,omitempty"`
}
```

### CSV Column Mapping

| Go Field | CSV Column | Description |
|----------|------------|-------------|
| `GPUUUID` | `uuid` | References the GPU this metric belongs to |
| `MetricName` | `metric_name` | Type of metric being measured |
| `Value` | `value` | Metric value (stored as string) |
| `Timestamp` | **NOT FROM CSV** | `time.Now()` at ingestion time |
| `LabelsRaw` | `labels_raw` | Optional Prometheus-style metadata |

### Key Design Decisions

**Timestamp Handling**
- ⚠️ **CSV `timestamp` column is IGNORED**
- We use `time.Now()` when the Streamer publishes the message
- This tracks when data flows through our system (ingestion time)
- Allows measuring end-to-end latency

**Value as String**
- Stored as string to preserve precision
- Different metrics have different types:
  - Percentages: `"0"`, `"100"`, `"98"`
  - Power (Watts): `"156.596"`, `"300.794"`
  - Memory (bytes): Large integers
  - Temperature (Celsius): Decimals

**Composite Key for Uniqueness**
- `GPUUUID + MetricName + Timestamp` = Unique telemetry record
- Example: `GPU-xxx:DCGM_FI_DEV_GPU_UTIL:2025-12-07T22:30:45.123456Z`

## Metric Types

The system recognizes these DCGM metric types:

### GPU Utilization Metrics
- `DCGM_FI_DEV_GPU_UTIL` - GPU utilization % (0-100)
- `DCGM_FI_DEV_MEM_COPY_UTIL` - Memory copy utilization
- `DCGM_FI_DEV_ENC_UTIL` - Encoder utilization
- `DCGM_FI_DEV_DEC_UTIL` - Decoder utilization

### Memory Metrics
- `DCGM_FI_DEV_FB_USED` - Framebuffer memory used (bytes)
- `DCGM_FI_DEV_FB_FREE` - Framebuffer memory free (bytes)

### Clock Metrics
- `DCGM_FI_DEV_MEM_CLOCK` - Memory clock speed (MHz)
- `DCGM_FI_DEV_SM_CLOCK` - SM clock speed (MHz)

### Power & Temperature Metrics
- `DCGM_FI_DEV_POWER_USAGE` - Power consumption (Watts)
- `DCGM_FI_DEV_GPU_TEMP` - GPU temperature (Celsius)

## API Query Patterns

### List All GPUs

```
GET /api/v1/gpus
```

Returns all GPUs for which we have telemetry data.

**Query Logic:**
1. Retrieve all unique GPU UUIDs from storage
2. For each UUID, fetch the GPU entity
3. Return array of GPU objects

### Get GPU Telemetry

```
GET /api/v1/gpus/{uuid}/telemetry
GET /api/v1/gpus/{uuid}/telemetry?start_time=2025-12-07T00:00:00Z&end_time=2025-12-07T23:59:59Z
```

Returns telemetry for a specific GPU, optionally filtered by time range.

**Query Logic:**
1. Validate GPU UUID exists
2. Query telemetry points WHERE `GPUUUID = {uuid}`
3. If `start_time` provided: filter WHERE `Timestamp >= start_time`
4. If `end_time` provided: filter WHERE `Timestamp <= end_time`
5. Order by `Timestamp` ASC
6. Return telemetry array

**Example Response:**
```json
{
  "gpu_uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
  "telemetry": [
    {
      "metric_name": "DCGM_FI_DEV_GPU_UTIL",
      "value": "98",
      "timestamp": "2025-12-07T22:30:45.123456Z"
    },
    {
      "metric_name": "DCGM_FI_DEV_POWER_USAGE",
      "value": "300.794",
      "timestamp": "2025-12-07T22:30:45.234567Z"
    }
  ],
  "total": 2
}
```

## Helper Structs

### CSVRow

A helper struct for parsing CSV rows (not part of core domain):

```go
type CSVRow struct {
    Timestamp   string // CSV column 0 - IGNORED
    MetricName  string // CSV column 1
    GPUIndex    string // CSV column 2
    Device      string // CSV column 3
    UUID        string // CSV column 4
    ModelName   string // CSV column 5
    Hostname    string // CSV column 6
    Container   string // CSV column 7
    Pod         string // CSV column 8
    Namespace   string // CSV column 9
    Value       string // CSV column 10
    LabelsRaw   string // CSV column 11
}
```

This struct is used by the CSV parser to read raw data before converting to domain entities.

## Data Flow

```
CSV Row
  ↓ [Parser reads]
CSVRow struct
  ↓ [Parser converts]
GPU + TelemetryPoint
  ↓ [Streamer adds Timestamp = time.Now()]
Message (with GPU + TelemetryPoint)
  ↓ [Queue]
Collector
  ↓ [Storage]
Repository (In-Memory / MongoDB)
  ↓ [API]
JSON Response
```

## Storage Considerations

### In-Memory Storage
- GPUs stored in map: `map[uuid]*GPU`
- Telemetry stored in map: `map[gpuUUID][]*TelemetryPoint`
- Thread-safe with `sync.RWMutex`
- Sorted by timestamp when queried

### Future MongoDB Storage
- GPUs collection: Indexed on `uuid`
- Telemetry collection: Compound index on `(gpu_uuid, timestamp)`
- Time-series optimizations for telemetry queries
- TTL index for data retention policies

## Design Principles Applied

1. **Single Responsibility**: Each struct has one clear purpose
2. **Immutability**: Structs are value objects (no setters)
3. **Explicit Keys**: UUID is clearly the primary key
4. **Type Safety**: Metric types defined as constants
5. **Extensibility**: Easy to add new metric types
6. **API-First**: Struct design aligns with API requirements
7. **Performance**: String values avoid conversion overhead
8. **Observability**: Ingestion timestamp tracks system latency
