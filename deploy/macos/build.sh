#!/bin/sh
# Build a self-contained macOS release archive. The Hub and local Agent are
# built for the requested Mac architecture; Linux Agents are bundled so a Hub
# running on macOS can keep using oh-my-beszel's SSH deployment workflow.
#
# Usage: deploy/macos/build.sh [tag] [arm64|amd64|all]
# Defaults: tag=dev, arch=current Mac architecture

set -eu

TAG="${1:-dev}"
REQUESTED_ARCH="${2:-$(uname -m)}"
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
SITE_DIR="$REPO_ROOT/internal/site"

case "$REQUESTED_ARCH" in
	arm64|aarch64) ARCHES="arm64" ;;
	amd64|x86_64) ARCHES="amd64" ;;
	all) ARCHES="arm64 amd64" ;;
	*) echo "unsupported macOS architecture: $REQUESTED_ARCH (use arm64, amd64, or all)" >&2; exit 1 ;;
esac

command -v go >/dev/null 2>&1 || { echo "go not found in PATH (Go 1.26.1+ is required)" >&2; exit 1; }
command -v shasum >/dev/null 2>&1 || { echo "shasum not found in PATH" >&2; exit 1; }

if [ "${SKIP_WEB:-0}" != "1" ]; then
	if command -v bun >/dev/null 2>&1; then
		bun install --cwd "$SITE_DIR" --frozen-lockfile
		bun run --cwd "$SITE_DIR" build
	elif command -v npm >/dev/null 2>&1; then
		npm ci --prefix "$SITE_DIR"
		npm run --prefix "$SITE_DIR" build
	else
		echo "bun or npm is required to build the web UI (or set SKIP_WEB=1 when internal/site/dist already exists)" >&2
		exit 1
	fi
elif [ ! -f "$SITE_DIR/dist/index.html" ]; then
	echo "SKIP_WEB=1 was set, but internal/site/dist/index.html does not exist" >&2
	exit 1
fi

export GOTOOLCHAIN=local
export CGO_ENABLED=0
export GOEXPERIMENT=nojsonv2
export GOCACHE="${GOCACHE:-$REPO_ROOT/.tmp/go-build-cache}"

build_one() {
	ARCH="$1"
	TOP="oh-my-beszel-darwin-$ARCH"
	STAGE_PARENT="$REPO_ROOT/build/macos/$ARCH"
	STAGE="$STAGE_PARENT/$TOP"
	TAR_PATH="$REPO_ROOT/build/oh-my-beszel_${TAG}_darwin_${ARCH}.tar.gz"

	rm -rf "$STAGE"
	mkdir -p "$STAGE/agents" "$STAGE/logs"

	cd "$REPO_ROOT"
	GOOS=darwin GOARCH="$ARCH" go build -trimpath -buildvcs=false -ldflags '-s -w' -o "$STAGE/beszel" ./internal/cmd/hub
	GOOS=darwin GOARCH="$ARCH" go build -trimpath -buildvcs=false -ldflags '-s -w' -o "$STAGE/beszel-agent" ./internal/cmd/agent
	GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags '-s -w' -o "$STAGE/agents/beszel-agent_linux_amd64" ./internal/cmd/agent
	GOOS=linux GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags '-s -w' -o "$STAGE/agents/beszel-agent_linux_arm64" ./internal/cmd/agent

	cp "$SCRIPT_DIR/package/README.txt" "$STAGE/README.txt"
	cp "$SCRIPT_DIR/package/QUICKSTART.zh-CN.md" "$STAGE/QUICKSTART.zh-CN.md"
	cp "$SCRIPT_DIR/package/Open.command" "$STAGE/Open.command"
	cp "$SCRIPT_DIR/package/config.env.example" "$STAGE/config.env.example"
	cp "$SCRIPT_DIR/package/start.sh" "$STAGE/start.sh"
	cp "$SCRIPT_DIR/package/install-service.sh" "$STAGE/install-service.sh"
	cp "$SCRIPT_DIR/package/uninstall-service.sh" "$STAGE/uninstall-service.sh"
	cp "$SCRIPT_DIR/package/com.oh-my-beszel.hub.plist" "$STAGE/com.oh-my-beszel.hub.plist"
	chmod 0755 "$STAGE/beszel" "$STAGE/beszel-agent" "$STAGE/agents/"* "$STAGE/"*.sh "$STAGE/Open.command"
	chmod 0644 "$STAGE/README.txt" "$STAGE/config.env.example" "$STAGE/com.oh-my-beszel.hub.plist"

	{
		echo "run_id=sh-$(date -u +%Y%m%d-%H%M%S)"
		echo "built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
		echo "distribution=$(sw_vers -productName) $(sw_vers -productVersion)"
		go version
		echo "goos=darwin"
		echo "goarch=$ARCH"
		echo "tag=$TAG"
	} > "$STAGE/build-info.txt"

	(
		cd "$STAGE"
		shasum -a 256 beszel beszel-agent \
			agents/beszel-agent_linux_amd64 agents/beszel-agent_linux_arm64 \
			Open.command start.sh install-service.sh uninstall-service.sh \
			config.env.example com.oh-my-beszel.hub.plist README.txt QUICKSTART.zh-CN.md build-info.txt \
			> sha256sums.txt
	)

	rm -f "$TAR_PATH"
	COPYFILE_DISABLE=1 tar -czf "$TAR_PATH" -C "$STAGE_PARENT" "$TOP"
	tar -tzf "$TAR_PATH"
	echo "macOS package ready: $TAR_PATH"
}

for ARCH in $ARCHES; do
	build_one "$ARCH"
done
