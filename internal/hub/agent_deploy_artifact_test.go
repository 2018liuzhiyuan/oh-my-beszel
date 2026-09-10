package hub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadAgentArtifact_readsMatchingPackagedBinary(t *testing.T) {
	// Given
	dir := t.TempDir()
	t.Setenv("BESZEL_AGENT_DEPLOY_DIR", dir)
	data := []byte("linux-agent-binary")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "beszel-agent_linux_amd64"), data, 0o600))
	digest := sha256.Sum256(data)

	// When
	artifact, err := loadAgentArtifact(context.Background(), "", agentPlatform{os: "linux", arch: "amd64"})

	// Then
	require.NoError(t, err)
	require.Equal(t, data, artifact.data)
	require.Equal(t, hex.EncodeToString(digest[:]), artifact.sha256)
}
