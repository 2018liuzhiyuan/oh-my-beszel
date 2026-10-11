//go:build testing

package hub

import (
	"context"
	"errors"
	"fmt"
	"testing"

	_ "github.com/henrygd/beszel/internal/migrations"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func TestAgentDeploymentPortRangeWarningAndRecovery(t *testing.T) {
	app, err := pbTests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()
	collection, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	record := core.NewRecord(collection)
	record.Set("name", "range-test")
	record.Set("host", "range-test")
	record.Set("port", "45876")
	record.Set("status", "down")
	record.Set("status_info", "previous connection failure")
	require.NoError(t, app.SaveNoValidate(record))
	target := agentDeploymentTarget{id: record.Id, host: "range-test", port: 45876}
	manager := newAgentDeploymentManagerForTest(func(context.Context, agentDeploymentTarget) error {
		return fmt.Errorf("%w: 请修改 config.json 的 portRanges.agent.start/end", errAgentPortRange)
	})
	manager.app = app
	t.Cleanup(manager.stop)

	manager.schedule(target, true)
	manager.wg.Wait()
	record, err = app.FindRecordById("systems", record.Id)
	require.NoError(t, err)
	require.Equal(t, "down", record.GetString("status"))
	require.Contains(t, record.GetString("status_info"), "portRanges.agent.start/end")

	manager.deploy = func(context.Context, agentDeploymentTarget) error { return errors.New("temporary SSH failure") }
	manager.schedule(target, true)
	manager.wg.Wait()
	record, err = app.FindRecordById("systems", record.Id)
	require.NoError(t, err)
	require.Contains(t, record.GetString("status_info"), "portRanges.agent.start/end")

	manager.deploy = func(context.Context, agentDeploymentTarget) error { return nil }
	manager.schedule(target, true)
	manager.wg.Wait()
	record, err = app.FindRecordById("systems", record.Id)
	require.NoError(t, err)
	require.Equal(t, "pending", record.GetString("status"))
	require.Empty(t, record.GetString("status_info"))

	record.Set("status", "paused")
	require.NoError(t, app.SaveNoValidate(record))
	manager.markPortRangeDown(record.Id, errAgentPortRange)
	manager.markPending(record.Id)
	record, err = app.FindRecordById("systems", record.Id)
	require.NoError(t, err)
	require.Equal(t, "paused", record.GetString("status"))
	require.Empty(t, record.GetString("status_info"))
}
