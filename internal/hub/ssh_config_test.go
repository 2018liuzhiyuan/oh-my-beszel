package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSSHHosts(t *testing.T) {
	config := `
# Pattern blocks are settings, not selectable hosts.
Host *
    ServerAliveInterval 30
Host gpu-*
    User worker

Host h102 H101
    HostName 192.0.2.79 # local tailnet address
Host RoboDojo
    HostName "203.0.113.111"
Host no-hostname
    User root
Host templated
    HostName node-%h
Host h102
    HostName ignored.example.com
Host=quoted
    HostName=quoted.example.com
`

	hosts, err := parseSSHHosts(strings.NewReader(config))
	require.NoError(t, err)
	require.Equal(t, []sshHost{
		{Name: "H101", HostName: "192.0.2.79"},
		{Name: "h102", HostName: "192.0.2.79"},
		{Name: "no-hostname", HostName: "no-hostname"},
		{Name: "quoted", HostName: "quoted.example.com"},
		{Name: "RoboDojo", HostName: "203.0.113.111"},
		{Name: "templated", HostName: "node-templated"},
	}, hosts)
}

func TestReadSSHHostsMissingFile(t *testing.T) {
	hosts, err := readSSHHosts(t.TempDir() + "/missing-config")
	require.NoError(t, err)
	require.Empty(t, hosts)
	require.NotNil(t, hosts)
}

func TestReadLocalSSHHostsEnvOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-ssh-config")
	require.NoError(t, os.WriteFile(path, []byte("Host gpu-box\n    HostName 10.0.0.9\n"), 0o600))

	t.Setenv("SSH_CONFIG_PATH", path)
	hosts, err := readLocalSSHHosts()
	require.NoError(t, err)
	require.Equal(t, []sshHost{{Name: "gpu-box", HostName: "10.0.0.9"}}, hosts)
}
