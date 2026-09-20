#!/bin/sh
# Install into a location launchd can read, then verify this service is healthy.
set -eu
LABEL="com.oh-my-beszel.hub"
SOURCE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
DOMAIN="gui/$(id -u)"
case "$(uname -m)" in
  arm64) ARCH=arm64 ;;
  x86_64) ARCH=amd64 ;;
  *) echo "Unsupported Mac architecture" >&2; exit 1 ;;
esac
PACKAGE_DIR="$HOME/Applications/oh-my-beszel-darwin-$ARCH"

# Detect a wrong download before changing the installed service.
if ! file "$SOURCE_DIR/beszel" | grep -q "$(uname -m)"; then
  echo "Wrong package architecture. Download darwin_$ARCH for this Mac." >&2
  exit 1
fi
(cd "$SOURCE_DIR" && shasum -a 256 -c sha256sums.txt)

OLD_PROGRAM=""
if [ -f "$PLIST" ]; then
  OLD_PROGRAM=$(plutil -extract Program raw -o - "$PLIST" 2>/dev/null || true)
fi
OLD_DIR=${OLD_PROGRAM%/*}
mkdir -p "$HOME/Applications" "$HOME/Library/LaunchAgents"

if [ "$SOURCE_DIR" != "$PACKAGE_DIR" ]; then
  # Stage before stopping the old service. Copy only distribution files, never
  # a user's logs or database. Keep the previous installed folder as a backup.
  STAGE=$(mktemp -d "$HOME/Applications/.oh-my-beszel-install.XXXXXX")
  for item in beszel beszel-agent agents start.sh install-service.sh uninstall-service.sh Open.command config.env.example com.oh-my-beszel.hub.plist README.txt QUICKSTART.zh-CN.md sha256sums.txt build-info.txt; do
    cp -R "$SOURCE_DIR/$item" "$STAGE/"
  done
  if [ -n "$OLD_PROGRAM" ] && [ -f "$OLD_DIR/config.env" ]; then
    cp "$OLD_DIR/config.env" "$STAGE/config.env"
  elif [ -f "$PACKAGE_DIR/config.env" ]; then
    cp "$PACKAGE_DIR/config.env" "$STAGE/config.env"
  elif [ -f "$SOURCE_DIR/config.env" ]; then
    cp "$SOURCE_DIR/config.env" "$STAGE/config.env"
  fi
  launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
  if [ -e "$PACKAGE_DIR" ]; then
    BACKUP=$(mktemp -d "$HOME/Applications/oh-my-beszel-backup.XXXXXX")
    mv "$PACKAGE_DIR" "$BACKUP/package"
    echo "Previous installation saved to: $BACKUP/package"
  fi
  mv "$STAGE" "$PACKAGE_DIR"
fi

cd "$PACKAGE_DIR"
if [ ! -f config.env ]; then
  cp config.env.example config.env
fi
chmod 600 config.env
set -a
. ./config.env
set +a
: "${BESZEL_HTTP:=127.0.0.1:8090}"
: "${APP_URL:=http://127.0.0.1:8090}"
case "$BESZEL_HTTP" in
  :*) HEALTH_ADDRESS="127.0.0.1$BESZEL_HTTP" ;;
  0.0.0.0:*) HEALTH_ADDRESS="127.0.0.1:${BESZEL_HTTP##*:}" ;;
  \[::\]:*) HEALTH_ADDRESS="[::1]:${BESZEL_HTTP##*:}" ;;
  *) HEALTH_ADDRESS="$BESZEL_HTTP" ;;
esac
mkdir -p logs
launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
cp com.oh-my-beszel.hub.plist "$PLIST"
plutil -replace Program -string "$PACKAGE_DIR/start.sh" "$PLIST"
plutil -replace WorkingDirectory -string "$PACKAGE_DIR" "$PLIST"
plutil -replace StandardOutPath -string "$PACKAGE_DIR/logs/hub.log" "$PLIST"
plutil -replace StandardErrorPath -string "$PACKAGE_DIR/logs/hub-error.log" "$PLIST"
plutil -lint "$PLIST"
# Enable before bootstrap: a previously disabled job must be enabled to load.
launchctl enable "$DOMAIN/$LABEL"
launchctl bootstrap "$DOMAIN" "$PLIST"

# Do not mistake an unrelated server occupying the port for our healthy Hub.
ATTEMPT=0
while [ "$ATTEMPT" -lt 30 ]; do
  SERVICE_PID=$(launchctl print "$DOMAIN/$LABEL" 2>/dev/null | awk '/^[[:space:]]*pid = / {print $3; exit}')
  if [ -n "$SERVICE_PID" ] &&
     lsof -nP -a -p "$SERVICE_PID" -iTCP:"${BESZEL_HTTP##*:}" -sTCP:LISTEN >/dev/null 2>&1 &&
     curl --noproxy '*' --max-time 2 -fsS "http://$HEALTH_ADDRESS/api/health" >/dev/null 2>&1; then
    echo "Hub is ready: $APP_URL"
    echo "Installed at: $PACKAGE_DIR"
    echo "You can close this terminal. The Hub starts at login."
    exit 0
  fi
  ATTEMPT=$((ATTEMPT + 1))
  sleep 1
done
echo "Hub did not become ready. See $PACKAGE_DIR/logs/hub-error.log" >&2
tail -n 15 "$PACKAGE_DIR/logs/hub-error.log" >&2 || true
echo "If another foreground Hub uses this port, stop it with Ctrl+C and try again." >&2
exit 1
