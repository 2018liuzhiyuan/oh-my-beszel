package systems

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsDirectHost(t *testing.T) {
	direct := []string{"", "localhost", "LOCALHOST", "127.0.0.1", "192.0.2.115", "::1", "0:0:0:0:0:0:0:1", "/tmp/sock"}
	for _, h := range direct {
		if !isDirectHost(h, "") {
			t.Errorf("expected %q to be direct", h)
		}
	}
	aliases := []string{"gpu-node-a", "render-node-b", "some-host.example.com"}
	for _, h := range aliases {
		if isDirectHost(h, "") {
			t.Errorf("expected %q to go through ssh", h)
		}
	}
}

func TestIsDirectHost_routes_configured_IP_through_SSH(t *testing.T) {
	// Given
	configPath := filepath.Join(t.TempDir(), "ssh", "config")

	// When
	direct := isDirectHost("198.51.100.17", configPath)

	// Then
	require.False(t, direct)
}

func TestSSHDialArgs_uses_selected_config_before_the_host(t *testing.T) {
	// Given
	configPath := filepath.Join(t.TempDir(), "ssh", "config")

	// When
	args := sshDialArgs("127.0.0.1:45876", "gpu-box", configPath)

	// Then
	require.Equal(t, []string{
		"-F", configPath,
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=8",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-W", "127.0.0.1:45876",
		"gpu-box",
	}, args)
}
