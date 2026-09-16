package hub

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShouldScheduleAgentDeployment_whenSSHSystemIsPendingOrDown(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		sshConfig string
		want      bool
	}{
		{name: "pending SSH system", status: "pending", sshConfig: `C:\Users\tester\.ssh\config`, want: true},
		{name: "down SSH system", status: "down", sshConfig: `C:\Users\tester\.ssh\config`, want: true},
		{name: "healthy SSH system", status: "up", sshConfig: `C:\Users\tester\.ssh\config`, want: false},
		{name: "paused SSH system", status: "paused", sshConfig: `C:\Users\tester\.ssh\config`, want: false},
		{name: "ordinary pending system", status: "pending", sshConfig: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			got := shouldScheduleAgentDeployment(test.status, test.sshConfig)

			// Then
			require.Equal(t, test.want, got)
		})
	}
}

func TestAgentDeploymentManager_schedulesOnlyOneJobForDuplicateEvents(t *testing.T) {
	// Given
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var calls atomic.Int32
	manager := newAgentDeploymentManagerForTest(func(ctx context.Context, target agentDeploymentTarget) error {
		calls.Add(1)
		close(started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			close(finished)
			return nil
		}
	})
	t.Cleanup(manager.stop)
	target := agentDeploymentTarget{id: "system-1", host: "gpu-1", port: 45876, sshConfig: "/tmp/ssh-config"}

	// When
	manager.schedule(target, false)
	<-started
	manager.schedule(target, false)
	close(release)

	// Then
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("deployment did not finish")
	}
	require.Equal(t, int32(1), calls.Load())
}

func TestAgentDeploymentManager_cooldownThrottlesRescheduleAfterRunFinishes(t *testing.T) {
	// Given: a run that fails immediately and finishes (no retry delays)
	var calls atomic.Int32
	manager := newAgentDeploymentManagerForTest(func(ctx context.Context, target agentDeploymentTarget) error {
		calls.Add(1)
		return context.Canceled
	})
	t.Cleanup(manager.stop)
	target := agentDeploymentTarget{id: "system-1", host: "gpu-1", port: 45876, sshConfig: "/tmp/ssh-config"}

	// When: an update-driven schedule starts and finishes the run
	manager.schedule(target, false)
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		_, running := manager.running[target.id]
		return !running
	}, time.Second, 10*time.Millisecond)

	// Then: another update-driven schedule inside the cooldown is ignored
	manager.schedule(target, false)
	require.Eventually(t, func() bool { return calls.Load() == 1 }, 300*time.Millisecond, 10*time.Millisecond)

	// But an immediate request (record creation) bypasses the cooldown
	manager.schedule(target, true)
	require.Eventually(t, func() bool { return calls.Load() == 2 }, time.Second, 10*time.Millisecond)
}

func TestAgentDeploymentManager_wakeInterruptsBackoffSleep(t *testing.T) {
	// Given: a failing deploy with a long backoff
	var calls atomic.Int32
	manager := newAgentDeploymentManagerForTest(func(ctx context.Context, target agentDeploymentTarget) error {
		if calls.Add(1) >= 2 {
			return nil
		}
		return context.DeadlineExceeded
	})
	manager.retryDelays = []time.Duration{10 * time.Minute}
	t.Cleanup(manager.stop)
	target := agentDeploymentTarget{id: "system-1", host: "gpu-1", port: 45876, sshConfig: "/tmp/ssh-config"}

	// When: the first attempt fails and an immediate schedule wakes the run
	manager.schedule(target, false)
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, 10*time.Millisecond)
	manager.schedule(target, true)

	// Then: the retry happens now instead of after the ten minute backoff
	require.Eventually(t, func() bool { return calls.Load() == 2 }, time.Second, 10*time.Millisecond)
}

func TestAgentDeploymentManager_keepsRetryingAtSlowestCadence(t *testing.T) {
	// Given: a deploy that always fails with short retry delays
	var calls atomic.Int32
	manager := newAgentDeploymentManagerForTest(func(ctx context.Context, target agentDeploymentTarget) error {
		calls.Add(1)
		return context.DeadlineExceeded
	})
	manager.retryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(manager.stop)
	target := agentDeploymentTarget{id: "system-1", host: "gpu-1", port: 45876, sshConfig: "/tmp/ssh-config"}

	// When
	manager.schedule(target, false)

	// Then: attempts continue past the configured delays instead of giving up
	require.Eventually(t, func() bool { return calls.Load() >= 5 }, 5*time.Second, 10*time.Millisecond)
}

func TestAgentDeploymentManager_retryDelay(t *testing.T) {
	manager := newAgentDeploymentManagerForTest(nil)

	// no delays configured: single attempt only
	delay, ok := manager.retryDelay(0)
	require.False(t, ok)
	require.Zero(t, delay)

	manager.retryDelays = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute}

	delay, ok = manager.retryDelay(0)
	require.True(t, ok)
	require.Equal(t, time.Minute, delay)

	delay, ok = manager.retryDelay(2)
	require.True(t, ok)
	require.Equal(t, 30*time.Minute, delay)

	// attempts beyond the schedule keep the slowest cadence forever
	for attempt := 3; attempt < 100; attempt++ {
		delay, ok = manager.retryDelay(attempt)
		require.True(t, ok)
		require.Equal(t, 30*time.Minute, delay)
	}
}
