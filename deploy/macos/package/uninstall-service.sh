#!/bin/sh

set -eu

LABEL="com.oh-my-beszel.hub"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
DOMAIN="gui/$(id -u)"

launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
rm -f "$PLIST"

echo "Removed $LABEL. Hub data was kept."
