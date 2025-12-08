package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Test configuration
const (
	APIBaseURL      = "http://localhost:8080"
	MaxWaitDuration = 120 * time.Second
	PollInterval    = 5 * time.Second
)

// GPU represents the GPU response from the API
type GPU struct {
	ID          string `json:"id"`
	Hostname    string `json:"hostname"`
	DeviceID    string `json:"device_id"`
	GPUIndex    int    `json:"gpu_index"`
	GPUUuid     string `json:"gpu_uuid"`
	DeviceModel string `json:"device_model"`
	Vendor      string `json:"vendor"`
	TotalCount  int    `json:"total_telemetry_count"`
	LastUpdated string `json:"last_updated"`
}

// TelemetryPoint represents a single telemetry measurement
type TelemetryPoint struct {
	GPUID              string    `json:"gpu_id"`
	Timestamp          time.Time `json:"timestamp"`
	GPUUtilization     float64   `json:"gpu_utilization"`
	MemoryUtilization  float64   `json:"memory_utilization"`
	PowerUsage         float64   `json:"power_usage"`
	Temperature        float64   `json:"temperature"`
	SMClock            int       `json:"sm_clock"`
	MemoryClock        int       `json:"memory_clock"`
	FrameBufferMemUsed int64     `json:"frame_buffer_mem_used"`
	FrameBufferMemFree int64     `json:"frame_buffer_mem_free"`
	PcieRxBytes        int64     `json:"pcie_rx_bytes"`
	PcieTxBytes        int64     `json:"pcie_tx_bytes"`
}

// TestResult tracks the result of each test
type TestResult struct {
	Name     string
	Passed   bool
	Error    error
	Duration time.Duration
}

