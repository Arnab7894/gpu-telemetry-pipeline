package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGPU_GetCompositeKey(t *testing.T) {
	gpu := &GPU{
		UUID:      "GPU-001",
		DeviceID:  "nvidia0",
		Hostname:  "host-001",
		Container: "my-container",
		Pod:       "my-pod",
		Namespace: "ml-team",
	}

	key := gpu.GetCompositeKey()
	expected := "host-001:nvidia0"
	assert.Equal(t, expected, key)
}

func TestGPU_GetCompositeKey_MinimalFields(t *testing.T) {
	gpu := &GPU{
		UUID:     "GPU-001",
		DeviceID: "nvidia0",
		Hostname: "host-001",
	}

	key := gpu.GetCompositeKey()
	expected := "host-001:nvidia0"
	assert.Equal(t, expected, key)
}
