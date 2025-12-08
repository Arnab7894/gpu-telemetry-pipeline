package parser

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSVParser_ParseHeader(t *testing.T) {
	parser := NewCSVParser()

	header := []string{"timestamp", "metric_name", "gpu_id", "device", "uuid", "modelName", "Hostname", "value", "labels_raw"}
	err := parser.ParseHeader(header)
	require.NoError(t, err)

	assert.Len(t, parser.headerMap, 9)
	assert.Equal(t, 0, parser.headerMap["timestamp"])
	assert.Equal(t, 1, parser.headerMap["metric_name"])
}

func TestCSVParser_ParseHeader_Empty(t *testing.T) {
	parser := NewCSVParser()
	err := parser.ParseHeader([]string{})
	assert.Error(t, err)
}

func TestCSVParser_ParseHeader_MissingColumn(t *testing.T) {
	parser := NewCSVParser()

	// Missing "uuid" column
	header := []string{"timestamp", "metric_name", "gpu_id"}
	err := parser.ParseHeader(header)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required column")
}

func TestCSVParser_ParseRow(t *testing.T) {
	parser := NewCSVParser()

	header := []string{"timestamp", "metric_name", "gpu_id", "device", "uuid", "modelName", "Hostname", "container", "pod", "namespace", "value", "labels_raw"}
	require.NoError(t, parser.ParseHeader(header))

	row := []string{
		"2025-07-18T20:42:34Z",
		"DCGM_FI_DEV_GPU_UTIL",
		"0",
		"nvidia0",
		"GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
		"NVIDIA H100 80GB HBM3",
		"mtv5-dgx1-hgpu-031",
		"",
		"",
		"",
		"75",
		"labels",
	}

	record, err := parser.ParseRow(row)
	require.NoError(t, err)
	assert.NotNil(t, record)

	assert.Equal(t, "2025-07-18T20:42:34Z", record.Timestamp)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", record.MetricName)
	assert.Equal(t, "0", record.GPUID)
	assert.Equal(t, "nvidia0", record.Device)
	assert.Equal(t, "GPU-5fd4f087-86f3-7a43-b711-4771313afc50", record.UUID)
	assert.Equal(t, "NVIDIA H100 80GB HBM3", record.ModelName)
	assert.Equal(t, "mtv5-dgx1-hgpu-031", record.Hostname)
	assert.Equal(t, "75", record.Value)
}

func TestCSVParser_ParseRow_EmptyRow(t *testing.T) {
	parser := NewCSVParser()
	record, err := parser.ParseRow([]string{})
	assert.Error(t, err)
	assert.Nil(t, record)
}

func TestCSVParser_ParseRow_NoHeader(t *testing.T) {
	parser := NewCSVParser()
	row := []string{"value1", "value2"}

	record, err := parser.ParseRow(row)
	assert.Error(t, err)
	assert.Nil(t, record)
	assert.Contains(t, err.Error(), "header not parsed")
}

func TestCSVParser_ParseRow_MissingRequiredField(t *testing.T) {
	parser := NewCSVParser()

	header := []string{"timestamp", "metric_name", "gpu_id", "device", "uuid", "modelName", "Hostname", "value"}
	require.NoError(t, parser.ParseHeader(header))

	// Missing UUID (empty string)
	row := []string{"2025-07-18T20:42:34Z", "DCGM_FI_DEV_GPU_UTIL", "0", "nvidia0", "", "NVIDIA H100", "host", "75"}

	record, err := parser.ParseRow(row)
	assert.Error(t, err)
	assert.Nil(t, record)
	assert.Contains(t, err.Error(), "missing required fields")
}

func TestCSVRecord_ToTelemetryPoint(t *testing.T) {
	record := &CSVRecord{
		UUID:       "GPU-123",
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		Value:      "75.5",
	}

	telemetry := record.ToTelemetryPoint()
	assert.NotNil(t, telemetry)
	assert.Equal(t, "GPU-123", telemetry.GPUUUID)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", telemetry.MetricName)
	assert.Equal(t, "75.5", telemetry.Value)
}

func TestCSVRecord_ToGPU(t *testing.T) {
	record := &CSVRecord{
		UUID:      "GPU-123",
		ModelName: "NVIDIA H100",
		Hostname:  "host-1",
		Device:    "nvidia0",
		GPUID:     "0",
		Container: "container-1",
		Pod:       "pod-1",
		Namespace: "default",
	}

	gpu := record.ToGPU()
	assert.NotNil(t, gpu)
	assert.Equal(t, "GPU-123", gpu.UUID)
	assert.Equal(t, "NVIDIA H100", gpu.ModelName)
	assert.Equal(t, "host-1", gpu.Hostname)
	assert.Equal(t, "nvidia0", gpu.DeviceID)
	assert.Equal(t, "0", gpu.GPUIndex)
	assert.Equal(t, "container-1", gpu.Container)
	assert.Equal(t, "pod-1", gpu.Pod)
	assert.Equal(t, "default", gpu.Namespace)
}

func TestStreamReader_New(t *testing.T) {
	csv := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-123,NVIDIA H100,host-1,,,, 75,labels`

	reader, err := NewStreamReader(strings.NewReader(csv))
	require.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestStreamReader_New_InvalidHeader(t *testing.T) {
	csv := `timestamp,metric_name
value1,value2`

	reader, err := NewStreamReader(strings.NewReader(csv))
	assert.Error(t, err)
	assert.Nil(t, reader)
}

func TestStreamReader_Next(t *testing.T) {
	csv := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-123,NVIDIA H100,host-1,,,, 75,labels
2025-07-18T20:42:35Z,DCGM_FI_DEV_MEM_UTIL,1,nvidia1,GPU-456,NVIDIA A100,host-2,,,, 50,labels`

	reader, err := NewStreamReader(strings.NewReader(csv))
	require.NoError(t, err)

	// Read first record
	record1, err := reader.Next()
	require.NoError(t, err)
	assert.Equal(t, "GPU-123", record1.UUID)
	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", record1.MetricName)
	assert.Equal(t, 2, reader.Row())

	// Read second record
	record2, err := reader.Next()
	require.NoError(t, err)
	assert.Equal(t, "GPU-456", record2.UUID)
	assert.Equal(t, "DCGM_FI_DEV_MEM_UTIL", record2.MetricName)
	assert.Equal(t, 3, reader.Row())

	// EOF
	record3, err := reader.Next()
	assert.Equal(t, io.EOF, err)
	assert.Nil(t, record3)
}

func TestStreamReader_Next_ParseError(t *testing.T) {
	// Missing required field (no uuid)
	csv := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,,NVIDIA H100,host-1,,,, 75,labels`

	reader, err := NewStreamReader(strings.NewReader(csv))
	require.NoError(t, err)

	record, err := reader.Next()
	assert.Error(t, err)
	assert.Nil(t, record)
	assert.Contains(t, err.Error(), "row 2")
}

func TestStreamReader_EmptyFile(t *testing.T) {
	csv := ``

	reader, err := NewStreamReader(strings.NewReader(csv))
	assert.Error(t, err)
	assert.Nil(t, reader)
}

func TestStreamReader_OnlyHeader(t *testing.T) {
	csv := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw`

	reader, err := NewStreamReader(strings.NewReader(csv))
	require.NoError(t, err)

	// EOF on first Next()
	record, err := reader.Next()
	assert.Equal(t, io.EOF, err)
	assert.Nil(t, record)
}
