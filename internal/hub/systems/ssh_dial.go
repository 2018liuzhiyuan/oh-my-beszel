package systems

import (
	"bytes"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"
)

const sshDialTimeout = 10 * time.Second

// DialAgent establishes a connection to the system's agent. Hosts that are
// IPs (or localhost) are dialed directly; anything else is treated as an
// ssh-config alias and delegated to the system's ssh client via `ssh -W`,
// so ProxyJump chains, key management, and later edits to ~/.ssh/config are
// all handled by ssh itself.
func (sm *SystemManager) DialAgent(host, port string) (net.Conn, error) {
	if isDirectHost(host) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), sshDialTimeout)
		if err != nil {
			return nil, err
		}
		return conn, nil
	}

	// the agent listens on the target host's loopback interface
	target := net.JoinHostPort("127.0.0.1", port)
	cmd := exec.Command("ssh",
		"-W", target, host,
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=8",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errBuf := &bytes.Buffer{}
	cmd.Stderr = errBuf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &sshPipeConn{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: errBuf,
	}, nil
}

// isDirectHost reports whether the host should be dialed directly instead of
// through the ssh client: IPs (v4/v6) and localhost.
func isDirectHost(host string) bool {
	if host == "" || host == "localhost" || strings.EqualFold(host, "localhost") {
		return true
	}
	if strings.HasPrefix(host, "/") { // unix socket path
		return true
	}
	return net.ParseIP(host) != nil
}

// sshPipeConn adapts an `ssh -W` child process to net.Conn. Closing the
// connection terminates the ssh process.
type sshPipeConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *bytes.Buffer

	closeOnce sync.Once
	closed    atomic.Bool
}

func (c *sshPipeConn) Read(b []byte) (int, error)  { return c.stdout.Read(b) }
func (c *sshPipeConn) Write(b []byte) (int, error) { return c.stdin.Write(b) }

func (c *sshPipeConn) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		c.stdin.Close()
		c.stdout.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait()
		if msg := strings.TrimSpace(c.stderr.String()); msg != "" {
			slog.Debug("ssh helper exited", "stderr", msg)
		}
	})
	return nil
}

func (c *sshPipeConn) LocalAddr() net.Addr                { return pipeAddr{} }
func (c *sshPipeConn) RemoteAddr() net.Addr               { return pipeAddr{} }
func (c *sshPipeConn) SetDeadline(time.Time) error         { return nil }
func (c *sshPipeConn) SetReadDeadline(time.Time) error     { return nil }
func (c *sshPipeConn) SetWriteDeadline(time.Time) error    { return nil }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "ssh" }
func (pipeAddr) String() string  { return "ssh-helper" }
