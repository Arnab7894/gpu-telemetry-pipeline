package telemetry

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCSV_SingleRow(t *testing.T) {
	csvData := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-5fd4f087-86f3-7a43-b711-4771313afc50,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,k8s_dcgm-exporter,dcgm-exporter-abcd,gpu-metrics,42.5,"DCGM_FI_DRIVER_VERSION=""535.129.03"""`

	parser := NewParser()
	reader := strings.NewReader(csvData)

	gpus, telemetry, err := parser.ParseCSV(reader)
	require.NoError(t, err)

	// Should have 1 GPU
	assert.Len(t, gpus, 1)
	gpu, exists := gpus["GPU-5fd4f087-86f3-7a43-b711-4771313afc50"]
	require.True(t, exists)

	// Verify GPU fields
	assert.Equal(t, "GPU-5fd4f087-86f3-7a43-b711-4771313afc50", gpu.UUID)
	assert.Equal(t, "nvidia0", gpu.DeviceID)
	assert.Equal(t, "0", gpu.GPUIndex)
	assert.Equal(t, "NVIDIA A100-SXM4-80GB", gpu.ModelName)
	assert.Equal(t, "mtv5-dgx1-hgpu-031", gpu.Hostname)
	assert.Equal(t, "k8s_dcgm-exporter", gpu.Container)
	assert.Equal(t, "dcgm-exporter-abcd", gpu.Pod)
	assert.Equal(t, "gpu-metrics", gpu.Namespace)

	// Verify composite key
	assert.Equal(t, "mtv5-dgx1-hgpu-031:nvidia0", gpu.GetCompositeKey())

	// Should have 1 telemetry point
	assert.Len(t, telemetry, 1)

	// Verify telemetry fields
	point := telemetry[0]
	assert.Equal(t, "GPU-5fd4f087-86f3-7a43-b711-4771313afc50", point.GPUUUID)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", point.MetricName)
	assert.Equal(t, "42.5", point.Value)
	assert.False(t, point.Timestamp.IsZero(), "Timestamp should be set")
}

func TestParseCSV_MultipleMetrics(t *testing.T) {
	csvData := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-5fd4f087-86f3-7a43-b711-4771313afc50,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,,,gpu-metrics,42.5,""
2024-12-18T10:32:45Z,DCGM_FI_DEV_POWER_USAGE,0,nvidia0,GPU-5fd4f087-86f3-7a43-b711-4771313afc50,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,,,gpu-metrics,300.794,""
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-5fd4f087-86f3-7a43-b711-4771313afc50,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,,,gpu-metrics,65,""
`

	parser := NewParser()
	reader := strings.NewReader(csvData)

	gpus, telemetry, err := parser.ParseCSV(reader)
	require.NoError(t, err)

	// Should have 1 GPU (deduplicated)
	assert.Len(t, gpus, 1)

	// Should have 3 telemetry points
	assert.Len(t, telemetry, 3)

	// Verify all metrics
	metricNames := []string{
		telemetry[0].MetricName,
		telemetry[1].MetricName,
		telemetry[2].MetricName,
	}
	assert.Contains(t, metricNames, "DCGM_FI_DEV_GPU_UTIL")
	assert.Contains(t, metricNames, "DCGM_FI_DEV_POWER_USAGE")
	assert.Contains(t, metricNames, "DCGM_FI_DEV_GPU_TEMP")

	// Verify values
	values := []string{
		telemetry[0].Value,
		telemetry[1].Value,
		telemetry[2].Value,
	}
	assert.Contains(t, values, "42.5")
	assert.Contains(t, values, "300.794")
	assert.Contains(t, values, "65")
}

func TestParseCSV_MultipleGPUs(t *testing.T) {
	csvData := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-5fd4f087-86f3-7a43-b711-4771313afc50,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,,,gpu-metrics,42.5,""
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_UTIL,1,nvidia1,GPU-8abc1234-56f7-8def-9012-3456789abcde,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,,,gpu-metrics,55.2,""
`

	parser := NewParser()
	reader := strings.NewReader(csvData)

	gpus, telemetry, err := parser.ParseCSV(reader)
	require.NoError(t, err)

	// Should have 2 GPUs
	assert.Len(t, gpus, 2)

	// Verify both GPUs exist
	_, exists1 := gpus["GPU-5fd4f087-86f3-7a43-b711-4771313afc50"]
	assert.True(t, exists1)
	_, exists2 := gpus["GPU-8abc1234-56f7-8def-9012-3456789abcde"]
	assert.True(t, exists2)

	// Should have 2 telemetry points
	assert.Len(t, telemetry, 2)
}

func TestValidateMetricType(t *testing.T) {
	parser := NewParser()

	// Valid metrics
	assert.True(t, parser.ValidateMetricType("DCGM_FI_DEV_GPU_UTIL"))
	assert.True(t, parser.ValidateMetricType("DCGM_FI_DEV_POWER_USAGE"))
	assert.True(t, parser.ValidateMetricType("DCGM_FI_DEV_GPU_TEMP"))

	// Invalid metric
	assert.False(t, parser.ValidateMetricType("INVALID_METRIC"))
}

func TestParseCSV_EmptyUUID(t *testing.T) {
	csvData := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2024-12-18T10:32:45Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,,NVIDIA A100-SXM4-80GB,mtv5-dgx1-hgpu-031,,,gpu-metrics,42.5,""
`

	parser := NewParser()
	reader := strings.NewReader(csvData)

	gpus, telemetry, err := parser.ParseCSV(reader)
	require.NoError(t, err)

	// Should skip rows with empty UUID
	assert.Len(t, gpus, 0)
	assert.Len(t, telemetry, 0)
}
