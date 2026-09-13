package systems

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// errSSHClientMissing is returned when no ssh client binary can be located.
// The message is user-facing: it explains the remediation for the most common
// cause (the Windows optional feature not installed / not on PATH).
var errSSHClientMissing = errors.New(
	"ssh client not found: install the Windows \"OpenSSH Client\" optional feature (Settings - Apps - Optional features) or place ssh.exe on PATH; SSH config connections and agent deployment require it",
)

// resolveSSHClient caches the lookup result; a missing client should not be
// re-searched on every poll, and the lookup itself is cheap to keep.
var resolveSSHClient = sync.OnceValues(func() (string, error) {
	if path, err := exec.LookPath("ssh"); err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = `C:\Windows`
		}
		candidates := []string{
			filepath.Join(systemRoot, "System32", "OpenSSH", "ssh.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "OpenSSH", "ssh.exe"),
		}
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				return candidate, nil
			}
		}
	}
	return "", errSSHClientMissing
})

// ResolveSSHClient returns the path of the OpenSSH client binary. PATH is
// preferred; on Windows it falls back to the standard optional-feature and
// winget install locations, which scheduled-task and minimal-shell contexts
// may not have on PATH.
func ResolveSSHClient() (string, error) {
	return resolveSSHClient()
}
