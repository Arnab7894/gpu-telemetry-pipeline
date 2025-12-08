# CSV to Domain Model Mapping Examples

This document provides concrete examples of how CSV rows are parsed and mapped to our domain model structs.

## CSV Structure

The input CSV has 12 columns:
```
timestamp, metric_name, gpu_id, device, uuid, modelName, Hostname, container, pod, namespace, value, labels_raw
```

## Sample CSV Row

```csv
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-5fd4f087-86f3-7a43-b711-4771313afc50,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,k8s_dcgm-exporter,dcgm-exporter-abcd,gpu-metrics,42.5,"DCGM_FI_DRIVER_VERSION=""535.129.03"",Hostname=""mtv5-dgx1-hgpu-031"""
```

## Parsing into CSVRow Helper Struct

The parser first reads the CSV row into the `CSVRow` helper struct:

```go
row := domain.CSVRow{
    Timestamp:  "2024-12-18T10:32:45Z",          // Column 0 - IGNORED
    MetricName: "DCGM_FI_DEV_GPU_UTIL",          // Column 1
    GPUIndex:   "0",                              // Column 2
    Device:     "nvidia0",                        // Column 3
    UUID:       "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",  // Column 4
    ModelName:  "NVIDIA A100-SXM4-80GB",         // Column 5
    Hostname:   "mtv5-dgx1-hgpu-031",            // Column 6
    Container:  "k8s_dcgm-exporter",             // Column 7
    Pod:        "dcgm-exporter-abcd",            // Column 8
    Namespace:  "gpu-metrics",                   // Column 9
    Value:      "42.5",                          // Column 10
    LabelsRaw:  "DCGM_FI_DRIVER_VERSION=\"535.129.03\",Hostname=\"mtv5-dgx1-hgpu-031\"",  // Column 11
}
```

**Important**: The CSV `timestamp` column is parsed but **NOT stored** in the domain model. We use `time.Now()` at ingestion time instead.

## Mapping to GPU Entity

One GPU entity is created per unique `uuid`:

```go
gpu := &domain.GPU{
    UUID:      "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",  // PRIMARY KEY from CSV column 4
    DeviceID:  "nvidia0",                                     // CSV column 3
    GPUIndex:  "0",                                          // CSV column 2
    ModelName: "NVIDIA A100-SXM4-80GB",                      // CSV column 5
    Hostname:  "mtv5-dgx1-hgpu-031",                         // CSV column 6
    Container: "k8s_dcgm-exporter",                          // CSV column 7 (optional)
    Pod:       "dcgm-exporter-abcd",                         // CSV column 8 (optional)
    Namespace: "gpu-metrics",                                // CSV column 9 (optional)
}
```

**Key Design Decision**: UUID is the PRIMARY KEY because:
- Globally unique across all GPUs in the cluster
- Stable across pod restarts (device ID might change)
- Used in API endpoints: `/api/v1/gpus/{uuid}`

**Alternative Composite Key**:
```go
compositeKey := gpu.GetCompositeKey()  
// Returns: "mtv5-dgx1-hgpu-031:nvidia0"
// Format: Hostname:DeviceID
```

## Mapping to TelemetryPoint Entity

One telemetry point is created for each CSV row:

```go
ingestionTime := time.Now()  // Single timestamp for entire batch

point := &domain.TelemetryPoint{
    GPUUUID:    "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",  // CSV column 4 (foreign key)
    MetricName: "DCGM_FI_DEV_GPU_UTIL",                       // CSV column 1
    Value:      "42.5",                                       // CSV column 10 (as string)
    Timestamp:  ingestionTime,                                // time.Now() NOT from CSV!
    LabelsRaw:  "DCGM_FI_DRIVER_VERSION=\"535.129.03\",Hostname=\"mtv5-dgx1-hgpu-031\"",  // CSV column 11
}
```

**Critical Timestamp Strategy**:
- **CSV timestamp is IGNORED** - We don't use column 0
- **Use `time.Now()` at ingestion** - When the Streamer reads the CSV and publishes to the queue
- **Rationale**: We want to track real-time data flow through the system, not historical data

**Value as String**:
- Stored as string to preserve precision
- Different metrics have different precisions (integer vs float)
- Allows flexible metric type handling

**Unique Key for TelemetryPoint**:
```go
uniqueKey := point.GetUniqueKey()
// Returns: "GPU-5fd4f087-86f3-7a43-b711-4771313afc50:DCGM_FI_DEV_GPU_UTIL:2024-12-18T10:35:12.123456789Z"
// Format: GPUUUID:MetricName:Timestamp
```

## Multiple Rows → Multiple Telemetry Points

Given this CSV:
```csv
timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-5fd4f087-86f3-7a43-b711-4771313afc50,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,k8s_dcgm-exporter,dcgm-exporter-abcd,gpu-metrics,42.5,""
2024-12-18T10:32:45Z,DCGM_FI_DEV_POWER_USAGE,0,nvidia0,GPU-5fd4f087-86f3-7a43-b711-4771313afc50,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,k8s_dcgm-exporter,dcgm-exporter-abcd,gpu-metrics,300.794,""
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-5fd4f087-86f3-7a43-b711-4771313afc50,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,k8s_dcgm-exporter,dcgm-exporter-abcd,gpu-metrics,65,""
```

