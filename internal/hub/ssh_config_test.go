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

Host gpu-node-a GPU-NODE-B
    HostName 192.0.2.79 # documentation address
Host render-node
    HostName "203.0.113.111"
Host no-hostname
    User root
Host templated
    HostName node-%h
Host gpu-node-a
    HostName ignored.example.com
Host=quoted
    HostName=quoted.example.com
`

	hosts, err := parseSSHHosts(strings.NewReader(config))
	require.NoError(t, err)
	require.Equal(t, []sshHost{
		{Name: "gpu-node-a", HostName: "192.0.2.79"},
		{Name: "GPU-NODE-B", HostName: "192.0.2.79"},
		{Name: "no-hostname", HostName: "no-hostname"},
		{Name: "quoted", HostName: "quoted.example.com"},
		{Name: "render-node", HostName: "203.0.113.111"},
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

func TestExpandSSHConfigPath(t *testing.T) {
	home := filepath.Join("home", "operator")
	require.Equal(t, filepath.Join(home, ".ssh", "config"), expandSSHConfigPath("~/.ssh/config", home))
	require.Equal(t, filepath.Join(home, "custom", "config"), expandSSHConfigPath("~/custom/config", home))
	require.Equal(t, filepath.Clean("relative/config"), expandSSHConfigPath("relative/config", home))
}

func writeSSHConfig(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestReadSSHHostsInclude(t *testing.T) {
	dir := t.TempDir()
	writeSSHConfig(t, filepath.Join(dir, "config"), `
Host main-node
    HostName 10.0.0.1
Include config.d/*.conf
Host after-include
`)
	writeSSHConfig(t, filepath.Join(dir, "config.d", "10-a.conf"), `
Host inc-a
    HostName 10.0.0.2
`)
	writeSSHConfig(t, filepath.Join(dir, "config.d", "20-b.conf"), `
Host inc-b
    HostName 10.0.0.3
Host main-node
    HostName ignored-later.example.com
`)

	hosts, err := readSSHHosts(filepath.Join(dir, "config"))
	require.NoError(t, err)
	require.Equal(t, []sshHost{
		{Name: "after-include", HostName: "after-include"},
		{Name: "inc-a", HostName: "10.0.0.2"},
		{Name: "inc-b", HostName: "10.0.0.3"},
		{Name: "main-node", HostName: "10.0.0.1"},
	}, hosts)
}

func TestReadSSHHostsIncludeMissingAndCyclic(t *testing.T) {
	dir := t.TempDir()
	writeSSHConfig(t, filepath.Join(dir, "config"), `
Include config.d/*.conf
Include absolute/missing/*.conf
Host loop-a
    HostName 10.0.0.4
`)
	writeSSHConfig(t, filepath.Join(dir, "config.d", "loop.conf"), `
Include ../config
Host loop-b
`)

	hosts, err := readSSHHosts(filepath.Join(dir, "config"))
	require.NoError(t, err)
	require.Equal(t, []sshHost{
		{Name: "loop-a", HostName: "10.0.0.4"},
		{Name: "loop-b", HostName: "loop-b"},
	}, hosts)
}

func TestReadSSHHostsIncludeHomePath(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home", "operator")
	writeSSHConfig(t, filepath.Join(dir, "config"), "Include ~/.ssh/extra.conf\n")
	writeSSHConfig(t, filepath.Join(home, ".ssh", "extra.conf"), "Host home-host\n    HostName 10.0.0.5\n")

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	hosts, err := readSSHHosts(filepath.Join(dir, "config"))
	require.NoError(t, err)
	require.Equal(t, []sshHost{{Name: "home-host", HostName: "10.0.0.5"}}, hosts)
}

func TestParseSSHHostsBOM(t *testing.T) {
	// the string starts with a UTF-8 BOM, as Windows editors write it
	config := string(rune(0xfeff)) + "Host bom-node\n    HostName 10.0.0.6\n"

	hosts, err := parseSSHHosts(strings.NewReader(config))
	require.NoError(t, err)
	require.Equal(t, []sshHost{{Name: "bom-node", HostName: "10.0.0.6"}}, hosts)
}
