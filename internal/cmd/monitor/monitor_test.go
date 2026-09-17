//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskXMLRunsNativeHub(t *testing.T) {
	xml := taskXML(`C:\Beszel\Monitor.exe`, `C:\Beszel`, `DOMAIN\user`)
	if !strings.Contains(xml, "<Arguments>hub</Arguments>") {
		t.Fatalf("task XML does not start native hub mode: %s", xml)
	}
	if strings.Contains(strings.ToLower(xml), ".ps1") || strings.Contains(strings.ToLower(xml), "powershell") {
		t.Fatalf("task XML contains a PowerShell dependency: %s", xml)
	}
}

func TestRotateHubLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "hub.log")
	file, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxHubLogSize + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rotateHubLog(logPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatalf("rotated log missing: %v", err)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("original log still exists after rotation: %v", err)
	}
}

func TestHubEnvironmentReplacesManagedValues(t *testing.T) {
	t.Setenv("AUTO_LOGIN", "stale@example.com")
	t.Setenv("CHECK_UPDATES", "true")
	checkUpdates := false
	cfg := &Config{
		Host: "127.0.0.1",
		Port: 8091,
		Hub: HubConfig{
			UserEmail:    "admin@example.com",
			UserPassword: "secret",
			CheckUpdates: &checkUpdates,
		},
	}
	env := environmentMap(hubEnvironment(cfg, "127.0.0.1:8091"))
	if env["APP_URL"] != "http://127.0.0.1:8091" {
		t.Fatalf("unexpected APP_URL: %q", env["APP_URL"])
	}
	if env["USER_EMAIL"] != "admin@example.com" || env["USER_PASSWORD"] != "secret" {
		t.Fatalf("hub credentials were not propagated")
	}
	if env["CHECK_UPDATES"] != "false" {
		t.Fatalf("unexpected CHECK_UPDATES: %q", env["CHECK_UPDATES"])
	}
	if _, ok := env["AUTO_LOGIN"]; ok {
		t.Fatalf("stale AUTO_LOGIN was inherited")
	}
}

func environmentMap(entries []string) map[string]string {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[strings.ToUpper(key)] = value
		}
	}
	return values
}
