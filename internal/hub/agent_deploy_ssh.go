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
	"strconv"
	"strings"

	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/henrygd/beszel/internal/hub/utils"
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
	if probe.matchesManagedAgent(artifact.sha256) {
		app.Logger().Info("Agent already installed and healthy; skipping redeploy", "system", target.id, "host", target.host)
		slog.Info("Agent already installed and healthy; skipping redeploy", "system", target.id, "host", target.host)
		result, err := parseAgentDeploymentResult(probeOutput)
		if err != nil {
			return err
		}
		return persistAgentDeployment(app, target, result)
	}
	stageName, err := newAgentStageName()
	if err != nil {
		return fmt.Errorf("create staging name: %w", err)
	}
	rangeStart, rangeEnd, err := configuredAgentPortRange(target.port)
	if err != nil {
		return err
	}
	uploadAndInstall := agentInstallCommand(target, stageName, artifact.sha256, rangeStart, rangeEnd)
	installOutput, err := runAgentSSH(ctx, target, uploadAndInstall, bytes.NewReader(artifact.data), true)
	if err != nil {
		if strings.Contains(err.Error(), "BESZEL_AGENT_PORT_RANGE_EXHAUSTED") {
			return fmt.Errorf("%w: Agent 端口 %d 及备用范围 %d-%d 均被占用 / Agent port %d and fallback range %d-%d are occupied. 请修改 Hub 的 config.json 中 portRanges.agent.start/end，扩大范围并重启 Hub / Edit portRanges.agent.start/end in Hub's config.json, expand the range and restart Hub", errAgentPortRange, target.port, rangeStart, rangeEnd, target.port, rangeStart, rangeEnd)
		}
		return fmt.Errorf("upload and install agent: %w", err)
	}
	result, err := parseAgentDeploymentResult(installOutput)
	if err != nil {
		return err
	}
	if err := persistAgentDeployment(app, target, result); err != nil {
		return err
	}
	app.Logger().Info("Agent auto-deploy completed", "system", target.id, "host", target.host, "port", result.port, "platform", probe.platform.os+"/"+probe.platform.arch, "init", probe.platform.init)
	slog.Info("Agent auto-deploy completed", "system", target.id, "host", target.host, "port", result.port, "platform", probe.platform.os+"/"+probe.platform.arch, "init", probe.platform.init)
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
	return fmt.Sprintf(`sh <<'BESZEL_PROBE'
printf "%%s\n%%s\n%%s\n%%s\n" "$(uname -s)" "$(uname -m)" "$([ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1 && echo systemd || echo none)" "$(getconf GNU_LIBC_VERSION >/dev/null 2>&1 && echo glibc || echo unknown)"
INSTALL_ROOT="${HOME:?}/oh-my-beszel"
BASE="$INSTALL_ROOT"
case "$(stat -f -c %%T "$HOME" 2>/dev/null)" in
  nfs*) MACHINE_NAMESPACE="$(hostname | sha256sum | cut -c1-32)"; BASE="$INSTALL_ROOT/hosts/$MACHINE_NAMESPACE" ;;
esac
BIN="$BASE/bin/beszel-agent"
ENV_FILE="$BASE/config/env"
KEY_FILE="$BASE/config/hub_keys"
PID_FILE="$BASE/data/agent.pid"
KEY="$(printf %%s %[2]s | base64 -d)"
PORT=%[1]d
if [ -f "$ENV_FILE" ]; then
  SAVED_PORT="$(sed -n 's/^LISTEN="\{0,1\}127[.]0[.]0[.]1:\([0-9][0-9]*\)"\{0,1\}$/\1/p' "$ENV_FILE")"
  case "$SAVED_PORT" in *[!0-9]*|"") ;; *) if [ "$SAVED_PORT" -gt 0 ] && [ "$SAVED_PORT" -le 65535 ]; then PORT="$SAVED_PORT"; fi ;; esac
fi
if [ -f "$BIN" ]; then sha256sum "$BIN" | cut -d" " -f1; else echo none; fi
if LISTEN="127.0.0.1:$PORT" DATA_DIR="$BASE/data" "$BIN" health >/dev/null 2>&1; then echo healthy; else echo unhealthy; fi
if command -v bash >/dev/null 2>&1 && bash -c "exec 3<>/dev/tcp/127.0.0.1/$PORT" 2>/dev/null; then echo listening
elif command -v nc >/dev/null 2>&1 && nc -z 127.0.0.1 "$PORT" >/dev/null 2>&1; then echo listening
else echo notlistening; fi
if [ -f "$KEY_FILE" ] && grep -Fqx "$KEY" "$KEY_FILE"; then echo authorized; else echo unauthorized; fi
owns_listener() {
  listener_pid="$1"
  port_hex="$(printf '%%04X' "$PORT")"
  inodes="$(awk -v address="0100007F:$port_hex" '$2 == address && $4 == "0A" {print $10}' "/proc/$listener_pid/net/tcp" 2>/dev/null)" || return 1
  for descriptor in /proc/"$listener_pid"/fd/*; do
    socket="$(readlink "$descriptor" 2>/dev/null || true)"
    for inode in $inodes; do
      if [ "$socket" = "socket:[$inode]" ]; then return 0; fi
    done
  done
  return 1
}
MANAGED=unmanaged
if [ -n "${SAVED_PORT:-}" ]; then
  RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
  if [ -d "$RUNTIME_DIR" ]; then export XDG_RUNTIME_DIR="$RUNTIME_DIR"; fi
  if command -v systemctl >/dev/null 2>&1 && systemctl --user is-active --quiet oh-my-beszel-agent.service >/dev/null 2>&1; then
    PID="$(systemctl --user show --property=MainPID --value oh-my-beszel-agent.service)"
  elif [ -f "$PID_FILE" ]; then PID="$(cat "$PID_FILE")"
  else PID=""; fi
  case "$PID" in *[!0-9]*|"") ;; *) if [ "$PID" -gt 1 ] && [ "$(readlink "/proc/$PID/exe")" = "$(readlink -f "$BIN")" ] && owns_listener "$PID"; then MANAGED=managed; fi ;; esac
fi
echo "$MANAGED"
if [ -f "$BIN" ]; then
  printf 'agent_machine=%%s\n' "$(DATA_DIR="$BASE/data" "$BIN" fingerprint view 2>/dev/null)"
fi
printf 'agent_uid=%%s\nagent_dir_b64=%%s\nagent_port=%%s\n' "$(id -u)" "$(cd "$BASE" 2>/dev/null && pwd -P | tr -d '\n' | base64 | tr -d '\n')" "$PORT"
BESZEL_PROBE`, port, publicKeyBase64)
}

