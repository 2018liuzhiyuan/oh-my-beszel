package hub

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAgentPlatform_supportsLinuxAMD64WithoutSystemd(t *testing.T) {
	// Given
	probeOutput := "Linux\nx86_64\nnone\nglibc\n"

	// When
	platform, err := parseAgentPlatform(probeOutput)

	// Then
	require.NoError(t, err)
	require.Equal(t, agentPlatform{os: "linux", arch: "amd64", init: agentInitNohup, libc: "glibc"}, platform)
}

func TestParseAgentPlatform_rejectsUnsupportedOperatingSystem(t *testing.T) {
	// Given
	probeOutput := "Windows_NT\nx86_64\nnone\nunknown\n"

	// When
	_, err := parseAgentPlatform(probeOutput)

	// Then
	require.ErrorIs(t, err, errAgentPlatformUnsupported)
}

func TestParseAgentProbe(t *testing.T) {
	// Given
	probeOutput := "Linux\nx86_64\nsystemd\nglibc\n0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\nhealthy\nlistening\nauthorized\nmanaged\n"

	// When
	probe, err := parseAgentProbe(probeOutput)

	// Then
	require.NoError(t, err)
	require.Equal(t, agentPlatform{os: "linux", arch: "amd64", init: agentInitSystemd, libc: "glibc"}, probe.platform)
	require.Equal(t, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", probe.installedSha256)
	require.True(t, probe.healthy)
	require.True(t, probe.listening)
	require.True(t, probe.authorized)
	require.True(t, probe.managed)
}

func TestAgentProbe_matchesOnlyManagedAgent(t *testing.T) {
	probe := agentProbe{
		installedSha256: "checksum",
		healthy:         true,
		listening:       true,
		authorized:      true,
		managed:         true,
	}
	require.True(t, probe.matchesManagedAgent("checksum"))

	probe.managed = false
	require.False(t, probe.matchesManagedAgent("checksum"))
}

func TestParseAgentProbe_defaultsWithoutAgent(t *testing.T) {
	// Given
	probeOutput := "Linux\naarch64\nnone\nunknown\nnone\nunhealthy\n"

	// When
	probe, err := parseAgentProbe(probeOutput)

	// Then
	require.NoError(t, err)
	require.Equal(t, agentPlatform{os: "linux", arch: "arm64", init: agentInitNohup, libc: "unknown"}, probe.platform)
	require.Equal(t, "none", probe.installedSha256)
	require.False(t, probe.healthy)
}

func TestParseAgentProbe_toleratesMissingTrailingLines(t *testing.T) {
	// Given: a BusyBox-like host without sha256sum still reports the platform
	probeOutput := "Linux\nx86_64\nnone\nunknown\n"

	// When
	probe, err := parseAgentProbe(probeOutput)

	// Then
	require.NoError(t, err)
	require.Equal(t, "amd64", probe.platform.arch)
	require.Equal(t, "none", probe.installedSha256)
	require.False(t, probe.healthy)
}

func TestParseAgentProbe_rejectsShortOutput(t *testing.T) {
	_, err := parseAgentProbe("Linux\nx86_64\n")
	require.Error(t, err)
}

func TestParseAgentProbe_reportsListenerState(t *testing.T) {
	// Given: a running agent
	probeOutput := "Linux\nx86_64\nnone\nglibc\n0123\nhealthy\nlistening\n"

	// When
	probe, err := parseAgentProbe(probeOutput)

	// Then
	require.NoError(t, err)
	require.True(t, probe.healthy)
	require.True(t, probe.listening)
}

func TestParseAgentProbe_reportsHubAuthorization(t *testing.T) {
	// Given: the current Hub key is present in the shared key file
	probeOutput := "Linux\nx86_64\nnone\nglibc\n0123\nhealthy\nlistening\nauthorized\n"

	// When
	probe, err := parseAgentProbe(probeOutput)

	// Then
	require.NoError(t, err)
	require.True(t, probe.authorized)
}

func TestParseAgentProbe_detectsDeadAgentDespiteLyingHealthCheck(t *testing.T) {
	// Given: agents up to 0.18.8 touch the health file at process start, so
	// their health subcommand reports healthy even when the agent died
	probeOutput := "Linux\nx86_64\nnone\nglibc\n0123\nhealthy\nnotlistening\n"

	// When
	probe, err := parseAgentProbe(probeOutput)

	// Then: the listener check exposes the dead agent so deploy reinstalls it
	require.NoError(t, err)
	require.True(t, probe.healthy)
	require.False(t, probe.listening)
}

func TestParseAgentProbe_defaultsListeningWithoutTrailingLine(t *testing.T) {
	// Given: six-line output from an older probe without the listener line
	probeOutput := "Linux\nx86_64\nnone\nglibc\n0123\nhealthy\n"

	// When
	probe, err := parseAgentProbe(probeOutput)

	// Then
	require.NoError(t, err)
	require.True(t, probe.healthy)
	require.False(t, probe.listening)
}

func TestAgentProbeCommand_checksAgentListener(t *testing.T) {
	// When
	command := agentProbeCommand(45876, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKey")

	// Then: the listener check tries bash's /dev/tcp first and falls back to nc
	require.Contains(t, command, `/dev/tcp/127.0.0.1/$PORT`)
	require.Contains(t, command, `nc -z 127.0.0.1 "$PORT"`)
	require.Contains(t, command, `echo listening`)
	require.Contains(t, command, `echo notlistening`)
	require.Contains(t, command, `BIN="$BASE/bin/beszel-agent"`)
	require.Contains(t, command, `KEY_FILE="$BASE/config/hub_keys"`)
	require.Contains(t, command, `echo authorized`)
	require.Contains(t, command, `echo unauthorized`)
	require.Contains(t, command, `ENV_FILE="$BASE/config/env"`)
	require.Contains(t, command, `MANAGED=unmanaged`)
	require.Contains(t, command, `MANAGED=managed`)
	require.Contains(t, command, `echo "$MANAGED"`)
}

func TestParseAgentInstallPort(t *testing.T) {
	port, err := parseAgentInstallPort("agent_port=45877\n")
	require.NoError(t, err)
	require.Equal(t, uint16(45877), port)

	_, err = parseAgentInstallPort("agent_port=70000\n")
	require.Error(t, err)

	_, err = parseAgentInstallPort("")
	require.Error(t, err)
}

func TestAgentInstallCommand_keepsRecordValuesOutOfShellSource(t *testing.T) {
	// Given
	target := agentDeploymentTarget{
		id:        "system-1",
		host:      "gpu-1",
		port:      45876,
		publicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKey",
		sshConfig: "/tmp/ssh config",
	}
	stageName := "beszel-agent-0123456789abcdef"
	checksum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	// When
	command := agentInstallCommand(target, stageName, checksum, 45876, 45976)

	// Then: upload and install share one connection, values are base64 encoded
	require.Contains(t, command, "umask 077; cat > /tmp/"+stageName+" && ")
	require.Contains(t, command, "printf %s "+base64.StdEncoding.EncodeToString([]byte(agentInstallScript()))+" | base64 -d | sh -s -- ")
	require.Contains(t, command, "45876 "+base64.StdEncoding.EncodeToString([]byte(target.publicKey))+" "+stageName+" "+checksum+" 45876 45976")
	require.Contains(t, command, `test -x "$HOME/oh-my-beszel/bin/run-agent"`)
	require.NotContains(t, command, target.publicKey)
}

func TestAgentInstallScript_usesSingleUserOwnedInstallRoot(t *testing.T) {
	// When
	script := agentInstallScript()

	// Then
	require.Contains(t, script, `INSTALL_ROOT="${HOME:?}/oh-my-beszel"`)
	require.Contains(t, script, `BIN="$BASE/bin/beszel-agent"`)
	require.Contains(t, script, `ENV_FILE="$BASE/config/env"`)
	require.Contains(t, script, `KEY_FILE="$BASE/config/hub_keys"`)
	require.Contains(t, script, `DATA_DIR="$BASE/data"`)
	require.Contains(t, script, `LOG_DIR="$BASE/logs"`)
	require.Contains(t, script, `KEY_FILE="$BESZEL_INSTANCE_BASE/config/hub_keys"`)
	require.Contains(t, script, `DATA_DIR="$BESZEL_INSTANCE_BASE/data"`)
	require.NotContains(t, script, "/opt/beszel-agent")
	require.NotContains(t, script, "sudo")
}

func TestAgentInstallScript_supportsUserSystemdAndDetachedFallback(t *testing.T) {
	// When
	script := agentInstallScript()

	// Then
	require.Contains(t, script, "systemctl --user")
	require.Contains(t, script, "WantedBy=default.target")
	require.Contains(t, script, `StandardOutput=append:%h/oh-my-beszel/logs/agent.log`)
	require.Contains(t, script, "setsid")
	require.Contains(t, script, "nohup")
	require.Contains(t, script, "sha256sum")
	require.Contains(t, script, "systemctl --user enable oh-my-beszel-agent.service")
	require.Contains(t, script, "systemctl --user restart oh-my-beszel-agent.service")
}

func TestAgentInstallScript_handlesPortsOwnedByForeignProcesses(t *testing.T) {
	// When
	script := agentInstallScript()

	// Then: stop this account's managed instances before deciding whether a
	// listener belongs to the freshly installed Agent.
	stopService := "systemctl --user stop oh-my-beszel-agent.service"
	portCheck := `if port_listening; then`
	require.Contains(t, script, stopService)
	require.Contains(t, script, portCheck)
	require.Less(t, strings.Index(script, stopService), strings.Index(script, portCheck))
	require.Contains(t, script, `PORT=$((PORT + 1))`)
	require.Contains(t, script, "BESZEL_AGENT_PORT_RANGE_EXHAUSTED")
	require.Contains(t, script, "exit 24")
}

func TestAgentInstallScript_selectsAndReportsNextFreePort(t *testing.T) {
	// When
	script := agentInstallScript()

	// Then: after stopping this account's Agent, a foreign listener causes a
	// bounded scan and the selected port is returned to the Hub for persistence.
	require.Contains(t, script, `while [ "$PORT" -le "$RANGE_END" ]; do`)
	require.Contains(t, script, `PORT=$((PORT + 1))`)
	require.Contains(t, script, `echo "BESZEL_AGENT_PORT_RANGE_EXHAUSTED: no free agent port for $REQUESTED_PORT in $RANGE_START-$RANGE_END"`)
	require.Contains(t, script, `printf 'agent_port=%s\n' "$PORT"`)
}

func TestConfiguredAgentPortRange(t *testing.T) {
	t.Setenv("BESZEL_HUB_AGENT_PORT_RANGE", "45876-45976")
	start, end, err := configuredAgentPortRange(45876)
	require.NoError(t, err)
	require.Equal(t, uint16(45876), start)
	require.Equal(t, uint16(45976), end)

	t.Setenv("BESZEL_HUB_AGENT_PORT_RANGE", "45976-45876")
	_, _, err = configuredAgentPortRange(45876)
	require.ErrorIs(t, err, errAgentPortRange)
	require.Contains(t, err.Error(), "无效")
	require.Contains(t, err.Error(), "Invalid portRanges.agent.start/end")
	require.Contains(t, err.Error(), "修改后重启 Hub")
	require.Contains(t, err.Error(), "Correct it and restart Hub")
}

func TestAgentInstallScript_appendsHubKeysWithoutDuplicates(t *testing.T) {
	// When
	script := agentInstallScript()

	// Then
	require.Contains(t, script, `grep -Fqx "$KEY" "$KEY_FILE"`)
	require.Contains(t, script, `printf '%s\n' "$KEY" >> "$KEY_FILE"`)
}
