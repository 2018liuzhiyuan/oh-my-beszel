package systems

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"

	"golang.org/x/crypto/ssh"
)

const sshDialTimeout = 10 * time.Second

// DialAgent establishes a connection to the system's agent. A selected SSH
// config delegates every host to `ssh -W`; otherwise IPs and localhost are
// dialed directly while aliases use the default SSH config.
func (sm *SystemManager) DialAgent(host, port, configPath string) (net.Conn, error) {
	if isDirectHost(host, configPath) {
		dialer := net.Dialer{Timeout: sshDialTimeout, KeepAlive: sshKeepAliveInterval}
		conn, err := dialer.Dial("tcp", net.JoinHostPort(host, port))
		if err != nil {
			return nil, err
		}
		return conn, nil
	}

	// the agent listens on the target host's loopback interface
	target := net.JoinHostPort("127.0.0.1", port)
	sshBinary, err := ResolveSSHClient()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(sshBinary, sshDialArgs(target, host, configPath)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	errBuf := &syncBuffer{}
	cmd.Stderr = errBuf
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ssh helper: %w", err)
	}
	return &sshPipeConn{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: errBuf,
	}, nil
}

func sshDialArgs(target, host, configPath string) []string {
	args := make([]string, 0, 15)
	if configPath != "" {
		args = append(args, "-F", configPath)
	}
	return append(args,
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=8",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-W", target,
		host,
	)
}

func isDirectHost(host, configPath string) bool {
	if configPath != "" {
		return false
	}
	if host == "" || host == "localhost" || strings.EqualFold(host, "localhost") {
		return true
	}
	if strings.HasPrefix(host, "/") { // unix socket path
		return true
	}
	return net.ParseIP(host) != nil
}

// sshHandshakeTimeout bounds the agent SSH handshake. The `ssh -W` helper's
// pipe connection cannot enforce deadlines, so without this bound a wedged
// tunnel would stall the system's poll loop indefinitely.
const sshHandshakeTimeout = 15 * time.Second

func sshHandshake(conn net.Conn, addr string, config *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
	type handshakeResult struct {
		conn  ssh.Conn
		chans <-chan ssh.NewChannel
		reqs  <-chan *ssh.Request
	}
	results := make(chan handshakeResult, 1)
	errs := make(chan error, 1)
	go func() {
		c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
		if err != nil {
			errs <- err
			return
		}
		results <- handshakeResult{conn: c, chans: chans, reqs: reqs}
	}()
	select {
	case result := <-results:
		return result.conn, result.chans, result.reqs, nil
	case err := <-errs:
		return nil, nil, nil, err
	case <-time.After(sshHandshakeTimeout):
		// kill the helper so the handshake goroutine can finish; its buffered
		// channel send never blocks, so the goroutine is not leaked
		conn.Close()
		return nil, nil, nil, fmt.Errorf("ssh handshake timed out after %s", sshHandshakeTimeout)
	}
}

// sshPipeConn adapts an `ssh -W` child process to net.Conn. Closing the
// connection terminates the ssh process.
type sshPipeConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *syncBuffer

	closeOnce sync.Once
	closed    atomic.Bool
}

// HelperStderr returns the ssh child process output captured so far. Callers
// should Close the connection first: Close waits for the process to exit, at
// which point stderr is complete.
func (c *sshPipeConn) HelperStderr() string {
	return strings.TrimSpace(c.stderr.String())
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

// syncBuffer is a bytes.Buffer that tolerates concurrent writes from the
// exec package's copying goroutine and reads from the connection owner.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (c *sshPipeConn) LocalAddr() net.Addr              { return pipeAddr{} }
func (c *sshPipeConn) RemoteAddr() net.Addr             { return pipeAddr{} }
func (c *sshPipeConn) SetDeadline(time.Time) error      { return nil }
func (c *sshPipeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *sshPipeConn) SetWriteDeadline(time.Time) error { return nil }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "ssh" }
func (pipeAddr) String() string  { return "ssh-helper" }
