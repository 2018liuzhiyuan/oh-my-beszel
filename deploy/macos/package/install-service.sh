#!/bin/sh

set -eu

LABEL="com.oh-my-beszel.hub"
PACKAGE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
LAUNCH_AGENTS_DIR="$HOME/Library/LaunchAgents"
PLIST="$LAUNCH_AGENTS_DIR/$LABEL.plist"
DOMAIN="gui/$(id -u)"

command -v plutil >/dev/null 2>&1 || { echo "plutil not found" >&2; exit 1; }
mkdir -p "$LAUNCH_AGENTS_DIR" "$PACKAGE_DIR/logs"
cp "$PACKAGE_DIR/$LABEL.plist" "$PLIST"
plutil -replace Program -string "$PACKAGE_DIR/start.sh" "$PLIST"
plutil -replace WorkingDirectory -string "$PACKAGE_DIR" "$PLIST"
plutil -replace StandardOutPath -string "$PACKAGE_DIR/logs/hub.log" "$PLIST"
plutil -replace StandardErrorPath -string "$PACKAGE_DIR/logs/hub-error.log" "$PLIST"
plutil -lint "$PLIST"

launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
launchctl bootstrap "$DOMAIN" "$PLIST"
launchctl enable "$DOMAIN/$LABEL"
launchctl kickstart -k "$DOMAIN/$LABEL"

echo "Installed and started $LABEL"
echo "Dashboard: ${APP_URL:-http://127.0.0.1:8090}"
echo "Logs: $PACKAGE_DIR/logs"
