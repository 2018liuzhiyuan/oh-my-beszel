package alerts

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCPUStateAlertValue(t *testing.T) {
	for _, test := range []struct {
		name string
		kind string
		data []float64
		want float64
		ok   bool
	}{
		{"iowait", "CPUIOWait", []float64{10, 20, 30, 5, 35}, 30, true},
		{"steal", "CPUSteal", []float64{10, 20, 30, 5, 35}, 5, true},
		{"zero is a valid percentage", "CPUIOWait", []float64{0, 0, 0, 0, 100}, 0, true},
		{"full usage", "CPUSteal", []float64{0, 0, 0, 100, 0}, 100, true},
		{"missing", "CPUIOWait", nil, 0, false},
		{"short", "CPUSteal", []float64{0, 0, 0, 100}, 0, false},
		{"empty sample", "CPUSteal", []float64{0, 0, 0, 0, 0}, 0, false},
		{"nan", "CPUIOWait", []float64{math.NaN(), 0, 50, 0, 50}, 0, false},
		{"infinite", "CPUSteal", []float64{0, 0, 0, math.Inf(1), 0}, 0, false},
		{"negative", "CPUSteal", []float64{0, 0, 0, -1, 100}, 0, false},
		{"over 100", "CPUIOWait", []float64{0, 0, 101, 0, 0}, 0, false},
		{"unsupported alert", "CPUIdle", []float64{0, 0, 0, 0, 100}, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, ok := cpuStateAlertValue(test.kind, test.data)
			assert.Equal(t, test.want, value)
			assert.Equal(t, test.ok, ok)
		})
	}
}