func main() {
	fmt.Println("=== GPU Telemetry Pipeline E2E System Test ===\n")

	ctx := context.Background()
	results := []TestResult{}

	// Test 1: API Health Check
	results = append(results, runTest("API Health Check", func() error {
		return testHealthCheck(ctx)
	}))

	// Test 2: Wait for Data Ingestion
	results = append(results, runTest("Wait for Data Ingestion", func() error {
		return waitForDataIngestion(ctx)
	}))

	// Test 3: List All GPUs
	var gpus []GPU
	results = append(results, runTest("List All GPUs", func() error {
		var err error
		gpus, err = testListGPUs(ctx)
		return err
	}))

	// Test 4: Get GPU Telemetry
	if len(gpus) > 0 {
		results = append(results, runTest("Get GPU Telemetry", func() error {
			return testGetTelemetry(ctx, gpus[0].ID)
		}))

		// Test 5: Get GPU Telemetry with Time Range
		results = append(results, runTest("Get GPU Telemetry with Time Range", func() error {
			return testGetTelemetryWithTimeRange(ctx, gpus[0].ID)
		}))

		// Test 6: Verify Telemetry Order
		results = append(results, runTest("Verify Telemetry Timestamp Order", func() error {
			return testTelemetryOrder(ctx, gpus[0].ID)
		}))

		// Test 7: Verify Data Freshness
		results = append(results, runTest("Verify Data Freshness", func() error {
			return testDataFreshness(ctx, gpus[0])
		}))
	}

	// Test 8: Swagger Documentation
	results = append(results, runTest("Swagger Documentation Available", func() error {
		return testSwaggerDocs(ctx)
	}))

	// Print results
	fmt.Println("\n=== Test Results ===\n")
	passed := 0
	failed := 0
	for _, result := range results {
		status := "✅ PASS"
		if !result.Passed {
			status = "❌ FAIL"
			failed++
		} else {
			passed++
		}
		fmt.Printf("%s %s (%.2fs)\n", status, result.Name, result.Duration.Seconds())
		if result.Error != nil {
			fmt.Printf("   Error: %v\n", result.Error)
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total: %d | Passed: %d | Failed: %d\n", len(results), passed, failed)

	if failed > 0 {
		os.Exit(1)
	}
}

func runTest(name string, testFunc func() error) TestResult {
	fmt.Printf("Running: %s...\n", name)
	start := time.Now()
	err := testFunc()
	duration := time.Since(start)

	result := TestResult{
		Name:     name,
		Passed:   err == nil,
		Error:    err,
		Duration: duration,
	}

	if err != nil {
		fmt.Printf("  ❌ Failed: %v\n", err)
	} else {
		fmt.Printf("  ✅ Passed\n")
	}

	return result
}

func testHealthCheck(ctx context.Context) error {
	resp, err := http.Get(APIBaseURL + "/health")
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("  Health response: %s\n", string(body))
	return nil
}

func waitForDataIngestion(ctx context.Context) error {
	fmt.Printf("  Waiting for telemetry data (max %v)...\n", MaxWaitDuration)

	deadline := time.Now().Add(MaxWaitDuration)
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		gpus, err := getGPUs(ctx)
		if err == nil && len(gpus) > 0 {
			fmt.Printf("  Data available! Found %d GPUs\n", len(gpus))
			return nil
		}

		fmt.Printf("  Waiting... (no data yet)\n")
		<-ticker.C
	}

	return fmt.Errorf("timeout: no data ingested after %v", MaxWaitDuration)
}

func testListGPUs(ctx context.Context) ([]GPU, error) {
	gpus, err := getGPUs(ctx)
	if err != nil {
		return nil, err
	}

	if len(gpus) == 0 {
		return nil, fmt.Errorf("expected at least one GPU, got zero")
	}

	fmt.Printf("  Found %d GPUs\n", len(gpus))
	for i, gpu := range gpus {
		if i < 3 { // Print first 3
			fmt.Printf("    - %s (model: %s, telemetry: %d points)\n",
				gpu.ID, gpu.DeviceModel, gpu.TotalCount)
		}
	}
	if len(gpus) > 3 {
		fmt.Printf("    ... and %d more\n", len(gpus)-3)
	}

	return gpus, nil
}

func testGetTelemetry(ctx context.Context, gpuID string) error {
	telemetry, err := getTelemetry(ctx, gpuID, "", "")
	if err != nil {
		return err
	}

	if len(telemetry) == 0 {
		return fmt.Errorf("expected telemetry data for GPU %s, got zero points", gpuID)
	}

	fmt.Printf("  Retrieved %d telemetry points for GPU %s\n", len(telemetry), gpuID)
	if len(telemetry) > 0 {
		fmt.Printf("    Latest: timestamp=%s, gpu_util=%.1f%%, mem_util=%.1f%%\n",
			telemetry[0].Timestamp.Format(time.RFC3339),
			telemetry[0].GPUUtilization,
			telemetry[0].MemoryUtilization)
	}

	return nil
}

func testGetTelemetryWithTimeRange(ctx context.Context, gpuID string) error {
	// Get telemetry from last 10 minutes
	endTime := time.Now()
	startTime := endTime.Add(-10 * time.Minute)

	telemetry, err := getTelemetry(ctx, gpuID,
		startTime.Format(time.RFC3339),
		endTime.Format(time.RFC3339))
	if err != nil {
		return err
	}

	fmt.Printf("  Retrieved %d telemetry points in time range\n", len(telemetry))

	// Verify all points are within range
	for _, point := range telemetry {
		if point.Timestamp.Before(startTime) || point.Timestamp.After(endTime) {
			return fmt.Errorf("telemetry point timestamp %v outside requested range [%v, %v]",
				point.Timestamp, startTime, endTime)
		}
	}

	return nil
}

func testTelemetryOrder(ctx context.Context, gpuID string) error {
	telemetry, err := getTelemetry(ctx, gpuID, "", "")
	if err != nil {
		return err
	}

	if len(telemetry) < 2 {
		fmt.Printf("  Skipping order check (need at least 2 points, got %d)\n", len(telemetry))
		return nil
	}

	// Verify telemetry is ordered by timestamp (descending - newest first)
	for i := 0; i < len(telemetry)-1; i++ {
		if telemetry[i].Timestamp.Before(telemetry[i+1].Timestamp) {
			return fmt.Errorf("telemetry not properly ordered: point %d (%v) is before point %d (%v)",
				i, telemetry[i].Timestamp, i+1, telemetry[i+1].Timestamp)
		}
	}

	fmt.Printf("  ✓ Telemetry is properly ordered (newest first)\n")
	return nil
}

func testDataFreshness(ctx context.Context, gpu GPU) error {
	// Parse last updated timestamp
	lastUpdated, err := time.Parse(time.RFC3339, gpu.LastUpdated)
	if err != nil {
		return fmt.Errorf("failed to parse last_updated timestamp: %w", err)
	}

	// Data should be recent (within last 5 minutes)
	age := time.Since(lastUpdated)
	if age > 5*time.Minute {
		return fmt.Errorf("data is stale: last updated %v ago", age)
	}

	fmt.Printf("  ✓ Data is fresh (last updated %v ago)\n", age.Round(time.Second))
	return nil
}

func testSwaggerDocs(ctx context.Context) error {
	resp, err := http.Get(APIBaseURL + "/swagger/index.html")
	if err != nil {
		return fmt.Errorf("swagger docs request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	fmt.Printf("  ✓ Swagger documentation is accessible\n")
	return nil
}

// Helper functions

func getGPUs(ctx context.Context) ([]GPU, error) {
	resp, err := http.Get(APIBaseURL + "/api/v1/gpus")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var gpus []GPU
	if err := json.NewDecoder(resp.Body).Decode(&gpus); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return gpus, nil
}

func getTelemetry(ctx context.Context, gpuID, startTime, endTime string) ([]TelemetryPoint, error) {
	url := fmt.Sprintf("%s/api/v1/gpus/%s/telemetry", APIBaseURL, gpuID)
	if startTime != "" || endTime != "" {
		url += "?"
		if startTime != "" {
			url += "start_time=" + startTime
		}
		if endTime != "" {
			if startTime != "" {
				url += "&"
			}
			url += "end_time=" + endTime
		}
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var telemetry []TelemetryPoint
	if err := json.NewDecoder(resp.Body).Decode(&telemetry); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return telemetry, nil
}
