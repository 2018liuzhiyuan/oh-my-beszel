package hub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxAgentArtifactSize = 128 << 20

type agentArtifact struct {
	data   []byte
	sha256 string
}

func loadAgentArtifact(ctx context.Context, appDataDir string, platform agentPlatform) (agentArtifact, error) {
	select {
	case <-ctx.Done():
		return agentArtifact{}, ctx.Err()
	default:
	}

	name := fmt.Sprintf("beszel-agent_%s_%s", platform.os, platform.arch)
	dirs := make([]string, 0, 3)
	if configured := strings.TrimSpace(os.Getenv("BESZEL_AGENT_DEPLOY_DIR")); configured != "" {
		dirs = append(dirs, configured)
	}
	if executable, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(executable), "agents"))
	}
	if appDataDir != "" {
		dirs = append(dirs, filepath.Join(appDataDir, "agents"))
	}

	for _, dir := range dirs {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return agentArtifact{}, fmt.Errorf("stat agent artifact: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxAgentArtifactSize {
			return agentArtifact{}, fmt.Errorf("invalid agent artifact %q", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return agentArtifact{}, fmt.Errorf("read agent artifact: %w", err)
		}
		digest := sha256.Sum256(data)
		return agentArtifact{data: data, sha256: hex.EncodeToString(digest[:])}, nil
	}
	return agentArtifact{}, fmt.Errorf("agent artifact %q not found; rebuild the Windows deployment package", name)
}
