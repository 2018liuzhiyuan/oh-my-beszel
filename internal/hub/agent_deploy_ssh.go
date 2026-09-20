package hub

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/pocketbase/pocketbase/core"
)

var errAgentPlatformUnsupported = errors.New("agent auto-deploy: unsupported platform")

type agentInit string

const (
	agentInitSystemd agentInit = "systemd"
	agentInitNohup   agentInit = "nohup"
)

type agentPlatform struct {
	os   string
	arch string
	init agentInit
	libc string
}

func deployAgentOverSSH(ctx context.Context, app core.App, target agentDeploymentTarget) error {
	probeOutput, err := runAgentSSH(ctx, target, agentProbeCommand(target.port, target.publicKey), nil, false)
	if err != nil {
		return fmt.Errorf("probe remote platform: %w", err)
	}
	probe, err := parseAgentProbe(probeOutput)
	if err != nil {
		return err
	}
	artifact, err := loadAgentArtifact(ctx, app.DataDir(), probe.platform)
	if err != nil {
		return err
	}
	if probe.installedSha256 == artifact.sha256 && probe.healthy && probe.listening && probe.authorized {
		app.Logger().Info("Agent already installed and healthy; skipping redeploy", "system", target.id, "host", target.host)
		slog.Info("Agent already installed and healthy; skipping redeploy", "system", target.id, "host", target.host)
		return nil
	}
	stageName, err := newAgentStageName()
	if err != nil {
		return fmt.Errorf("create staging name: %w", err)
	}
	uploadAndInstall := agentInstallCommand(target, stageName, artifact.sha256)
	if _, err := runAgentSSH(ctx, target, uploadAndInstall, bytes.NewReader(artifact.data), true); err != nil {
		return fmt.Errorf("upload and install agent: %w", err)
	}
	app.Logger().Info("Agent auto-deploy completed", "system", target.id, "host", target.host, "platform", probe.platform.os+"/"+probe.platform.arch, "init", probe.platform.init)
	slog.Info("Agent auto-deploy completed", "system", target.id, "host", target.host, "platform", probe.platform.os+"/"+probe.platform.arch, "init", probe.platform.init)
	return nil
}

// agentProbeCommand reports the platform (four fields), the SHA-256 of any
// installed agent binary (or "none"), whether that binary answers its health
// check, and whether anything accepts connections on the agent port. The
// health check alone cannot be trusted: agents up to 0.18.8 touch the health
// file at process start, so their `health` subcommand always reports healthy
// even when the agent died. The listener check catches that case, so a dead
// agent is reinstalled instead of skipped.
func agentProbeCommand(port uint16, publicKey string) string {
	publicKeyBase64 := base64.StdEncoding.EncodeToString([]byte(publicKey))
	return fmt.Sprintf(`sh -c 'printf "%%s\n%%s\n%%s\n%%s\n" "$(uname -s)" "$(uname -m)" "$([ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1 && echo systemd || echo none)" "$(getconf GNU_LIBC_VERSION >/dev/null 2>&1 && echo glibc || echo unknown)"; BIN="$HOME/oh-my-beszel/bin/beszel-agent"; KEY_FILE="$HOME/oh-my-beszel/config/hub_keys"; KEY="$(printf %%s %[2]s | base64 -d)"; if [ -f "$BIN" ]; then sha256sum "$BIN" | cut -d" " -f1; else echo none; fi; if LISTEN="127.0.0.1:%[1]d" "$BIN" health >/dev/null 2>&1; then echo healthy; else echo unhealthy; fi; if command -v bash >/dev/null 2>&1 && bash -c "exec 3<>/dev/tcp/127.0.0.1/%[1]d" 2>/dev/null; then echo listening; elif command -v nc >/dev/null 2>&1 && nc -z 127.0.0.1 %[1]d >/dev/null 2>&1; then echo listening; else echo notlistening; fi; if [ -f "$KEY_FILE" ] && grep -Fqx "$KEY" "$KEY_FILE"; then echo authorized; else echo unauthorized; fi'`, port, publicKeyBase64)
}

type agentProbe struct {
	platform        agentPlatform
	installedSha256 string
	healthy         bool
	listening       bool
	authorized      bool
}

func parseAgentProbe(output string) (agentProbe, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 4 {
		return agentProbe{}, fmt.Errorf("unexpected probe output %q", strings.TrimSpace(output))
	}
	platform, err := parseAgentPlatform(strings.Join(lines[:4], " "))
	if err != nil {
		return agentProbe{}, err
	}
	probe := agentProbe{platform: platform, installedSha256: "none"}
	if len(lines) > 4 && strings.TrimSpace(lines[4]) != "" {
		probe.installedSha256 = strings.TrimSpace(lines[4])
	}
	if len(lines) > 5 && strings.TrimSpace(lines[5]) == "healthy" {
		probe.healthy = true
	}
	if len(lines) > 6 && strings.TrimSpace(lines[6]) == "listening" {
		probe.listening = true
	}
	if len(lines) > 7 && strings.TrimSpace(lines[7]) == "authorized" {
		probe.authorized = true
	}
	return probe, nil
}

