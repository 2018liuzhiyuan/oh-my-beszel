//go:build !linux

package agent

// collectHardwareDetails is a no-op on non-Linux platforms.
func (a *Agent) collectHardwareDetails() {}
