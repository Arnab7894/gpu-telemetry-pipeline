package parser

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
)

// CSVRecord represents a single row from the DCGM metrics CSV
type CSVRecord struct {
	Timestamp  string
	MetricName string
	GPUID      string
	Device     string
	UUID       string
	ModelName  string
	Hostname   string
	Container  string
	Pod        string
	Namespace  string
	Value      string
	LabelsRaw  string
}

// CSVParser parses DCGM metrics CSV data
type CSVParser struct {
	headerMap map[string]int
}

// NewCSVParser creates a new CSV parser
func NewCSVParser() *CSVParser {
	return &CSVParser{
		headerMap: make(map[string]int),
	}
}

// ParseHeader parses the CSV header and builds column index map
func (p *CSVParser) ParseHeader(header []string) error {
	if len(header) == 0 {
		return domain.ErrInvalidInput
	}

	// Build column index map
	for i, col := range header {
		p.headerMap[strings.TrimSpace(col)] = i
	}

	// Validate required columns
	required := []string{"timestamp", "metric_name", "gpu_id", "device", "uuid", "modelName", "Hostname", "value"}
	for _, col := range required {
		if _, exists := p.headerMap[col]; !exists {
			return fmt.Errorf("missing required column: %s", col)
		}
	}

	return nil
}

// ParseRow parses a single CSV row into CSVRecord
func (p *CSVParser) ParseRow(row []string) (*CSVRecord, error) {
	if len(row) == 0 {
		return nil, domain.ErrInvalidInput
	}

	// Ensure we have parsed the header
	if len(p.headerMap) == 0 {
		return nil, fmt.Errorf("header not parsed yet")
	}

	record := &CSVRecord{}

	// Helper to safely get column value
	getCol := func(name string) string {
		if idx, ok := p.headerMap[name]; ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}

	record.Timestamp = getCol("timestamp")
	record.MetricName = getCol("metric_name")
	record.GPUID = getCol("gpu_id")
	record.Device = getCol("device")
	record.UUID = getCol("uuid")
	record.ModelName = getCol("modelName")
	record.Hostname = getCol("Hostname")
	record.Container = getCol("container")
	record.Pod = getCol("pod")
	record.Namespace = getCol("namespace")
	record.Value = getCol("value")
	record.LabelsRaw = getCol("labels_raw")

	// Validate required fields
	if record.UUID == "" || record.MetricName == "" {
		return nil, fmt.Errorf("missing required fields: uuid or metric_name")
	}

	return record, nil
}

// ToTelemetryPoint converts CSVRecord to domain.TelemetryPoint
// Note: Timestamp will be overwritten by streamer with time.Now()
func (r *CSVRecord) ToTelemetryPoint() *domain.TelemetryPoint {
	return &domain.TelemetryPoint{
		GPUUUID:    r.UUID,
		MetricName: r.MetricName,
		Value:      r.Value,
		// Timestamp will be set by streamer
	}
}

// ToGPU converts CSVRecord to domain.GPU
func (r *CSVRecord) ToGPU() *domain.GPU {
	return &domain.GPU{
		UUID:      r.UUID,
		ModelName: r.ModelName,
		Hostname:  r.Hostname,
		DeviceID:  r.Device,
		GPUIndex:  r.GPUID,
		Container: r.Container,
		Pod:       r.Pod,
		Namespace: r.Namespace,
	}
}

// StreamReader provides an iterator-like interface for reading CSV records
type StreamReader struct {
	reader *csv.Reader
	parser *CSVParser
	row    int
}

// NewStreamReader creates a new stream reader for CSV data
func NewStreamReader(r io.Reader) (*StreamReader, error) {
	csvReader := csv.NewReader(r)
	csvReader.TrimLeadingSpace = true
	csvReader.ReuseRecord = true // Optimize memory usage

	parser := NewCSVParser()

	// Read and parse header
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	if err := parser.ParseHeader(header); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	return &StreamReader{
		reader: csvReader,
		parser: parser,
		row:    1, // Row 0 is header
	}, nil
}

// Next reads the next record from CSV
// Returns (record, nil) on success
// Returns (nil, io.EOF) when end of file is reached
// Returns (nil, error) on parse error
func (sr *StreamReader) Next() (*CSVRecord, error) {
	row, err := sr.reader.Read()
	if err != nil {
		return nil, err // io.EOF or other error
	}

	sr.row++
	record, err := sr.parser.ParseRow(row)
	if err != nil {
		return nil, fmt.Errorf("row %d: %w", sr.row, err)
	}

	return record, nil
}

// Row returns the current row number (1-indexed, excluding header)
func (sr *StreamReader) Row() int {
	return sr.row
}
