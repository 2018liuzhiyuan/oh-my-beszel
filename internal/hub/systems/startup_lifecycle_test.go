//go:build testing

package systems

import (
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/henrygd/beszel/internal/migrations"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSystemsForStartupKeepsSSHConfigAndSkipsPaused(t *testing.T) {
	app, err := pbTests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.SetEmail("startup-query@example.com")
	user.SetPassword("password123")
	require.NoError(t, app.Save(user))

	collection, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)

	active := core.NewRecord(collection)
	active.Set("name", "jump system")
	active.Set("host", "jump-alias")
	active.Set("port", "45876")
	active.Set("users", []string{user.Id})
	active.Set("status", "down")
	active.Set("ssh_config", `F:\custom\ssh_config`)
	require.NoError(t, app.Save(active))

	pausedSystem := core.NewRecord(collection)
	pausedSystem.Set("name", "paused system")
	pausedSystem.Set("host", "127.0.0.1")
	pausedSystem.Set("port", "45876")
	pausedSystem.Set("users", []string{user.Id})
	pausedSystem.Set("status", "paused")
	require.NoError(t, app.Save(pausedSystem))

	loaded, err := loadSystemsForStartup(app.DB())
	require.NoError(t, err)
	require.Len(t, loaded, 1, "paused systems are excluded from startup monitoring")
	assert.NotEmpty(t, loaded[0].Id)
	assert.Equal(t, "jump-alias", loaded[0].Host)
	assert.Equal(t, `F:\custom\ssh_config`, loaded[0].SSHConfigPath, "custom SSH config must survive a hub restart")
	assert.Equal(t, "down", loaded[0].Status)
}

func TestResolveSSHClientLocatesBinary(t *testing.T) {
	path, err := ResolveSSHClient()
	require.NoError(t, err)
	base := filepath.Base(path)
	assert.True(t, base == "ssh" || strings.EqualFold(base, "ssh.exe"), "unexpected ssh binary %q", path)
}