func parseAgentPlatform(output string) (agentPlatform, error) {
	fields := strings.Fields(output)
	if len(fields) != 4 || !strings.EqualFold(fields[0], "linux") {
		return agentPlatform{}, fmt.Errorf("%w: %q", errAgentPlatformUnsupported, strings.TrimSpace(output))
	}
	platform := agentPlatform{os: "linux", libc: strings.ToLower(fields[3])}
	switch strings.ToLower(fields[1]) {
	case "x86_64", "amd64":
		platform.arch = "amd64"
	case "aarch64", "arm64":
		platform.arch = "arm64"
	default:
		return agentPlatform{}, fmt.Errorf("%w: architecture %q", errAgentPlatformUnsupported, fields[1])
	}
	if fields[2] == "systemd" {
		platform.init = agentInitSystemd
	} else {
		platform.init = agentInitNohup
	}
	return platform, nil
}

func newAgentStageName() (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "beszel-agent-" + hex.EncodeToString(random), nil
}

// agentInstallCommand streams the artifact into the staging file and pipes
// the install script through the same connection, so a deploy needs only one
// connection after the probe. Record values and the script travel base64
// encoded so they can never be interpreted as shell source; the trailing
// executable check guards against an empty script slipping through a failed
// decoder.
func agentInstallCommand(target agentDeploymentTarget, stageName, checksum string) string {
	script := base64.StdEncoding.EncodeToString([]byte(agentInstallScript()))
	publicKey := base64.StdEncoding.EncodeToString([]byte(target.publicKey))
	return fmt.Sprintf(
		`umask 077; cat > /tmp/%[1]s && printf %%s %[2]s | base64 -d | sh -s -- %[3]d %[4]s %[1]s %[5]s && test -x "$HOME/oh-my-beszel/bin/beszel-agent"`,
		stageName, script, target.port, publicKey, checksum,
	)
}

