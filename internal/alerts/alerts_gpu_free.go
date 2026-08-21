package alerts

import (
	"log/slog"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

// GpuMemoryFree "sustained" alert sampler.
//
// Semantics (per user spec): trigger only when the maximum free VRAM across
// GPUs stays above the threshold for the whole window — the window (y
// minutes) is probed with 20 evenly spaced samples (interval = 3y seconds),
// and all 20 must satisfy the threshold. Changing the (threshold, window)
// pair resets the counter.

const gpuFreeSamples = 20

type gpuFreeState struct {
	threshold  float64
	window     uint8 // minutes
	cnt        int   // consecutive satisfying samples
	lastSample time.Time
	triggered  bool
	alertData  CachedAlertData
	systemID   string
}

type gpuFreeSampler struct {
	mu     sync.Mutex
	states map[string]*gpuFreeState // key: alert record id
}

func newGpuFreeSampler() gpuFreeSampler {
	return gpuFreeSampler{states: make(map[string]*gpuFreeState)}
}

// SampleGpuFreeAlerts evaluates GpuMemoryFree alerts for one system using
// its latest CombinedData. Safe to call concurrently and as often as
// desired; internal pacing enforces the even sampling interval.
func (am *AlertManager) SampleGpuFreeAlerts(systemID string, data *system.CombinedData) {
	if data == nil || len(data.Stats.GPUData) == 0 {
		am.gpuFree.dropSystem(systemID)
		return
	}
	// current max free VRAM across GPUs (GB)
	maxFree := 0.0
	for _, gpu := range data.Stats.GPUData {
		if gpu.MemoryTotal > 0 {
			free := (gpu.MemoryTotal - max(0, min(gpu.MemoryUsed, gpu.MemoryTotal))) / 1024
			if free > maxFree {
				maxFree = free
			}
		}
	}
	if maxFree <= 0 {
		am.gpuFree.dropSystem(systemID)
		return
	}

	alertsData := am.alertsCache.GetAlertsExcludingNames(systemID, "Status")
	am.gpuFree.mu.Lock()
	defer am.gpuFree.mu.Unlock()
	now := time.Now()
	seen := make(map[string]struct{}, len(alertsData))
	for _, ad := range alertsData {
		if ad.Name != "GpuMemoryFree" {
			continue
		}
		seen[ad.Id] = struct{}{}
		st, ok := am.gpuFree.states[ad.Id]
		if !ok {
			st = &gpuFreeState{}
			am.gpuFree.states[ad.Id] = st
		}
		// any (threshold, window) change resets the counter
		if !ok || st.threshold != ad.Value || st.window != ad.Min {
			st.threshold = ad.Value
			st.window = ad.Min
			st.cnt = 0
			st.alertData = ad
			st.systemID = systemID
			st.lastSample = time.Time{}
		}
		// even pacing: one sample per window/20
		interval := time.Duration(st.window) * time.Minute / gpuFreeSamples
		if interval <= 0 || now.Sub(st.lastSample) < interval {
			continue
		}
		st.lastSample = now
		satisfied := maxFree > st.threshold
		switch {
		case st.triggered:
			if !satisfied {
				am.finishGpuFree(st, maxFree, false)
			}
		case satisfied:
			st.cnt++
			if st.cnt >= gpuFreeSamples {
				am.finishGpuFree(st, maxFree, true)
			}
		default:
			st.cnt = 0
		}
	}
	// drop trackers of THIS system whose alert records no longer exist.
	// Trackers of other systems are owned by their own evaluations — the
	// states map is shared across all systems.
	for id, st := range am.gpuFree.states {
		if st.systemID == systemID {
			if _, ok := seen[id]; !ok {
				delete(am.gpuFree.states, id)
			}
		}
	}
}

// dropSystem removes trackers for a system that has no GPU data right now.
// Alert records usually still exist; they will be re-created on next sample.
func (s *gpuFreeSampler) dropSystem(systemID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, st := range s.states {
		if st.systemID == systemID {
			delete(s.states, id)
		}
	}
}

// finishGpuFree flips the triggered flag and sends the notification.
// Called with s.mu held.
func (am *AlertManager) finishGpuFree(st *gpuFreeState, val float64, triggered bool) {
	st.triggered = triggered
	if triggered {
		st.cnt = gpuFreeSamples
	} else {
		st.cnt = 0
	}
	record, err := am.hub.FindRecordById("systems", st.systemID)
	if err != nil {
		slog.Error("gpu free alert: system record not found", "system", st.systemID, "err", err)
		return
	}
	alert := SystemAlertData{
		systemRecord: record,
		alertData:    st.alertData,
		name:         "GpuMemoryFree",
		unit:         " GB",
		val:          val,
		threshold:    st.threshold,
		triggered:    triggered,
		min:          max(1, st.window),
		descriptor:   "Free VRAM",
	}
	go am.sendSystemAlert(alert)
}
