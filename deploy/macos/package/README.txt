oh-my-beszel for macOS — Quick start

For packages containing Open.command:
1. Download darwin_arm64 for Apple Silicon or darwin_amd64 for Intel.
2. Extract the archive and double-click Open.command.
3. Wait for the browser, then create an account or sign in.

No Go, Bun, Node.js, or manual configuration is needed to run this package.
The launcher checks the included checksums, installs under ~/Applications,
keeps the existing service's config.env, registers login startup, and opens
the browser only after checking the service and HTTP health endpoint.
You can close the terminal after it reports success.

macOS may block this unsigned/unnotarized download. Verify its trusted release
origin and archive SHA-256 first. In Terminal, type "cd " and drag the extracted
folder into the window, press Return, then run:
  xattr -dr com.apple.quarantine .
  /bin/sh ./Open.command
No sudo or system-wide security changes are needed.

Daily use: http://127.0.0.1:8090 (default). The Hub starts when you log in.
Configuration: edit config.env in the installed folder, then run Open.command.
Data: ~/Library/Application Support/oh-my-beszel by default. If overriding the
path, use a stable absolute path. Do not run two Hubs against the same database.
Upgrade: double-click Open.command from the new package. The previous installed
folder is backed up under ~/Applications/oh-my-beszel-backup.*; config is kept.
Logs: logs/hub-error.log in the installed folder. If the port is busy, stop any
foreground Hub with Ctrl+C and retry.
Stop and remove login startup: run ./uninstall-service.sh in the installed
folder. Data is kept. Open.command enables startup again.

Linux amd64/arm64 Agents are bundled for SSH deployment. The native macOS Agent
can also be started manually with your Hub public key in KEY, a suitable LISTEN
address, and DATA_DIR. SSH auto-deployment targets Linux, not macOS.

Chinese instructions: QUICKSTART.zh-CN.md
