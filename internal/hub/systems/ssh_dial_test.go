package systems

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resetDialCache() {
	sshDialMu.Lock()
	sshDialEntries = nil
	sshDialExactMap = nil
	sshDialLoaded = false
	sshDialPath = ""
	sshDialModTime = time.Time{}
	sshDialMu.Unlock()
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSH_CONFIG_PATH", path)
	return path
}

func TestParseSSHConfigKeywords(t *testing.T) {
	resetDialCache()
	writeTestConfig(t, `
Host h100-jump
    HostName 100.73.31.115
    User lzy
    IdentityFile ~/.ssh/h100_lzy
    IdentitiesOnly yes

Host=meta-006
    HostName=10.240.16.9
    ProxyJump=beszel-h101-jump
    Port 45876
`)
	loadSSHDialConfig()

	e, ok := lookupSSHHost("h100-jump")
	if !ok {
		t.Fatal("h100-jump not found")
	}
	if e.HostName != "100.73.31.115" || e.User != "lzy" || e.Port != "" {
		t.Fatalf("h100-jump fields wrong: %+v", e)
	}
	if e.IdentityFile == "~/.ssh/h100_lzy" {
		t.Fatal("IdentityFile not expanded")
	}

	m, ok := lookupSSHHost("meta-006")
	if !ok {
		t.Fatal("meta-006 not found")
	}
	if m.HostName != "10.240.16.9" || m.Port != "45876" || m.ProxyJump != "beszel-h101-jump" {
		t.Fatalf("meta-006 fields wrong: %+v", m)
	}

	if _, ok := lookupSSHHost("unknown-host"); ok {
		t.Fatal("unknown host matched")
	}
}

func TestLookupPatternMatch(t *testing.T) {
	resetDialCache()
	writeTestConfig(t, `
Host gpu-*
    HostName 10.0.0.1

Host *
    ServerAliveInterval 30
`)
	loadSSHDialConfig()
	if _, ok := lookupSSHHost("gpu-9"); !ok {
		t.Fatal("pattern gpu-* should match gpu-9")
	}
	if _, ok := lookupSSHHost("other"); ok {
		t.Fatal("bare Host * block must not match")
	}
}

func TestBuildJumpChainOrder(t *testing.T) {
	resetDialCache()
	writeTestConfig(t, `
Host target
    HostName 10.240.16.9
    ProxyJump mid-jump

Host mid-jump
    HostName 10.1.1.1
    Port 2222
    ProxyJump first-jump

Host first-jump
    HostName 10.0.0.254
`)
	loadSSHDialConfig()
	entry, ok := lookupSSHHost("target")
	if !ok {
		t.Fatal("target not found")
	}
	hops, err := buildJumpChain(entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(hops) != 2 {
		t.Fatalf("expected 2 hops (first-jump, mid-jump), got %d: %+v", len(hops), hops)
	}
	if hops[0].addr != "10.0.0.254:22" {
		t.Fatalf("nearest hop should be first-jump, got %s", hops[0].addr)
	}
	if hops[1].addr != "10.1.1.1:2222" {
		t.Fatalf("second hop should be mid-jump with port, got %s", hops[1].addr)
	}
}

func TestDialConfigReloadsOnMtimeChange(t *testing.T) {
	resetDialCache()
	path := writeTestConfig(t, "Host alias\n    HostName 10.0.0.1\n")
	loadSSHDialConfig()
	if e, _ := lookupSSHHost("alias"); e.HostName != "10.0.0.1" {
		t.Fatalf("initial load wrong: %+v", e)
	}
	// rewrite with a new IP; mtime must trigger a reload
	time.Sleep(10 * time.Millisecond) // ensure mtime differs
	if err := os.WriteFile(path, []byte("Host alias\n    HostName 10.9.9.9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if e, _ := lookupSSHHost("alias"); e.HostName != "10.9.9.9" {
		t.Fatalf("reload did not pick up new IP: %+v", e)
	}
}
