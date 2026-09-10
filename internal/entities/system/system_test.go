package system

import (
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsBatteryTransport(t *testing.T) {
	stats := Stats{Battery: [2]uint8{0, 1}, Batteries: map[string]uint8{"Primary": 0, "Mouse": 75}}

	jsonData, err := json.Marshal(stats)
	require.NoError(t, err)
	var jsonPayload map[string]any
	require.NoError(t, json.Unmarshal(jsonData, &jsonPayload))
	assert.Equal(t, []any{float64(0), float64(1)}, jsonPayload["bat"])
	assert.Equal(t, map[string]any{"Primary": float64(0), "Mouse": float64(75)}, jsonPayload["bats"])

	cborData, err := cbor.Marshal(stats)
	require.NoError(t, err)
	var decoded Stats
	require.NoError(t, cbor.Unmarshal(cborData, &decoded))
	assert.Equal(t, stats.Battery, decoded.Battery)
	assert.Equal(t, stats.Batteries, decoded.Batteries)
}

func TestStatsLegacyBatteryPayload(t *testing.T) {
	data, err := json.Marshal(Stats{Battery: [2]uint8{50, 4}})
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	assert.Contains(t, payload, "bat")
	assert.NotContains(t, payload, "bats")
}

func TestInfoGpuZeroPreservesTelemetryPresence(t *testing.T) {
	gpuPct := 0.0
	data, err := json.Marshal(Info{GpuPct: &gpuPct})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	assert.Contains(t, payload, "g")
	assert.Equal(t, 0.0, payload["g"])

	data, err = json.Marshal(Info{})
	require.NoError(t, err)
	var emptyPayload map[string]any
	require.NoError(t, json.Unmarshal(data, &emptyPayload))
	assert.NotContains(t, emptyPayload, "g")
}

func TestInfoMaxTemperatureZeroPreservesTelemetryPresence(t *testing.T) {
	maxTemp := int16(0)
	data, err := json.Marshal(Info{MaxTemp: &maxTemp})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(data, &payload))
	assert.Equal(t, 0.0, payload["mt"])
}
