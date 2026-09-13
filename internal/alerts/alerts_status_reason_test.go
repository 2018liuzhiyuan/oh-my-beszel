//go:build testing

package alerts_test

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusAlertDownIncludesReason(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)

	synctest.Test(t, func(t *testing.T) {
		defer hub.Cleanup()

		setStatusAlertEmail(t, hub, user.Id, "reason@example.com")

		systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
		require.NoError(t, err)
		system := systems[0]

		_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
			"name":   "Status",
			"system": system.Id,
			"user":   user.Id,
			"min":    1,
		})
		require.NoError(t, err)

		// status alerts only arm on an up -> down transition
		system.Set("status", "up")
		require.NoError(t, hub.SaveNoValidate(system))
		time.Sleep(time.Second)

		system.Set("status", "down")
		system.Set("status_info", "ssh: handshake failed: EOF; ssh: Could not resolve hostname gpu-node-a")
		require.NoError(t, hub.SaveNoValidate(system))

		// min 1 minute elapses and the down alert fires
		time.Sleep(time.Minute + time.Second)

		assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "expected the down alert email")
		message := hub.TestMailer.LastMessage()
		assert.Contains(t, message.Subject, "down")
		assert.True(
			t,
			strings.Contains(message.Text, "Could not resolve hostname gpu-node-a"),
			"alert body should include the stored failure reason, got: %s",
			message.Text,
		)
	})
}
