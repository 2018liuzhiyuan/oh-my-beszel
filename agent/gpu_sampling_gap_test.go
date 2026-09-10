//go:build testing

package agent

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCurrentDataPreservesIntelMetricsDuringSamplingGap(t *testing.T) {
	// Given an Intel collector without temperature or VRAM readings.
	gm := &GPUManager{GpuDataMap: make(map[string]*system.GPUData)}
	require.True(t, gm.updateIntelFromStats(&intelGpuStats{
		PowerGPU: 4.5, PowerPkg: 12, Engines: map[string]float64{"Render/3D": 25},
	}))
	previous := gm.GetCurrentData(5000)["i0"]

	// When the hub polls again before the collector produces another sample.
	current := gm.GetCurrentData(5000)["i0"]

	// Then the latest measured utilization and power survive the gap.
	assert.Equal(t, previous, current)
}

func TestGetCurrentDataStillClearsSuspendedGPU(t *testing.T) {
	// Given a previously active discrete GPU.
	gm := &GPUManager{GpuDataMap: map[string]*system.GPUData{
		"0": {Name: "Discrete GPU", Count: 1, Temperature: 50, Usage: 25, Power: 75},
	}}
	gm.GetCurrentData(5000)
	gm.GpuDataMap["0"].Temperature = 0

	// When the next poll sees zero instantaneous readings with no new sample.
	current := gm.GetCurrentData(5000)["0"]

	// Then existing suspension behavior still clears usage and power.
	assert.Zero(t, current.Usage)
	assert.Zero(t, current.Power)
}

func TestGetCurrentDataKeepsIntelNameBeforeFirstSample(t *testing.T) {
	gm := &GPUManager{GpuDataMap: map[string]*system.GPUData{
		"i0": {Name: "Intel GPU", Engines: map[string]float64{}},
	}}
	current := gm.GetCurrentData(5000)["i0"]
	assert.Equal(t, "Intel GPU", current.Name)
	assert.Zero(t, current.Power)
}
