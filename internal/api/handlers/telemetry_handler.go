package handlers

import (
	"net/http"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/api/dto"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/gin-gonic/gin"
)

// TelemetryHandler handles telemetry-related API requests
type TelemetryHandler struct {
	telemetryRepo storage.TelemetryRepository
	gpuRepo       storage.GPURepository
}

// NewTelemetryHandler creates a new telemetry handler
func NewTelemetryHandler(telemetryRepo storage.TelemetryRepository, gpuRepo storage.GPURepository) *TelemetryHandler {
	return &TelemetryHandler{
		telemetryRepo: telemetryRepo,
		gpuRepo:       gpuRepo,
	}
}

// GetGPUTelemetry godoc
// @Summary Get GPU telemetry data
// @Description Get telemetry data for a specific GPU with optional time filtering. Results are ordered by timestamp (newest first).
// @Tags telemetry
// @Accept json
// @Produce json
// @Param uuid path string true "GPU UUID" example("GPU-5fd4f087-86f3-7a43-b711-4771313afc50")
// @Param start_time query string false "Start time in RFC3339 format" example("2025-01-18T00:00:00Z")
// @Param end_time query string false "End time in RFC3339 format" example("2025-01-18T23:59:59Z")
// @Success 200 {object} dto.TelemetryListResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/gpus/{uuid}/telemetry [get]
func (h *TelemetryHandler) GetGPUTelemetry(c *gin.Context) {
	uuid := c.Param("uuid")

	// Validate UUID parameter
	if uuid == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:     "Invalid request",
			Message:   "GPU UUID is required",
			Timestamp: time.Now(),
		})
		return
	}

	// Verify GPU exists
	_, err := h.gpuRepo.GetByUUID(uuid)
	if err != nil {
		if err == domain.ErrGPUNotFound {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{
				Error:     "GPU not found",
				Message:   "No GPU found with UUID: " + uuid,
				Timestamp: time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:     "Failed to verify GPU",
			Message:   "Internal server error occurred while verifying GPU",
			Timestamp: time.Now(),
		})
		return
	}

	// Parse optional time filters
	filter := storage.TimeFilter{}

	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:     "Invalid start_time format",
				Message:   "Use RFC3339 format (e.g., 2023-01-01T00:00:00Z). Got: " + startTimeStr,
				Timestamp: time.Now(),
			})
			return
		}
		filter.StartTime = &startTime
	}

	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:     "Invalid end_time format",
				Message:   "Use RFC3339 format (e.g., 2023-01-01T00:00:00Z). Got: " + endTimeStr,
				Timestamp: time.Now(),
			})
			return
		}
		filter.EndTime = &endTime
	}

	// Validate time range if both are provided
	if filter.StartTime != nil && filter.EndTime != nil {
		if filter.StartTime.After(*filter.EndTime) {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:     "Invalid time range",
				Message:   "start_time must be before end_time",
				Timestamp: time.Now(),
			})
			return
		}
	}

	// Retrieve telemetry data
	telemetry, err := h.telemetryRepo.GetByGPU(uuid, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:     "Failed to retrieve telemetry data",
			Message:   "Internal server error occurred while fetching telemetry",
			Timestamp: time.Now(),
		})
		return
	}

	// Convert domain models to DTOs
	response := dto.ToTelemetryListResponse(telemetry, uuid, filter)

	// Add filter info to response
	if filter.StartTime != nil {
		response.StartTime = filter.StartTime
	}
	if filter.EndTime != nil {
		response.EndTime = filter.EndTime
	}

	c.JSON(http.StatusOK, response)
}
