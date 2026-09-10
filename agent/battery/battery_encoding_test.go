package battery

import (
	"testing"
	"unicode/utf8"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeBatteriesRemovesInvalidUTF8BeforeNaming(t *testing.T) {
	// Given OS names containing invalid bytes, Unicode and duplicates.
	batteries := []Battery{{Name: "\xff"}, {Name: " 鼠标\xfe "}, {Name: "鼠标"}}

	// When names are normalized for the telemetry payload.
	got := normalizeBatteries(batteries)

	// Then invalid bytes are removed before fallback and duplicate handling.
	assert.Equal(t, []Battery{{Name: "Battery 1"}, {Name: "鼠标"}, {Name: "鼠标 (2)"}}, got)
	for _, battery := range got {
		assert.True(t, utf8.ValidString(battery.Name))
	}
}

func TestNormalizedBatteryNamesRoundTripThroughCBOR(t *testing.T) {
	// Given a normalized OS battery name that originally contained invalid bytes.
	batteries := normalizeBatteries([]Battery{{Name: "Battery\xff", Percent: 80}})
	encoded, err := cbor.Marshal(batteries)
	require.NoError(t, err)

	// When the receiver decodes the CBOR telemetry.
	var decoded []Battery
	err = cbor.Unmarshal(encoded, &decoded)

	// Then the payload is accepted and preserves its readings.
	require.NoError(t, err)
	assert.Equal(t, batteries, decoded)
}