Results in:
- **1 GPU entity** (deduplicated by UUID)
- **3 TelemetryPoint entities** (one per metric)

```go
// Single GPU
gpu := &domain.GPU{
    UUID: "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
    // ... other fields from row 1
}

// Three telemetry points
points := []*domain.TelemetryPoint{
    {GPUUUID: "GPU-...", MetricName: "DCGM_FI_DEV_GPU_UTIL", Value: "42.5", Timestamp: time.Now()},
    {GPUUUID: "GPU-...", MetricName: "DCGM_FI_DEV_POWER_USAGE", Value: "300.794", Timestamp: time.Now()},
    {GPUUUID: "GPU-...", MetricName: "DCGM_FI_DEV_GPU_TEMP", Value: "65", Timestamp: time.Now()},
}
```

## Multiple GPUs in CSV

Given this CSV with 2 different GPUs:
```csv
timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-5fd4f087-86f3-7a43-b711-4771313afc50,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,,,gpu-metrics,42.5,""
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_UTIL,1,nvidia1,GPU-8abc1234-56f7-8def-9012-3456789abcde,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,,,gpu-metrics,55.2,""
```

Results in:
- **2 GPU entities** (one per unique UUID)
- **2 TelemetryPoint entities**

```go
gpus := map[string]*domain.GPU{
    "GPU-5fd4f087-86f3-7a43-b711-4771313afc50": &domain.GPU{
        UUID:     "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
        DeviceID: "nvidia0",
        GPUIndex: "0",
        Hostname: "mtv5-dgx1-hgpu-031",
        // Container and Pod are empty strings
    },
    "GPU-8abc1234-56f7-8def-9012-3456789abcde": &domain.GPU{
        UUID:     "GPU-8abc1234-56f7-8def-9012-3456789abcde",
        DeviceID: "nvidia1",
        GPUIndex: "1",
        Hostname: "mtv5-dgx1-hgpu-031",
    },
}
```

## API Query Patterns

### List all GPUs
```http
GET /api/v1/gpus

Response:
[
    {
        "uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
        "device_id": "nvidia0",
        "gpu_index": "0",
        "model_name": "NVIDIA A100-SXM4-80GB",
        "hostname": "mtv5-dgx1-hgpu-031",
        "container": "k8s_dcgm-exporter",
        "pod": "dcgm-exporter-abcd",
        "namespace": "gpu-metrics"
    }
]
```

### Get specific GPU
```http
GET /api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50

Response:
{
    "uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
    "device_id": "nvidia0",
    ...
}
```

### Get telemetry for a GPU
```http
GET /api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry

Response:
[
    {
        "gpu_uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
        "metric_name": "DCGM_FI_DEV_GPU_UTIL",
        "value": "42.5",
        "timestamp": "2024-12-18T10:35:12.123456789Z"
    },
    {
        "gpu_uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
        "metric_name": "DCGM_FI_DEV_POWER_USAGE",
        "value": "300.794",
        "timestamp": "2024-12-18T10:35:12.123456789Z"
    }
]
```

### Get telemetry with time filtering
```http
GET /api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry?start_time=2024-12-18T10:00:00Z&end_time=2024-12-18T11:00:00Z

Returns only telemetry points where:
- timestamp >= start_time
- timestamp < end_time
```

## Metric Type Validation

The parser validates metric names against known DCGM metrics:

```go
validMetrics := []domain.MetricType{
    domain.MetricGPUUtil,           // "DCGM_FI_DEV_GPU_UTIL"
    domain.MetricMemCopyUtil,       // "DCGM_FI_DEV_MEM_COPY_UTIL"
    domain.MetricEncUtil,           // "DCGM_FI_DEV_ENC_UTIL"
    domain.MetricDecUtil,           // "DCGM_FI_DEV_DEC_UTIL"
    domain.MetricFBUsed,            // "DCGM_FI_DEV_FB_USED"
    domain.MetricFBFree,            // "DCGM_FI_DEV_FB_FREE"
    domain.MetricMemClock,          // "DCGM_FI_DEV_MEM_CLOCK"
    domain.MetricSMClock,           // "DCGM_FI_DEV_SM_CLOCK"
    domain.MetricPowerUsage,        // "DCGM_FI_DEV_POWER_USAGE"
    domain.MetricGPUTemp,           // "DCGM_FI_DEV_GPU_TEMP"
}
```

## Data Flow Summary

```
CSV File (12 columns)
    ↓
[Parser] CSVRow helper struct
    ↓
    ├─→ GPU entity (deduplicated by UUID)
    │   └─→ Stored in GPURepository (in-memory map)
    │
    └─→ TelemetryPoint entity (one per CSV row)
        └─→ Stored in TelemetryRepository (grouped by GPU UUID)
            └─→ Queryable via API with time filtering
```

## Key Takeaways

1. **UUID is PRIMARY KEY** - Used in all API endpoints
2. **CSV timestamp is IGNORED** - We use `time.Now()` at ingestion
3. **Value is stored as string** - Preserves precision for all metric types
4. **One GPU per unique UUID** - Deduplicated during parsing
5. **One TelemetryPoint per CSV row** - Many-to-one relationship with GPU
6. **Time filtering supported** - Query telemetry by time range
7. **Repository pattern** - Clean separation between domain and storage
