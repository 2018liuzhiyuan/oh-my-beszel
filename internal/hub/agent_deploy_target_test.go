package hub

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestTargetFromRecord_acceptsConcreteHostFromSelectedSSHConfig(t *testing.T) {
	// Given
	configPath := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(configPath, []byte("Host gpu-1\n  HostName 192.0.2.10\n"), 0o600))
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sshKey, err := ssh.NewPublicKey(publicKey)
	require.NoError(t, err)
	record := core.NewRecord(&core.Collection{})
	record.Id = "system-1"
	record.Set("host", "gpu-1")
	record.Set("port", "45876")
	record.Set("ssh_config", configPath)
	hubPublicKey := string(ssh.MarshalAuthorizedKey(sshKey))

	// When
	target, err := (&agentDeploymentManager{publicKey: func() string { return hubPublicKey }}).targetFromRecord(record)

	// Then
	require.NoError(t, err)
	require.Equal(t, "gpu-1", target.host)
	require.Equal(t, uint16(45876), target.port)
	require.NotEmpty(t, target.publicKey)
}

func TestTargetFromRecord_rejectsHostMissingFromSelectedSSHConfig(t *testing.T) {
	// Given
	configPath := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(configPath, []byte("Host gpu-1\n  HostName 192.0.2.10\n"), 0o600))
	record := core.NewRecord(&core.Collection{})
	record.Id = "system-2"
	record.Set("host", "unlisted-host")
	record.Set("port", "45876")
	record.Set("ssh_config", configPath)

	// When
	_, err := (&agentDeploymentManager{}).targetFromRecord(record)

	// Then
	require.Error(t, err)
}
