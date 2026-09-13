//go:build testing

package alerts

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlertManagerStopWaitsForActiveWork(t *testing.T) {
	am := &AlertManager{}
	started := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{})

	// Given background work that has started but cannot finish yet.
	require.True(t, am.runAsync(func() {
		close(started)
		<-release
	}))
	<-started

	// When Stop runs concurrently, it must remain blocked on that work.
	go func() {
		am.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop returned before active work completed")
	case <-time.After(25 * time.Millisecond):
	}

	// Then releasing the work allows Stop to complete.
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return after active work completed")
	}
}

func TestAlertManagerRejectsWorkAfterStop(t *testing.T) {
	am := &AlertManager{}
	am.Stop()

	// Given a stopped manager, no new asynchronous work may begin.
	ran := make(chan struct{}, 1)
	accepted := am.runAsync(func() { ran <- struct{}{} })

	// Then rejection is observable and the task is never invoked.
	assert.False(t, accepted)
	assert.False(t, am.schedulePendingStatusAlert("sys", "stopped", "", CachedAlertData{Id: "alert"}, time.Hour))
	assert.Zero(t, am.GetPendingAlertsCount())
	select {
	case <-ran:
		t.Fatal("work ran after Stop")
	default:
	}
}
