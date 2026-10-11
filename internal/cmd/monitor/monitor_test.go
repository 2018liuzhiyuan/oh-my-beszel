//go:build windows

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestIsReadyRequiresHubHealthResponse(t *testing.T) {
	for _, body := range []string{`<html>another service</html>`, `{"code":503}`, `{"code":200}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/health" {
					t.Errorf("unexpected probe path: %s", r.URL.Path)
				}
				w.Write([]byte(body))
			}))
			defer server.Close()
			if got, want := isReady(server.URL), body == `{"code":200}`; got != want {
				t.Fatalf("ready = %v, want %v", got, want)
			}
		})
	}
}

func TestSaveHubPortPreservesOtherConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	initial := `{"port":8090,"portRanges":{"hub":{"start":8090,"end":8190}},"custom":{"value":"preserved"}}`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveHubPort(path, 8091); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["port"]) != "8091" || !strings.Contains(string(got["custom"]), "preserved") || !strings.Contains(string(got["portRanges"]), "8190") {
		t.Fatal("updating the active port lost existing settings")
	}
}

func TestSaveHubPortRetriesWhileConfigurationIsBeingRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"port":8090}`), 0o600); err != nil {
		t.Fatal(err)
	}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := windows.CreateFile(pathPtr, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	time.AfterFunc(50*time.Millisecond, func() { closed <- windows.CloseHandle(reader) })
	if err := saveHubPort(path, 8091); err != nil {
		t.Error(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}

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
		Host:       "127.0.0.1",
		Port:       8091,
		PortRanges: PortRanges{Agent: &PortRange{Start: 45876, End: 45976}},
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
	if env["BESZEL_HUB_AGENT_PORT_RANGE"] != "45876-45976" {
		t.Fatalf("agent port range was not propagated: %q", env["BESZEL_HUB_AGENT_PORT_RANGE"])
	}
}

func TestSelectHubPortUsesConfiguredFallbackAndReportsExhaustion(t *testing.T) {
	preferred, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer preferred.Close()
	fallback, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fallbackPort := fallback.Addr().(*net.TCPAddr).Port
	fallback.Close()
	cfg := &Config{Host: "127.0.0.1", Port: preferred.Addr().(*net.TCPAddr).Port, PortRanges: PortRanges{Hub: &PortRange{Start: fallbackPort, End: fallbackPort}}}
	selected, err := selectHubPort(cfg)
	if err != nil || selected != fallbackPort {
		t.Fatalf("fallback port = %d, error = %v", selected, err)
	}
	occupiedFallback, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(fallbackPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer occupiedFallback.Close()
	_, err = selectHubPort(cfg)
	if err == nil || !strings.Contains(err.Error(), "portRanges.hub.start/end") {
		t.Fatalf("expected configuration guidance, got %v", err)
	}
	for _, text := range []string{"均不可用", "are unavailable", "扩大范围", "Expand the range", "重新启动 Hub", "save and restart Hub"} {
		if !strings.Contains(err.Error(), text) {
			t.Fatalf("missing bilingual guidance %q: %v", text, err)
		}
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
