package hub_test

import (
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

// TestCreateSystemRespectsProvidedStatus guards against the create hook
// stomping caller-provided statuses: API or seeded systems created as paused
// must stay paused instead of being forced into pending and polled down.
func TestCreateSystemRespectsProvidedStatus(t *testing.T) {
	testHub, user := beszelTests.GetHubWithUser(t)
	defer testHub.Cleanup()

	collection, err := testHub.TestApp.FindCollectionByNameOrId("systems")
	require.NoError(t, err)

	pausedRecord := core.NewRecord(collection)
	pausedRecord.Set("name", "created-paused")
	pausedRecord.Set("host", "127.0.0.1")
	pausedRecord.Set("port", "45876")
	pausedRecord.Set("users", []string{user.Id})
	pausedRecord.Set("status", "paused")
	require.NoError(t, testHub.TestApp.Save(pausedRecord))
	fetched, err := testHub.TestApp.FindRecordById("systems", pausedRecord.Id)
	require.NoError(t, err)
	require.Equal(t, "paused", fetched.GetString("status"), "paused status must survive record creation")

	defaultRecord := core.NewRecord(collection)
	defaultRecord.Set("name", "created-default")
	defaultRecord.Set("host", "127.0.0.1")
	defaultRecord.Set("port", "45876")
	defaultRecord.Set("users", []string{user.Id})
	require.NoError(t, testHub.TestApp.Save(defaultRecord))
	fetched, err = testHub.TestApp.FindRecordById("systems", defaultRecord.Id)
	require.NoError(t, err)
	require.Equal(t, "pending", fetched.GetString("status"), "records without a status still default to pending")
}
