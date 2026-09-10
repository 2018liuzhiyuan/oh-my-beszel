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
	"os/exec"
	"strconv"
	"strings"

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
	platformOutput, err := runAgentSSH(ctx, target, "sh -c 'printf \"%s\\n%s\\n%s\\n%s\\n\" \"$(uname -s)\" \"$(uname -m)\" \"$([ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1 && echo systemd || echo none)\" \"$(getconf GNU_LIBC_VERSION >/dev/null 2>&1 && echo glibc || echo unknown)\"'", nil)
	if err != nil {
		return fmt.Errorf("probe remote platform: %w", err)
	}
	platform, err := parseAgentPlatform(platformOutput)
	if err != nil {
		return err
	}
	artifact, err := loadAgentArtifact(ctx, app.DataDir(), platform)
	if err != nil {
		return err
	}
	stageName, err := newAgentStageName()
	if err != nil {
		return fmt.Errorf("create staging name: %w", err)
	}
	uploadCommand := "umask 077; cat > /tmp/" + stageName
	if _, err := runAgentSSH(ctx, target, uploadCommand, bytes.NewReader(artifact.data)); err != nil {
		return fmt.Errorf("upload agent artifact: %w", err)
	}
	command, args := agentInstallCommand(target, stageName, artifact.sha256)
	remoteCommand := command + " " + strings.Join(args, " ")
	if _, err := runAgentSSH(ctx, target, remoteCommand, strings.NewReader(agentInstallScript())); err != nil {
		return fmt.Errorf("install agent: %w", err)
	}
	app.Logger().Info("Agent auto-deploy completed", "system", target.id, "host", target.host, "platform", platform.os+"/"+platform.arch, "init", platform.init)
	return nil
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

func agentInstallCommand(target agentDeploymentTarget, stageName, checksum string) (string, []string) {
	return "sh -s --", []string{
		strconv.FormatUint(uint64(target.port), 10),
		base64.StdEncoding.EncodeToString([]byte(target.publicKey)),
		stageName,
		checksum,
	}
}

func runAgentSSH(ctx context.Context, target agentDeploymentTarget, remoteCommand string, stdin io.Reader) (string, error) {
	args := []string{
		"-F", target.sshConfig,
		"-o", "BatchMode=yes",
		"-o", "ClearAllForwardings=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=8",
		"--", target.host, remoteCommand,
	}
	cmd := exec.CommandContext(ctx, "ssh", args...)
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
DIR="/opt/beszel-agent"
BIN="$DIR/beszel-agent"
ENV_FILE="$DIR/env"
RUNNER="$DIR/run.sh"
PID_FILE="$DIR/beszel-agent.pid"

cleanup() {
  rm -f "$STAGE" "/tmp/$STAGE_NAME.env" "/tmp/$STAGE_NAME.run" "/tmp/$STAGE_NAME.service"
}
trap cleanup EXIT

if [ "$(sha256sum "$STAGE" | awk '{print $1}')" != "$EXPECTED_SHA" ]; then
  echo "uploaded agent checksum mismatch" >&2
  exit 20
fi

if [ "$(id -u)" -eq 0 ]; then
  SUDO=""
elif sudo -n true >/dev/null 2>&1; then
  SUDO="sudo -n"
else
  echo "root or passwordless sudo is required" >&2
  exit 21
fi

run() {
  if [ -n "$SUDO" ]; then
    sudo -n "$@"
  else
    "$@"
  fi
}

KEY="$(printf '%s' "$KEY_B64" | base64 -d)"
printf 'LISTEN="127.0.0.1:%s"\nKEY="%s"\n' "$PORT" "$KEY" > "/tmp/$STAGE_NAME.env"
cat > "/tmp/$STAGE_NAME.run" <<'RUNNER'
#!/bin/sh
set -a
. /opt/beszel-agent/env
set +a
exec /opt/beszel-agent/beszel-agent
RUNNER
cat > "/tmp/$STAGE_NAME.service" <<'SERVICE'
[Unit]
Description=Beszel Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/opt/beszel-agent/env
ExecStart=/opt/beszel-agent/beszel-agent
Restart=always
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
SERVICE

run install -d -m 0755 "$DIR"
if run test -f "$BIN"; then
  run cp -f "$BIN" "$BIN.previous"
fi
run install -m 0755 "$STAGE" "$BIN"
run install -m 0600 "/tmp/$STAGE_NAME.env" "$ENV_FILE"
run install -m 0755 "/tmp/$STAGE_NAME.run" "$RUNNER"

rollback() {
  if run test -f "$BIN.previous"; then
    run cp -f "$BIN.previous" "$BIN"
  fi
}

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
  run install -m 0644 "/tmp/$STAGE_NAME.service" /etc/systemd/system/beszel-agent.service
  run systemctl daemon-reload
  run systemctl enable beszel-agent.service >/dev/null
  if ! run systemctl restart beszel-agent.service; then
    rollback
    run systemctl restart beszel-agent.service || true
    exit 22
  fi
else
  run sh -c 'if [ -f /opt/beszel-agent/beszel-agent.pid ]; then pid=$(cat /opt/beszel-agent/beszel-agent.pid); case "$pid" in *[!0-9]*|"") pid=0;; esac; if [ "$pid" -gt 1 ] && [ -e "/proc/$pid/exe" ] && [ "$(readlink "/proc/$pid/exe")" = "/opt/beszel-agent/beszel-agent" ]; then kill "$pid" || true; fi; fi'
  if command -v setsid >/dev/null 2>&1; then
    run sh -c 'cd /opt/beszel-agent; setsid ./run.sh >agent.log 2>&1 </dev/null & echo $! >beszel-agent.pid'
  else
    run sh -c 'cd /opt/beszel-agent; nohup ./run.sh >agent.log 2>&1 </dev/null & echo $! >beszel-agent.pid'
  fi
fi

attempt=0
while [ "$attempt" -lt 8 ]; do
  if LISTEN="127.0.0.1:$PORT" "$BIN" health >/dev/null 2>&1; then
    exit 0
  fi
  attempt=$((attempt + 1))
  sleep 1
done

rollback
echo "agent failed its health check" >&2
exit 23
`
}
