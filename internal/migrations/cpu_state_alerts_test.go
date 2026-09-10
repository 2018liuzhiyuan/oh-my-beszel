package migrations

import (
	"slices"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCPUStateMigrationPreservesExistingConfiguration(t *testing.T) {
	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()
	collection, err := app.FindCollectionByNameOrId("alerts")
	require.NoError(t, err)
	field, ok := collection.Fields.GetByName("name").(*core.SelectField)
	require.True(t, ok)
	field.Values = slices.DeleteFunc(field.Values, func(name string) bool {
		return name == "CPUIOWait" || name == "CPUSteal"
	})
	field.Values = append(field.Values, "LocalCustomAlert")
	originalNames := slices.Clone(field.Values)
	collection.Fields.Add(&core.TextField{Name: "local_note"})
	require.NoError(t, app.Save(collection))
	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.SetEmail("migration@example.com")
	user.SetPassword("password123")
	require.NoError(t, app.Save(user))
	systems, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	system := core.NewRecord(systems)
	system.Set("name", "Existing system")
	system.Set("host", "127.0.0.1")
	system.Set("port", "45876")
	system.Set("users", []string{user.Id})
	system.Set("ssh_config", "existing-config")
	require.NoError(t, app.Save(system))
	alert := core.NewRecord(collection)
	alert.Set("user", user.Id)
	alert.Set("system", system.Id)
	alert.Set("name", "GpuMemoryFree")
	alert.Set("value", 24)
	alert.Set("min", 3)
	alert.Set("triggered", true)
	alert.Set("local_note", "preserve")
	require.NoError(t, app.Save(alert))
	alert, err = app.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	system, err = app.FindRecordById("systems", system.Id)
	require.NoError(t, err)
	_, err = app.DB().Delete(core.DefaultMigrationsTable, dbx.HashExp{"file": "3_cpu_state_alerts.go"}).Execute()
	require.NoError(t, err)

	runner := core.NewMigrationsRunner(app, core.AppMigrations)
	applied, err := runner.Up()
	require.NoError(t, err)
	assert.Equal(t, []string{"3_cpu_state_alerts.go"}, applied)
	updatedCollection, err := app.FindCollectionByNameOrId("alerts")
	require.NoError(t, err)
	updatedField, ok := updatedCollection.Fields.GetByName("name").(*core.SelectField)
	require.True(t, ok)
	assert.Equal(t, append(originalNames, "CPUIOWait", "CPUSteal"), updatedField.Values)
	assert.Equal(t, collection.ListRule, updatedCollection.ListRule)
	assert.Equal(t, collection.CreateRule, updatedCollection.CreateRule)
	assert.Equal(t, collection.UpdateRule, updatedCollection.UpdateRule)
	assert.Equal(t, collection.DeleteRule, updatedCollection.DeleteRule)
	updatedAlert, err := app.FindRecordById("alerts", alert.Id)
	require.NoError(t, err)
	assert.Equal(t, alert.PublicExport(), updatedAlert.PublicExport())
	updatedSystem, err := app.FindRecordById("systems", system.Id)
	require.NoError(t, err)
	assert.Equal(t, system.PublicExport(), updatedSystem.PublicExport())
	applied, err = runner.Up()
	require.NoError(t, err)
	assert.Empty(t, applied)
}
