//go:build testing

package alerts_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setCPUStateAlertValue(_ *system.Info, stats *system.Stats, value []float64) {
	stats.CpuBreakdown = value
}

func TestCPUStateAlertsTriggerAndRecover(t *testing.T) {
	for _, name := range []string{"CPUIOWait", "CPUSteal"} {
		for _, minutes := range []int{1, 2} {
			t.Run(name+time.Duration(minutes).String(), func(t *testing.T) {
				// Given: different CPU states ensure each alert reads its own metric.
				trigger := []float64{0, 0, 51, 0, 49}
				resolve := []float64{0, 0, 48, 0, 52}
				baseline := []float64{0, 0, 10, 0, 90}
				if name == "CPUSteal" {
					trigger[2], trigger[3] = trigger[3], trigger[2]
					resolve[2], resolve[3] = resolve[3], resolve[2]
					baseline[2], baseline[3] = baseline[3], baseline[2]
				}
				// When / Then: submit stats through CreateRecords and observe state and notifications.
				if minutes == 1 {
					testOneMinuteSystemAlert(t, name, 50, setCPUStateAlertValue, trigger, resolve)
				} else {
					testMultiMinuteSystemAlert(t, name, 50, minutes, setCPUStateAlertValue, baseline, trigger, resolve)
				}
			})
		}
	}
}

func TestCPUStateAlertsIgnoreUnavailableSamples(t *testing.T) {
	for _, name := range []string{"CPUIOWait", "CPUSteal"} {
		for _, triggered := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "Inactive", true: "Triggered"}[triggered], func(t *testing.T) {
				// Given: an existing alert whose state must survive missing or invalid agent data.
				fixture := newSystemAlertTestFixture(t, name, 1, 1)
				defer fixture.cleanup()
				record, err := fixture.hub.FindRecordById("alerts", fixture.alertID)
				require.NoError(t, err)
				record.Set("triggered", triggered)
				require.NoError(t, fixture.hub.Save(record))
				synctest.Test(t, func(t *testing.T) {
					// When: old agents or failed samplers emit no usable breakdown.
					for _, sample := range [][]float64{nil, {0, 0, 0}, {0, 0, 0, 0, 0}, {0, 0, -1, -1, 100}, {0, 0, 101, 101, 0}} {
						submitValue(fixture, t, sample, setCPUStateAlertValue)
					}
					waitForSystemAlert(time.Second)
					// Then: neither a false trigger nor a false recovery is emitted.
					fixture.assertTriggered(t, triggered, "Unavailable CPU samples must retain alert state")
					assert.Zero(t, fixture.hub.TestMailer.TotalSend())
				})
			})
		}
	}
}

func TestCPUStateAlertsRequireValidHistory(t *testing.T) {
	for _, name := range []string{"CPUIOWait", "CPUSteal"} {
		for _, triggered := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "Inactive", true: "Triggered"}[triggered], func(t *testing.T) {
				fixture := newSystemAlertTestFixture(t, name, 2, 50)
				defer fixture.cleanup()
				record, err := fixture.hub.FindRecordById("alerts", fixture.alertID)
				require.NoError(t, err)
				record.Set("triggered", triggered)
				require.NoError(t, fixture.hub.Save(record))
				current := []float64{0, 0, 0, 0, 100}
				if !triggered {
					current = []float64{0, 0, 60, 60, 0}
				}
				synctest.Test(t, func(t *testing.T) {
					submitValue(fixture, t, []float64(nil), setCPUStateAlertValue)
					waitForSystemAlert(time.Minute + time.Second)
					submitValue(fixture, t, []float64{0, 0, 0, 0, 0}, setCPUStateAlertValue)
					waitForSystemAlert(time.Minute)
					submitValue(fixture, t, current, setCPUStateAlertValue)
					waitForSystemAlert(time.Second)
					fixture.assertTriggered(t, triggered, "Invalid historical CPU samples cannot satisfy the duration")
					assert.Zero(t, fixture.hub.TestMailer.TotalSend())
				})
			})
		}
	}
}
