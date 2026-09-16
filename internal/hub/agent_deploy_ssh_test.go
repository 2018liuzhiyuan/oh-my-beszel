package hub

import (
	"encoding/base64"
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
	probeOutput := "Linux\nx86_64\nsystemd\nglibc\n0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\nhealthy\n"

	// When
	probe, err := parseAgentProbe(probeOutput)

	// Then
	require.NoError(t, err)
	require.Equal(t, agentPlatform{os: "linux", arch: "amd64", init: agentInitSystemd, libc: "glibc"}, probe.platform)
	require.Equal(t, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", probe.installedSha256)
	require.True(t, probe.healthy)
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
	command := agentProbeCommand(45876)

	// Then: the listener check tries bash's /dev/tcp first and falls back to nc
	require.Contains(t, command, `/dev/tcp/127.0.0.1/45876`)
	require.Contains(t, command, `nc -z 127.0.0.1 45876`)
	require.Contains(t, command, `echo listening`)
	require.Contains(t, command, `echo notlistening`)
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
	command := agentInstallCommand(target, stageName, checksum)

	// Then: upload and install share one connection, values are base64 encoded
	require.Contains(t, command, "umask 077; cat > /tmp/"+stageName+" && ")
	require.Contains(t, command, "printf %s "+base64.StdEncoding.EncodeToString([]byte(agentInstallScript()))+" | base64 -d | sh -s -- ")
	require.Contains(t, command, "45876 "+base64.StdEncoding.EncodeToString([]byte(target.publicKey))+" "+stageName+" "+checksum)
	require.Contains(t, command, "test -x /opt/beszel-agent/beszel-agent")
	require.NotContains(t, command, target.publicKey)
}

func TestAgentInstallScript_supportsSystemdAndDetachedFallback(t *testing.T) {
	// When
	script := agentInstallScript()

	// Then
	require.Contains(t, script, "command -v systemctl")
	require.Contains(t, script, "setsid")
	require.Contains(t, script, "nohup")
	require.Contains(t, script, "sha256sum")
	require.Contains(t, script, "systemctl enable beszel-agent.service")
	require.Contains(t, script, "systemctl restart beszel-agent.service")
}