type agentProbe struct {
	platform        agentPlatform
	installedSha256 string
	healthy         bool
	listening       bool
	authorized      bool
	managed         bool
}

func (probe agentProbe) matchesManagedAgent(checksum string) bool {
	return probe.installedSha256 == checksum && probe.healthy && probe.listening && probe.authorized && probe.managed
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
	if len(lines) > 8 && strings.TrimSpace(lines[8]) == "managed" {
		probe.managed = true
	}
	return probe, nil
}

func parseAgentInstallPort(output string) (uint16, error) {
	for line := range strings.Lines(output) {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "agent_port=")
		if !ok {
			continue
		}
		port, err := strconv.ParseUint(value, 10, 16)
		if err != nil || port == 0 {
			return 0, fmt.Errorf("invalid installed agent port %q", value)
		}
		return uint16(port), nil
	}
	return 0, fmt.Errorf("agent install did not report its port")
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
func configuredAgentPortRange(preferred uint16) (uint16, uint16, error) {
	value, exists := utils.GetEnv("AGENT_PORT_RANGE")
	if !exists || value == "" {
		start := uint16(min(int(preferred)+1, 65535))
		end := uint16(min(int(preferred)+100, 65535))
		return start, end, nil
	}
	startText, endText, ok := strings.Cut(value, "-")
	start, startErr := strconv.ParseUint(startText, 10, 16)
	end, endErr := strconv.ParseUint(endText, 10, 16)
	if !ok || startErr != nil || endErr != nil || start == 0 || end == 0 || start > end {
		return 0, 0, fmt.Errorf("%w: config.json 的 portRanges.agent.start/end 无效，需满足 1 <= start <= end <= 65535；修改后重启 Hub / Invalid portRanges.agent.start/end in config.json; require 1 <= start <= end <= 65535. Correct it and restart Hub", errAgentPortRange)
	}
	return uint16(start), uint16(end), nil
}

