package alerts

import "math"

var cpuStateAlerts = map[string]struct {
	index int
	label string
}{
	"CPUIOWait": {2, "CPU I/O Wait"},
	"CPUSteal":  {3, "CPU Steal Time"},
}

func cpuStateAlertValue(name string, breakdown []float64) (float64, bool) {
	state, ok := cpuStateAlerts[name]
	if !ok || len(breakdown) < 5 {
		return 0, false
	}
	var total float64
	for _, value := range breakdown {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			return 0, false
		}
		total += value
	}
	if total <= 0 {
		return 0, false
	}
	return breakdown[state.index], true
}
