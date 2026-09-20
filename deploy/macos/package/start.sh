#!/bin/sh

set -eu

PACKAGE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
CONFIG_FILE="$PACKAGE_DIR/config.env"

if [ -f "$CONFIG_FILE" ]; then
	set -a
	# This file is user-owned shell configuration; keep it readable only by the user
	# if it contains initial credentials.
	. "$CONFIG_FILE"
	set +a
fi

: "${BESZEL_HTTP:=127.0.0.1:8090}"
: "${BESZEL_DATA_DIR:=$HOME/Library/Application Support/oh-my-beszel}"
: "${APP_URL:=http://127.0.0.1:8090}"
: "${BESZEL_AGENT_DEPLOY_DIR:=$PACKAGE_DIR/agents}"

export APP_URL BESZEL_AGENT_DEPLOY_DIR
mkdir -p "$BESZEL_DATA_DIR" "$PACKAGE_DIR/logs"

exec "$PACKAGE_DIR/beszel" serve --http "$BESZEL_HTTP" --dir "$BESZEL_DATA_DIR"