// runAgentSSH executes remoteCommand on the target host. The compress flag
// enables ssh transport compression for data-heavy calls such as the artifact
// upload; the Go agent binary shrinks roughly threefold.
func runAgentSSH(ctx context.Context, target agentDeploymentTarget, remoteCommand string, stdin io.Reader, compress bool) (string, error) {
	args := []string{
		"-F", target.sshConfig,
		"-o", "BatchMode=yes",
		"-o", "ClearAllForwardings=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=8",
	}
	if compress {
		args = append(args, "-C")
	}
	args = append(args, "--", target.host, remoteCommand)
	sshBinary, err := systems.ResolveSSHClient()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, sshBinary, args...)
	cmd.Stdin = stdin
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("ssh command: %s", message)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func agentInstallScript() string {
	return `set -eu
PORT="$1"
KEY_B64="$2"
STAGE_NAME="$3"
EXPECTED_SHA="$4"
STAGE="/tmp/$STAGE_NAME"
BASE="${HOME:?}/oh-my-beszel"
BIN_DIR="$BASE/bin"
CONFIG_DIR="$BASE/config"
DATA_DIR="$BASE/data"
LOG_DIR="$BASE/logs"
BIN="$BASE/bin/beszel-agent"
ENV_FILE="$BASE/config/env"
KEY_FILE="$BASE/config/hub_keys"
RUNNER="$BASE/bin/run-agent"
PID_FILE="$DATA_DIR/agent.pid"
SERVICE_NAME="oh-my-beszel-agent.service"
SERVICE_FILE="$CONFIG_DIR/$SERVICE_NAME"
USER_UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
USER_UNIT="$USER_UNIT_DIR/$SERVICE_NAME"

cleanup() {
  rm -f "$STAGE" "/tmp/$STAGE_NAME.env" "/tmp/$STAGE_NAME.run" "/tmp/$STAGE_NAME.service"
}
trap cleanup EXIT

if [ "$(sha256sum "$STAGE" | awk '{print $1}')" != "$EXPECTED_SHA" ]; then
  echo "uploaded agent checksum mismatch" >&2
  exit 20
fi

KEY="$(printf '%s' "$KEY_B64" | base64 -d)"
printf 'LISTEN="127.0.0.1:%s"\nKEY_FILE="$HOME/oh-my-beszel/config/hub_keys"\nDATA_DIR="$HOME/oh-my-beszel/data"\n' "$PORT" > "/tmp/$STAGE_NAME.env"
cat > "/tmp/$STAGE_NAME.run" <<'RUNNER'
#!/bin/sh
set -eu
set -a
. "$HOME/oh-my-beszel/config/env"
set +a
exec "$HOME/oh-my-beszel/bin/beszel-agent"
RUNNER
cat > "/tmp/$STAGE_NAME.service" <<'SERVICE'
[Unit]
Description=Oh My Beszel Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%h/oh-my-beszel/bin/run-agent
WorkingDirectory=%h/oh-my-beszel/data
StandardOutput=append:%h/oh-my-beszel/logs/agent.log
StandardError=append:%h/oh-my-beszel/logs/agent.log
Restart=always
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=default.target
SERVICE

install -d -m 0755 "$BASE" "$BIN_DIR" "$DATA_DIR" "$LOG_DIR"
install -d -m 0700 "$CONFIG_DIR"
if test -f "$BIN"; then
  cp -f "$BIN" "$BIN.previous"
fi
install -m 0755 "$STAGE" "$BIN"
install -m 0600 "/tmp/$STAGE_NAME.env" "$ENV_FILE"
install -m 0755 "/tmp/$STAGE_NAME.run" "$RUNNER"
install -m 0644 "/tmp/$STAGE_NAME.service" "$SERVICE_FILE"
touch "$KEY_FILE"
chmod 0600 "$KEY_FILE"
if ! grep -Fqx "$KEY" "$KEY_FILE"; then
  printf '%s\n' "$KEY" >> "$KEY_FILE"
fi

rollback() {
  if test -f "$BIN.previous"; then
    cp -f "$BIN.previous" "$BIN"
  fi
}

stop_detached() {
  if [ ! -f "$PID_FILE" ]; then
    return
  fi
  pid="$(cat "$PID_FILE")"
  case "$pid" in
    *[!0-9]*|"") return ;;
  esac
  if [ "$pid" -gt 1 ] && [ -e "/proc/$pid/exe" ] && [ "$(readlink "/proc/$pid/exe")" = "$BIN" ]; then
    kill "$pid" || true
  fi
  rm -f "$PID_FILE"
}

port_listening() {
  if command -v bash >/dev/null 2>&1 && bash -c "exec 3<>/dev/tcp/127.0.0.1/$PORT" 2>/dev/null; then
    return 0
  fi
  command -v nc >/dev/null 2>&1 && nc -z 127.0.0.1 "$PORT" >/dev/null 2>&1
}

SYSTEMD_USER=0
RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1 && [ -d "$RUNTIME_DIR" ]; then
  export XDG_RUNTIME_DIR="$RUNTIME_DIR"
  if systemctl --user show-environment >/dev/null 2>&1; then
    SYSTEMD_USER=1
  fi
fi

# Stop only the Agent managed by this account before checking the requested
# port. On shared hosts another user or a system service may already own the
# default port; treating that listener as ours reports a false successful
# deployment and later surfaces as a misleading public-key authentication
# error from the foreign Agent.
if [ "$SYSTEMD_USER" -eq 1 ]; then
  stop_detached
  systemctl --user stop oh-my-beszel-agent.service >/dev/null 2>&1 || true
else
  stop_detached
fi
if port_listening; then
  rollback
  echo "agent port $PORT is already in use after stopping this account's managed Agent; choose a different Agent port for this system" >&2
  exit 24
fi

if [ "$SYSTEMD_USER" -eq 1 ]; then
  install -d -m 0755 "$USER_UNIT_DIR"
  ln -sfn "$SERVICE_FILE" "$USER_UNIT"
  systemctl --user daemon-reload
  systemctl --user enable oh-my-beszel-agent.service >/dev/null
  if ! systemctl --user restart oh-my-beszel-agent.service; then
    rollback
    systemctl --user restart oh-my-beszel-agent.service || true
    exit 22
  fi
else
  if command -v setsid >/dev/null 2>&1; then
    setsid "$RUNNER" >>"$LOG_DIR/agent.log" 2>&1 </dev/null &
  else
    nohup "$RUNNER" >>"$LOG_DIR/agent.log" 2>&1 </dev/null &
  fi
  echo $! > "$PID_FILE"
fi

attempt=0
while [ "$attempt" -lt 8 ]; do
  if port_listening; then
    if [ "$SYSTEMD_USER" -eq 0 ] || systemctl --user is-active --quiet oh-my-beszel-agent.service; then
      exit 0
    fi
  fi
  attempt=$((attempt + 1))
  sleep 1
done

rollback
if [ "$SYSTEMD_USER" -eq 1 ]; then
  systemctl --user restart oh-my-beszel-agent.service || true
fi
echo "agent failed its health check" >&2
exit 23
`
}