func agentInstallCommand(target agentDeploymentTarget, stageName, checksum string, rangeStart, rangeEnd uint16) string {
	script := base64.StdEncoding.EncodeToString([]byte(agentInstallScript()))
	publicKey := base64.StdEncoding.EncodeToString([]byte(target.publicKey))
	return fmt.Sprintf(
		`umask 077; cat > /tmp/%[1]s && printf %%s %[2]s | base64 -d | sh -s -- %[3]d %[4]s %[1]s %[5]s %[6]d %[7]d && test -x "$HOME/oh-my-beszel/bin/run-agent"`,
		stageName, script, target.port, publicKey, checksum, rangeStart, rangeEnd,
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
umask 077
PORT="$1"
REQUESTED_PORT="$PORT"
KEY_B64="$2"
STAGE_NAME="$3"
EXPECTED_SHA="$4"
RANGE_START="$5"
RANGE_END="$6"
STAGE="/tmp/$STAGE_NAME"
INSTALL_ROOT="${HOME:?}/oh-my-beszel"
BASE="$INSTALL_ROOT"
case "$(stat -f -c %T "$HOME" 2>/dev/null)" in
  nfs*) MACHINE_NAMESPACE="$(hostname | sha256sum | cut -c1-32)"; BASE="$INSTALL_ROOT/hosts/$MACHINE_NAMESPACE" ;;
esac
BIN_DIR="$BASE/bin"
CONFIG_DIR="$BASE/config"
DATA_DIR="$BASE/data"
LOG_DIR="$BASE/logs"
BIN="$BASE/bin/beszel-agent"
ENV_FILE="$BASE/config/env"
KEY_FILE="$BASE/config/hub_keys"
RUNNER="$INSTALL_ROOT/bin/run-agent"
PID_FILE="$DATA_DIR/agent.pid"
SERVICE_NAME="oh-my-beszel-agent.service"
SERVICE_FILE="$INSTALL_ROOT/config/$SERVICE_NAME"
USER_UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
USER_UNIT="$USER_UNIT_DIR/$SERVICE_NAME"

cleanup() {
  rm -f "$STAGE" "/tmp/$STAGE_NAME.env" "/tmp/$STAGE_NAME.run" "/tmp/$STAGE_NAME.service" "$RUNNER.$STAGE_NAME" "$SERVICE_FILE.$STAGE_NAME"
}
trap cleanup EXIT
trap 'exit 26' HUP INT TERM

install -d -m 0700 "$INSTALL_ROOT" "$BASE"
if ! command -v flock >/dev/null 2>&1; then
  echo "agent deployment requires flock" >&2
  exit 25
fi
exec 9>"$BASE/.deploy.lock"
if ! flock -w 90 9; then
  echo "agent deployment lock timed out" >&2
  exit 25
fi

if [ "$(sha256sum "$STAGE" | awk '{print $1}')" != "$EXPECTED_SHA" ]; then
  echo "uploaded agent checksum mismatch" >&2
  exit 20
fi
KEY="$(printf '%s' "$KEY_B64" | base64 -d)"
SYSTEMD_USER=0
RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1 && [ -d "$RUNTIME_DIR" ]; then
  export XDG_RUNTIME_DIR="$RUNTIME_DIR"
  if systemctl --user show-environment >/dev/null 2>&1; then SYSTEMD_USER=1; fi
fi

managed_alive() {
  if [ "$SYSTEMD_USER" -eq 1 ]; then
    pid="$(systemctl --user show --property=MainPID --value "$SERVICE_NAME" 2>/dev/null)" || return 1
  elif [ -f "$PID_FILE" ]; then pid="$(cat "$PID_FILE")"
  else return 1; fi
  case "$pid" in *[!0-9]*|"") return 1 ;; esac
  [ "$pid" -gt 1 ] && [ "$(readlink "/proc/$pid/exe")" = "$(readlink -f "$BIN")" ]
}

port_listening() {
  if command -v bash >/dev/null 2>&1 && bash -c "exec 3<>/dev/tcp/127.0.0.1/$PORT" 2>/dev/null; then
    return 0
  fi
  command -v nc >/dev/null 2>&1 && nc -z 127.0.0.1 "$PORT" >/dev/null 2>&1
}

