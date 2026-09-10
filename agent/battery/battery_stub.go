//go:build !darwin && !linux

package battery

import "errors"

func HasReadableBattery() bool {
	return false
}

func GetBatteryStats() ([]Battery, error) {
	return nil, errors.ErrUnsupported
}
