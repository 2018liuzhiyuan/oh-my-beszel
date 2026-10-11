//go:build linux && testing

package hub

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentInstallScriptAutomaticallySelectsNextFreePort(t *testing.T) {
	occupied, requestedPort := listenWithFreeSuccessor(t)
	t.Cleanup(func() { require.NoError(t, occupied.Close()) })

	helperDirectory := t.TempDir()
	helperSource := filepath.Join(helperDirectory, "main.go")
	helperBinary := filepath.Join(helperDirectory, "fake-agent")
	require.NoError(t, os.WriteFile(helperSource, []byte(fakeAgentSource), 0o600))
	build := exec.Command("go", "build", "-o", helperBinary, helperSource)
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, string(buildOutput))

	stageName := fmt.Sprintf("beszel-agent-port-test-%d", os.Getpid())
	stagePath := filepath.Join("/tmp", stageName)
	helperData, err := os.ReadFile(helperBinary)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(stagePath, helperData, 0o700))
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(stagePath)) })
	checksum := sha256.Sum256(helperData)
	key := base64.StdEncoding.EncodeToString([]byte("ssh-ed25519 test-key"))

	home := t.TempDir()
	exhausted := exec.Command(
		"sh", "-s", "--", strconv.Itoa(requestedPort), key, stageName,
		hex.EncodeToString(checksum[:]), strconv.Itoa(requestedPort), strconv.Itoa(requestedPort),
	)
	exhausted.Stdin = strings.NewReader(agentInstallScript())
	exhausted.Env = append(os.Environ(), "HOME="+home, "XDG_RUNTIME_DIR="+filepath.Join(home, "missing-runtime"))
	exhaustedOutput, err := exhausted.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(exhaustedOutput), "BESZEL_AGENT_PORT_RANGE_EXHAUSTED")
	require.NoError(t, os.WriteFile(stagePath, helperData, 0o700))

	install := exec.Command(
		"sh",
		"-s",
		"--",
		strconv.Itoa(requestedPort),
		key,
		stageName,
		hex.EncodeToString(checksum[:]),
		strconv.Itoa(requestedPort),
		strconv.Itoa(requestedPort+1),
	)
	install.Stdin = strings.NewReader(agentInstallScript())
	install.Env = append(os.Environ(), "HOME="+home, "XDG_RUNTIME_DIR="+filepath.Join(home, "missing-runtime"))
	installOutput, err := install.CombinedOutput()
	require.NoError(t, err, string(installOutput))

	selectedPort := requestedPort + 1
	require.Contains(t, string(installOutput), fmt.Sprintf("agent_port=%d", selectedPort))
	envData, err := os.ReadFile(filepath.Join(home, "oh-my-beszel", "config", "env"))
	require.NoError(t, err)
	require.Contains(t, string(envData), fmt.Sprintf(`LISTEN="127.0.0.1:%d"`, selectedPort))

	pidData, err := os.ReadFile(filepath.Join(home, "oh-my-beszel", "data", "agent.pid"))
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	require.NoError(t, err)
	t.Cleanup(func() {
		if process, findErr := os.FindProcess(pid); findErr == nil {
			_ = process.Kill()
		}
	})

	require.Eventually(t, func() bool {
		connection, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", selectedPort), 100*time.Millisecond)
		if dialErr != nil {
			return false
		}
		_ = connection.Close()
		return true
	}, 5*time.Second, 50*time.Millisecond)
}

func listenWithFreeSuccessor(t *testing.T) (net.Listener, int) {
	t.Helper()
	for range 100 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		port := listener.Addr().(*net.TCPAddr).Port
		if port >= 65535 {
			require.NoError(t, listener.Close())
			continue
		}
		successor, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port+1))
		if err != nil {
			require.NoError(t, listener.Close())
			continue
		}
		require.NoError(t, successor.Close())
		return listener, port
	}
	t.Fatal("unable to reserve an occupied port with a free successor")
	return nil, 0
}

const fakeAgentSource = `package main

import (
	"net"
	"os"
	"fmt"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "fingerprint" {
		fmt.Println("fixture-machine-fingerprint")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "health" {
		return
	}
	listener, err := net.Listen("tcp", os.Getenv("LISTEN"))
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		connection.Close()
	}
}
`
