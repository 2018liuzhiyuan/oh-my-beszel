oh-my-beszel macOS portable package

This package runs natively on the architecture named in the archive
(darwin_arm64 for Apple Silicon or darwin_amd64 for Intel). It includes a
macOS Hub, a macOS Agent, and Linux amd64/arm64 Agents for SSH deployment.

Quick start - Hub in the foreground:
  cp config.env.example config.env
  # Optional: edit config.env
  ./start.sh
  open http://127.0.0.1:8090

The first visit creates the admin account unless USER_EMAIL and USER_PASSWORD
are set before the first start. Hub data defaults to:
  ~/Library/Application Support/oh-my-beszel

Start automatically when this user logs in:
  ./install-service.sh

Stop and remove login startup (data is kept):
  ./uninstall-service.sh

Logs for the launchd service are in this package's logs/ directory. Keep the
package at a stable path while the service is installed. After moving it, run
install-service.sh again so launchd receives the new absolute path.

Quick start - monitor this Mac with the bundled Agent:
  KEY='<hub public key from Add System>' \
  LISTEN=127.0.0.1:45876 \
  DATA_DIR="$HOME/Library/Application Support/oh-my-beszel-agent" \
  ./beszel-agent

The Hub's SSH auto-deployment supports Linux targets. The required Linux
amd64 and arm64 binaries are bundled in agents/ and selected automatically.

Verify package binaries:
  shasum -a 256 -c sha256sums.txt

If macOS reports that a downloaded binary cannot be opened, verify that the
archive came from a trusted release and that its checksum matches, then remove
the quarantine attribute from the extracted package:
  xattr -dr com.apple.quarantine .
