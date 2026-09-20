#!/bin/sh
set -eu
PACKAGE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if ! /bin/sh "$PACKAGE_DIR/install-service.sh"; then
  echo "Startup failed. Read the message above or QUICKSTART.zh-CN.md."
  printf "Press Enter to close... "
  read -r REPLY || true
  exit 1
fi
case "$(uname -m)" in arm64) ARCH=arm64 ;; *) ARCH=amd64 ;; esac
. "$HOME/Applications/oh-my-beszel-darwin-$ARCH/config.env"
open "${APP_URL:-http://127.0.0.1:8090}"
