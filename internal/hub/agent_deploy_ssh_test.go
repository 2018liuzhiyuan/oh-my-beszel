package hub

import (
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

	// When
	command, args := agentInstallCommand(target, stageName, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	// Then
	require.Equal(t, "sh -s --", command)
	require.NotContains(t, command, target.publicKey)
	require.Equal(t, []string{"45876", "c3NoLWVkMjU1MTkgQUFBQUMzTnphQzFsWkRJMU5URTVBQUFBSVRlc3RLZXk=", stageName, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}, args)
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
