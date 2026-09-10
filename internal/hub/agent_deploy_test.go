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
	manager.schedule(target)
	<-started
	manager.schedule(target)
	close(release)

	// Then
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("deployment did not finish")
	}
	require.Equal(t, int32(1), calls.Load())
}
