package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/arnabghosh/gpu-metrics-streamer/internal/api/dto"
	"github.com/gin-gonic/gin"
)

// ErrorHandlerMiddleware handles errors and formats responses
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check if there were any errors during request processing
		if len(c.Errors) > 0 {
			err := c.Errors.Last()

			slog.Error("Request error",
				"error", err.Error(),
				"path", c.Request.URL.Path,
				"method", c.Request.Method,
			)

			// If response hasn't been written yet, send error response
			if !c.Writer.Written() {
				c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
					Error:     "Internal Server Error",
					Message:   err.Error(),
					Timestamp: time.Now(),
				})
			}
		}
	}
}
