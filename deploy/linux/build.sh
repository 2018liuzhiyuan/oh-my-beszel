#!/bin/sh
# Builds the Linux (amd64) release tarball on a Linux host. Twin of
# build.ps1, which does the same from Windows via cross-compilation; both
# produce build/oh-my-beszel_<tag>_linux_amd64.tar.gz with identical layout.
#
# Modes are forced explicitly (tar --mode), never taken from the filesystem:
# on drvfs/NTFS mounts every file can appear 0777 or 0644, and trusting that
# is exactly how the om.3 launch shipped non-executable binaries.
#
# Usage: deploy/linux/build.sh [tag]   (default tag: dev)

set -eu

TAG="${1:-dev}"
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
STAGE="$REPO_ROOT/build/linux"
TOP="oh-my-beszel-linux-amd64"
TAR_PATH="$REPO_ROOT/build/oh-my-beszel_${TAG}_linux_amd64.tar.gz"

command -v go >/dev/null 2>&1 || { echo "go not found in PATH" >&2; exit 1; }

export GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64
# PocketBase 0.36 recursively re-enters Collection.UnmarshalJSON under the
# Go 1.27 json/v2 implementation
export GOEXPERIMENT=nojsonv2

mkdir -p "$STAGE"
cd "$REPO_ROOT"
go build -trimpath -buildvcs=false -ldflags '-s -w' -o "$STAGE/beszel" ./internal/cmd/hub
go build -trimpath -buildvcs=false -ldflags '-s -w' -o "$STAGE/beszel-agent" ./internal/cmd/agent
go build -trimpath -buildvcs=false -tags glibc -ldflags '-s -w' -o "$STAGE/beszel-agent-glibc" ./internal/cmd/agent

cp "$SCRIPT_DIR/README.txt" "$STAGE/README.txt"

{
	echo "run_id=sh-$(date -u +%Y%m%d-%H%M%S)"
	echo "built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	echo "distribution=$( (. /etc/os-release 2>/dev/null && echo "$PRETTY_NAME") || uname -s )"
	go version
	echo "goos=linux"
	echo "goarch=amd64"
	echo "tag=$TAG"
} > "$STAGE/build-info.txt"

(cd "$STAGE" && sha256sum beszel beszel-agent beszel-agent-glibc > sha256sums.txt)

WORK_TAR="$REPO_ROOT/build/.pkg-$TAG.tar"
rm -f "$WORK_TAR" "$TAR_PATH"
tar --owner=0 --group=0 --numeric-owner --mode=0755 \
	--transform "s,^,$TOP/," -cf "$WORK_TAR" -C "$STAGE" \
	beszel beszel-agent beszel-agent-glibc
tar --owner=0 --group=0 --numeric-owner --mode=0644 \
	--transform "s,^,$TOP/," -rf "$WORK_TAR" -C "$STAGE" \
	README.txt build-info.txt sha256sums.txt
gzip -n -9 "$WORK_TAR"
mv "$WORK_TAR.gz" "$TAR_PATH"

tar -tvzf "$TAR_PATH"
echo "Linux package ready: $TAR_PATH"
