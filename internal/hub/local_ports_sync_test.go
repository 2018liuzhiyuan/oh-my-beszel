//go:build testing

package hub

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/henrygd/beszel/internal/migrations"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func TestSyncLocalPortAssignments(t *testing.T) {
	type systemConfig struct {
		host, sshConfig, status string
		updated                 bool
	}
	for _, test := range []struct {
		name    string
		active  bool
		systems []systemConfig
	}{
		{"ignores same-name remote and SSH systems", true, []systemConfig{
			{"192.0.2.10", "", "paused", false},
			{"127.0.0.1", "ssh-config", "paused", false},
			{"127.0.0.1", "", "up", true},
		}},
		{"preserves paused status", true, []systemConfig{{"127.0.0.1", "", "paused", true}}},
		{"skips ambiguous local systems", true, []systemConfig{
			{"127.0.0.1", "", "paused", false},
			{"127.0.0.1", "", "paused", false},
		}},
		{"ignores inactive assignments", false, []systemConfig{{"127.0.0.1", "", "paused", false}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, err := pbTests.NewTestApp(t.TempDir())
			require.NoError(t, err)
			defer app.Cleanup()
			collection, err := app.FindCollectionByNameOrId("systems")
			require.NoError(t, err)
			ids := make([]string, 0, len(test.systems))
			for _, config := range test.systems {
				record := core.NewRecord(collection)
				record.Set("name", "shared-name")
				record.Set("host", config.host)
				record.Set("ssh_config", config.sshConfig)
				record.Set("port", "47000")
				record.Set("status", config.status)
				require.NoError(t, app.SaveNoValidate(record))
				ids = append(ids, record.Id)
			}
			directory := filepath.Join(app.DataDir(), "local-ports")
			require.NoError(t, os.MkdirAll(directory, 0o700))
			data, err := json.Marshal(localPortAssignment{Name: "shared-name", Port: 47100, Active: test.active})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "shared-name.json"), data, 0o600))
			syncLocalPortAssignments(app)
			for index, config := range test.systems {
				record, err := app.FindRecordById("systems", ids[index])
				require.NoError(t, err)
				port, status := 47000, config.status
				if config.updated {
					port = 47100
					if status != "paused" {
						status = "pending"
					}
				}
				require.Equal(t, port, record.GetInt("port"))
				require.Equal(t, status, record.GetString("status"))
			}
		})
	}
}

func TestSyncLocalPortAssignmentsContinuesAfterInvalidFiles(t *testing.T) {
	app, err := pbTests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()
	collection, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	record := core.NewRecord(collection)
	record.Set("name", "z-valid")
	record.Set("host", "127.0.0.1")
	record.Set("port", "47000")
	record.Set("status", "paused")
	require.NoError(t, app.SaveNoValidate(record))

	syncLocalPortAssignments(app)
	directory := filepath.Join(app.DataDir(), "local-ports")
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "a-directory.json"), 0o700))
	for name, data := range map[string]string{
		"a-broken.json":     `{"name":`,
		"a-mismatched.json": `{"name":"z-valid","port":47199,"active":true}`,
		"a-oversized.json":  strings.Repeat(" ", 1025),
		"a-ignored.tmp":     `{"name":"z-valid","port":47199,"active":true}`,
		"z-valid.json":      `{"name":"z-valid","port":47100,"active":true}`,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(data), 0o600))
	}
	syncLocalPortAssignments(app)
	record, err = app.FindRecordById("systems", record.Id)
	require.NoError(t, err)
	require.Equal(t, "47100", record.GetString("port"))
	require.Equal(t, "paused", record.GetString("status"))
}

func TestSyncLocalPortAssignmentsSaveFailureAndRecovery(t *testing.T) {
	app, err := pbTests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()
	collection, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	directory := filepath.Join(app.DataDir(), "local-ports")
	require.NoError(t, os.MkdirAll(directory, 0o700))
	ids := make(map[string]string)
	for _, name := range []string{"a-failed", "z-valid"} {
		record := core.NewRecord(collection)
		record.Set("name", name)
		record.Set("host", "127.0.0.1")
		record.Set("port", "47000")
		record.Set("status", "paused")
		require.NoError(t, app.SaveNoValidate(record))
		ids[name] = record.Id
		data, err := json.Marshal(localPortAssignment{Name: name, Port: 47100, Active: true})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(directory, name+".json"), data, 0o600))
	}
	failWrites := true
	writes := make(map[string]int)
	app.OnRecordUpdate("systems").BindFunc(func(event *core.RecordEvent) error {
		name := event.Record.GetString("name")
		writes[name]++
		if name == "a-failed" && failWrites {
			return errors.New("simulated system save failure")
		}
		return event.Next()
	})

	syncLocalPortAssignments(app)
	failed, err := app.FindRecordById("systems", ids["a-failed"])
	require.NoError(t, err)
	require.Equal(t, "47000", failed.GetString("port"))
	valid, err := app.FindRecordById("systems", ids["z-valid"])
	require.NoError(t, err)
	require.Equal(t, "47100", valid.GetString("port"))

	failWrites = false
	syncLocalPortAssignments(app)
	failed, err = app.FindRecordById("systems", ids["a-failed"])
	require.NoError(t, err)
	require.Equal(t, "47100", failed.GetString("port"))
	require.Equal(t, "paused", failed.GetString("status"))
	syncLocalPortAssignments(app)
	require.Equal(t, map[string]int{"a-failed": 2, "z-valid": 1}, writes, "unchanged assignments should not save records again")
}
