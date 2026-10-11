//go:build testing

package hub

import (
	"encoding/base64"
	"strings"
	"testing"

	_ "github.com/henrygd/beszel/internal/migrations"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func TestAgentDeploymentIdentitySeparatesMachinesAccountsAndDirectories(t *testing.T) {
	identity := agentDeploymentIdentity{machineFingerprint: "machine", remoteUID: "1000", installDir: "/home/user/oh-my-beszel"}
	require.Equal(t, identity.instanceID(), identity.instanceID())
	other := identity
	other.machineFingerprint = "other-machine"
	require.NotEqual(t, identity.instanceID(), other.instanceID())
	other = identity
	other.remoteUID = "1001"
	require.NotEqual(t, identity.instanceID(), other.instanceID())
	other = identity
	other.installDir = "/home/user/other-install"
	require.NotEqual(t, identity.instanceID(), other.instanceID())
}

func TestParseAgentDeploymentResult(t *testing.T) {
	output := "agent_port=45877\nagent_machine=saved-fingerprint\nagent_uid=0\nagent_dir_b64=" + base64.StdEncoding.EncodeToString([]byte("/home/user name/oh-my-beszel")) + "\n"
	result, err := parseAgentDeploymentResult(output)
	require.NoError(t, err)
	require.Equal(t, uint16(45877), result.port)
	require.Equal(t, "saved-fingerprint", result.machineFingerprint)
	require.Equal(t, "0", result.remoteUID)
	require.Equal(t, "/home/user name/oh-my-beszel", result.installDir)
	for _, invalid := range []string{
		strings.ReplaceAll(output, "agent_uid=0", "agent_uid=-1"),
		strings.ReplaceAll(output, "agent_uid=0", "agent_uid=00"),
		strings.ReplaceAll(output, "saved-fingerprint", ""),
		strings.ReplaceAll(output, base64.StdEncoding.EncodeToString([]byte(result.installDir)), "bad base64"),
		strings.ReplaceAll(output, base64.StdEncoding.EncodeToString([]byte(result.installDir)), base64.StdEncoding.EncodeToString([]byte("relative/path"))),
		strings.ReplaceAll(output, base64.StdEncoding.EncodeToString([]byte(result.installDir)), base64.StdEncoding.EncodeToString([]byte("/home/../other"))),
		"agent_port=45877\n",
	} {
		_, err := parseAgentDeploymentResult(invalid)
		require.Error(t, err, invalid)
	}
}

func TestPersistAgentDeploymentKeepsUserMembership(t *testing.T) {
	app, err := pbTests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	defer app.Cleanup()
	collection, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	record := core.NewRecord(collection)
	record.Set("name", "identity-test")
	record.Set("host", "gpu-alias")
	record.Set("port", "45876")
	record.Set("ssh_config", "/test/config")
	record.Set("users", []string{"existing-user"})
	require.NoError(t, app.SaveNoValidate(record))
	target := agentDeploymentTarget{id: record.Id, host: "gpu-alias", port: 45876, sshConfig: "/test/config"}
	result := agentDeploymentResult{port: 45877, agentDeploymentIdentity: agentDeploymentIdentity{machineFingerprint: "machine", remoteUID: "1000", installDir: "/home/user/oh-my-beszel"}}
	require.NoError(t, persistAgentDeployment(app, target, result))
	reloaded, err := app.FindRecordById("systems", record.Id)
	require.NoError(t, err)
	require.Equal(t, "45877", reloaded.GetString("port"))
	require.Equal(t, result.instanceID(), reloaded.GetString("agent_instance_id"))
	require.Equal(t, "machine", reloaded.GetString("agent_machine_fingerprint"))
	require.Equal(t, "1000", reloaded.GetString("agent_remote_uid"))
	require.Equal(t, result.installDir, reloaded.GetString("agent_install_dir"))
	require.Equal(t, []string{"existing-user"}, reloaded.GetStringSlice("users"))

	reloaded.Set("host", "changed-alias")
	require.NoError(t, app.SaveNoValidate(reloaded))
	require.ErrorContains(t, persistAgentDeployment(app, target, result), "target changed")
}