owns_listener() {
  listener_pid="$1"
  port_hex="$(printf '%04X' "$PORT")"
  inodes="$(awk -v address="0100007F:$port_hex" '$2 == address && $4 == "0A" {print $10}' "/proc/$listener_pid/net/tcp" 2>/dev/null)" || return 1
  for descriptor in /proc/"$listener_pid"/fd/*; do
    socket="$(readlink "$descriptor" 2>/dev/null || true)"
    for inode in $inodes; do
      if [ "$socket" = "socket:[$inode]" ]; then return 0; fi
    done
  done
  return 1
}

managed_listening() {
  managed_alive && owns_listener "$pid"
}

report_instance() {
  fingerprint="$(DATA_DIR="$DATA_DIR" "$BIN" fingerprint view)"
  [ -n "$fingerprint" ] || return 1
  printf 'agent_machine=%s\nagent_uid=%s\nagent_dir_b64=%s\n' "$fingerprint" "$(id -u)" "$(cd "$BASE" && pwd -P | tr -d '\n' | base64 | tr -d '\n')"
  printf 'agent_port=%s\n' "$PORT"
}

# A second Hub or SSH alias may have completed the deployment while we waited.
# Reuse that account's live instance and its selected port without restarting it.
if [ -f "$ENV_FILE" ]; then
  SAVED_PORT="$(sed -n 's/^LISTEN="\{0,1\}127[.]0[.]0[.]1:\([0-9][0-9]*\)"\{0,1\}$/\1/p' "$ENV_FILE")"
  case "$SAVED_PORT" in *[!0-9]*|"") ;; *) if [ "$SAVED_PORT" -gt 0 ] && [ "$SAVED_PORT" -le 65535 ]; then PORT="$SAVED_PORT"; fi ;; esac
fi
if [ -f "$BIN" ] && [ -f "$KEY_FILE" ] && [ "$(sha256sum "$BIN" | awk '{print $1}')" = "$EXPECTED_SHA" ] && grep -Fqx "$KEY" "$KEY_FILE" && managed_listening && LISTEN="127.0.0.1:$PORT" DATA_DIR="$DATA_DIR" "$BIN" health >/dev/null 2>&1; then
  report_instance
  exit 0
fi

stop_detached() {
  if [ ! -f "$PID_FILE" ]; then return; fi
  pid="$(cat "$PID_FILE")"
  case "$pid" in *[!0-9]*|"") return ;; esac
  if [ "$pid" -gt 1 ] && [ "$(readlink "/proc/$pid/exe")" = "$(readlink -f "$BIN")" ]; then
    kill "$pid" || true
    stop_attempt=0
    while [ "$stop_attempt" -lt 5 ] && [ "$(readlink "/proc/$pid/exe")" = "$(readlink -f "$BIN")" ]; do
      stop_attempt=$((stop_attempt + 1))
      sleep 1
    done
    if [ "$(readlink "/proc/$pid/exe")" = "$(readlink -f "$BIN")" ]; then
      echo "managed agent did not stop" >&2
      exit 27
    fi
  fi
  rm -f "$PID_FILE"
}

# Stop this account's process before replacing its executable or selecting a port.
stop_detached
if [ "$SYSTEMD_USER" -eq 1 ]; then
  systemctl --user stop oh-my-beszel-agent.service >/dev/null 2>&1 || true
fi
if port_listening; then
  port_found=0
  PORT="$RANGE_START"
  while [ "$PORT" -le "$RANGE_END" ]; do
    if [ "$PORT" -eq "$REQUESTED_PORT" ]; then PORT=$((PORT + 1)); continue; fi
    if ! port_listening; then port_found=1; break; fi
    PORT=$((PORT + 1))
  done
  if [ "$port_found" -ne 1 ]; then
    echo "BESZEL_AGENT_PORT_RANGE_EXHAUSTED: no free agent port for $REQUESTED_PORT in $RANGE_START-$RANGE_END" >&2
    exit 24
  fi
fi

install -d -m 0755 "$INSTALL_ROOT/bin" "$BIN_DIR"
install -d -m 0700 "$INSTALL_ROOT/config" "$INSTALL_ROOT/logs" "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
if test -f "$BIN"; then cp -f "$BIN" "$BIN.previous"; fi
if test -f "$ENV_FILE"; then cp -f "$ENV_FILE" "$ENV_FILE.previous"; fi
install -m 0755 "$STAGE" "$BIN"
printf 'LISTEN="127.0.0.1:%s"\nKEY_FILE="$BESZEL_INSTANCE_BASE/config/hub_keys"\nDATA_DIR="$BESZEL_INSTANCE_BASE/data"\n' "$PORT" > "$ENV_FILE"
chmod 0600 "$ENV_FILE"
cat > "/tmp/$STAGE_NAME.run" <<'RUNNER'
#!/bin/sh
set -eu
set -a
INSTALL_ROOT="${HOME:?}/oh-my-beszel"
BASE="$INSTALL_ROOT"
case "$(stat -f -c %T "$HOME" 2>/dev/null)" in
  nfs*) MACHINE_NAMESPACE="$(hostname | sha256sum | cut -c1-32)"; BASE="$INSTALL_ROOT/hosts/$MACHINE_NAMESPACE" ;;
esac
BESZEL_INSTANCE_BASE="$BASE"
. "$BASE/config/env"
set +a
exec "$BASE/bin/beszel-agent" >>"$BASE/logs/agent.log" 2>&1
RUNNER
cat > "/tmp/$STAGE_NAME.service" <<'SERVICE'
[Unit]
Description=Oh My Beszel Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%h/oh-my-beszel/bin/run-agent
WorkingDirectory=%h/oh-my-beszel
StandardOutput=append:%h/oh-my-beszel/logs/agent.log
StandardError=append:%h/oh-my-beszel/logs/agent.log
Restart=always
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=default.target
SERVICE
install -m 0755 "/tmp/$STAGE_NAME.run" "$RUNNER.$STAGE_NAME"
mv -f "$RUNNER.$STAGE_NAME" "$RUNNER"
install -m 0644 "/tmp/$STAGE_NAME.service" "$SERVICE_FILE.$STAGE_NAME"
mv -f "$SERVICE_FILE.$STAGE_NAME" "$SERVICE_FILE"
touch "$KEY_FILE"
chmod 0600 "$KEY_FILE"
if ! grep -Fqx "$KEY" "$KEY_FILE"; then printf '%s\n' "$KEY" >> "$KEY_FILE"; fi

rollback() {
  if test -f "$BIN.previous"; then cp -f "$BIN.previous" "$BIN"; fi
  if test -f "$ENV_FILE.previous"; then cp -f "$ENV_FILE.previous" "$ENV_FILE"; fi
}

if [ "$SYSTEMD_USER" -eq 1 ]; then
  install -d -m 0755 "$USER_UNIT_DIR"
  ln -sfn "$SERVICE_FILE" "$USER_UNIT"
  systemctl --user daemon-reload
  systemctl --user enable oh-my-beszel-agent.service >/dev/null
fi

# Another UID can claim a candidate between the check and bind. Retry only when
# our process failed and that port now has a foreign listener.
while :; do
  if [ "$SYSTEMD_USER" -eq 1 ]; then
    systemctl --user restart oh-my-beszel-agent.service || true
  else
    if command -v setsid >/dev/null 2>&1; then
      setsid "$RUNNER" 9>&- >>"$LOG_DIR/agent.log" 2>&1 </dev/null &
    else
      nohup "$RUNNER" 9>&- >>"$LOG_DIR/agent.log" 2>&1 </dev/null &
    fi
    echo $! > "$PID_FILE"
  fi
  attempt=0
  while [ "$attempt" -lt 30 ]; do
    if managed_listening && LISTEN="127.0.0.1:$PORT" DATA_DIR="$DATA_DIR" "$BIN" health >/dev/null 2>&1; then
      report_instance
      exit 0
    fi
    attempt=$((attempt + 1))
    if [ "$attempt" -gt 1 ] && ! managed_alive; then break; fi
    sleep 1
  done
  if managed_listening || ! port_listening; then break; fi
  stop_detached
  if [ "$SYSTEMD_USER" -eq 1 ]; then systemctl --user stop "$SERVICE_NAME" >/dev/null 2>&1 || true; fi
  if [ "$PORT" -ge "$RANGE_START" ] && [ "$PORT" -le "$RANGE_END" ]; then PORT=$((PORT + 1)); else PORT="$RANGE_START"; fi
  while [ "$PORT" -le "$RANGE_END" ] && port_listening; do PORT=$((PORT + 1)); done
  if [ "$PORT" -gt "$RANGE_END" ]; then
    rollback
    echo "BESZEL_AGENT_PORT_RANGE_EXHAUSTED: no free agent port for $REQUESTED_PORT in $RANGE_START-$RANGE_END" >&2
    exit 24
  fi
  printf 'LISTEN="127.0.0.1:%s"\nKEY_FILE="$BESZEL_INSTANCE_BASE/config/hub_keys"\nDATA_DIR="$BESZEL_INSTANCE_BASE/data"\n' "$PORT" > "$ENV_FILE"
done

stop_detached
rollback
if [ "$SYSTEMD_USER" -eq 1 ]; then systemctl --user restart oh-my-beszel-agent.service || true; fi
echo "agent failed its health check" >&2
exit 23
`
}
