package migrations

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemStatusInfoMigrationAddsField(t *testing.T) {
	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.SetEmail("status-info@example.com")
	user.SetPassword("password123")
	require.NoError(t, app.Save(user))

	systemsCollection, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	system := core.NewRecord(systemsCollection)
	system.Set("name", "Status info system")
	system.Set("host", "127.0.0.1")
	system.Set("port", "45876")
	system.Set("users", []string{user.Id})
	system.Set("status", "down")
	system.Set("status_info", "preserved reason")
	require.NoError(t, app.Save(system))

	// roll the collection back to its pre-migration shape and replay
	systemsCollection.Fields.RemoveByName("status_info")
	require.NoError(t, app.Save(systemsCollection))
	_, err = app.DB().Delete(core.DefaultMigrationsTable, dbx.HashExp{"file": "4_system_status_info.go"}).Execute()
	require.NoError(t, err)

	runner := core.NewMigrationsRunner(app, core.AppMigrations)
	_, err = runner.Up()
	require.NoError(t, err)

	replayed, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	assert.NotNil(t, replayed.Fields.GetByName("status_info"), "status_info field should be re-added by the migration")

	// the replayed field accepts new values again
	reloaded, err := app.FindRecordById("systems", system.Id)
	require.NoError(t, err)
	reloaded.Set("status_info", "fresh reason after replay")
	require.NoError(t, app.Save(reloaded))
	roundTripped, err := app.FindRecordById("systems", system.Id)
	require.NoError(t, err)
	assert.Equal(t, "fresh reason after replay", roundTripped.GetString("status_info"))
}
