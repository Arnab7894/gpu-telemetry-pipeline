package dto

import "time"

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Error     string    `json:"error" example:"GPU not found"`
	Message   string    `json:"message" example:"No GPU found with UUID: GPU-invalid-uuid"`
	Timestamp time.Time `json:"timestamp" example:"2025-01-18T12:34:56Z"`
}
