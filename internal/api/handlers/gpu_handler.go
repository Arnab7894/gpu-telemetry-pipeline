package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/api/dto"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/domain"
	"github.com/arnabghosh/gpu-metrics-streamer/internal/storage"
	"github.com/gin-gonic/gin"
)

// GPUHandler handles GPU-related API requests
type GPUHandler struct {
	gpuRepo storage.GPURepository
}

// NewGPUHandler creates a new GPU handler
func NewGPUHandler(gpuRepo storage.GPURepository) *GPUHandler {
	return &GPUHandler{
		gpuRepo: gpuRepo,
	}
}

// ListGPUs godoc
// @Summary List all GPUs
// @Description Get list of all GPU devices for which telemetry data exists
// @Tags gpus
// @Accept json
// @Produce json
// @Success 200 {object} dto.GPUListResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /gpus [get]
func (h *GPUHandler) ListGPUs(c *gin.Context) {
	gpus, err := h.gpuRepo.List()
	if err != nil {
		slog.Error("Failed to retrieve GPUs",
			"error", err,
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
		)
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:     "Failed to retrieve GPUs",
			Message:   "Internal server error occurred while fetching GPU list",
			Timestamp: time.Now(),
		})
		return
	}

	slog.Info("Listed GPUs",
		"count", len(gpus),
		"path", c.Request.URL.Path,
	)

	// Convert domain models to DTOs
	response := dto.ToGPUListResponse(gpus)
	c.JSON(http.StatusOK, response)
}

// GetGPU godoc
// @Summary Get GPU by UUID
// @Description Get a specific GPU device by its UUID
// @Tags gpus
// @Accept json
// @Produce json
// @Param uuid path string true "GPU UUID" example("GPU-5fd4f087-86f3-7a43-b711-4771313afc50")
// @Success 200 {object} dto.GPUResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /gpus/{uuid} [get]
func (h *GPUHandler) GetGPU(c *gin.Context) {
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

	gpu, err := h.gpuRepo.GetByUUID(uuid)
	if err != nil {
		if err == domain.ErrGPUNotFound {
			slog.Info("GPU not found",
				"uuid", uuid,
				"path", c.Request.URL.Path,
			)
			c.JSON(http.StatusNotFound, dto.ErrorResponse{
				Error:     "GPU not found",
				Message:   "No GPU found with UUID: " + uuid,
				Timestamp: time.Now(),
			})
			return
		}
		slog.Error("Failed to retrieve GPU",
			"error", err,
			"uuid", uuid,
			"path", c.Request.URL.Path,
		)
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:     "Failed to retrieve GPU",
			Message:   "Internal server error occurred",
			Timestamp: time.Now(),
		})
		return
	}

	// Convert domain model to DTO
	response := dto.ToGPUResponse(gpu)
	c.JSON(http.StatusOK, response)
}
