package telemetry

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
)

// Parser handles CSV telemetry data parsing
type Parser struct{}

// NewParser creates a new Parser instance
func NewParser() *Parser {
	return &Parser{}
}

// ParseCSV reads CSV from reader and returns GPU and TelemetryPoint records
// Returns: (gpus, telemetry points, error)
// Note: CSV timestamp is IGNORED - we use time.Now() for ingestion timestamp
func (p *Parser) ParseCSV(reader io.Reader) (map[string]*domain.GPU, []*domain.TelemetryPoint, error) {
	csvReader := csv.NewReader(reader)

	// Read header
	header, err := csvReader.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Validate expected columns
	expectedCols := 12
	if len(header) != expectedCols {
		return nil, nil, fmt.Errorf("expected %d columns, got %d", expectedCols, len(header))
	}

	gpus := make(map[string]*domain.GPU)
	telemetry := make([]*domain.TelemetryPoint, 0)
	ingestionTime := time.Now() // Single timestamp for this batch

	lineNum := 1
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("error reading CSV line %d: %w", lineNum, err)
		}
		lineNum++

		if len(record) != expectedCols {
			continue // Skip malformed rows
		}

		// Parse CSV row into helper struct
		row := domain.CSVRow{
			Timestamp:  record[0],
			MetricName: record[1],
			GPUIndex:   record[2],
			Device:     record[3],
			UUID:       record[4],
			ModelName:  record[5],
			Hostname:   record[6],
			Container:  record[7],
			Pod:        record[8],
			Namespace:  record[9],
			Value:      record[10],
			LabelsRaw:  record[11],
		} // Skip rows with empty UUID (invalid)
		if strings.TrimSpace(row.UUID) == "" {
			continue
		}

		// Create or update GPU record
		if _, exists := gpus[row.UUID]; !exists {
			gpus[row.UUID] = &domain.GPU{
				UUID:      row.UUID,
				DeviceID:  row.Device,
				GPUIndex:  row.GPUIndex,
				ModelName: row.ModelName,
				Hostname:  row.Hostname,
				Container: row.Container,
				Pod:       row.Pod,
				Namespace: row.Namespace,
			}
		} // Create telemetry point with ingestion timestamp (NOT CSV timestamp)
		point := &domain.TelemetryPoint{
			GPUUUID:    row.UUID,
			MetricName: row.MetricName,
			Value:      row.Value,
			Timestamp:  ingestionTime, // Use current time, not CSV timestamp
			LabelsRaw:  row.LabelsRaw,
		}
		telemetry = append(telemetry, point)
	}

	return gpus, telemetry, nil
}

// ValidateMetricType checks if the metric name is a known DCGM metric
func (p *Parser) ValidateMetricType(metricName string) bool {
	validMetrics := map[string]bool{
		string(domain.MetricGPUUtil):     true,
		string(domain.MetricMemCopyUtil): true,
		string(domain.MetricEncUtil):     true,
		string(domain.MetricDecUtil):     true,
		string(domain.MetricFBUsed):      true,
		string(domain.MetricFBFree):      true,
		string(domain.MetricMemClock):    true,
		string(domain.MetricSMClock):     true,
		string(domain.MetricPowerUsage):  true,
		string(domain.MetricGPUTemp):     true,
	}
	return validMetrics[metricName]
}
