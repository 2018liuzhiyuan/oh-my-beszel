# Setup and operations

[Back to README](../readme.md) · [简体中文](guide.zh-CN.md)

<a id="features"></a>

## Features

| Feature | Behavior |
| --- | --- |
| GPU overview | Utilization, VRAM usage, temperature, and available VRAM in the systems table. |
| Multi-GPU history | Aggregate and per-GPU charts, with stable series identifiers. |
| Available VRAM alerts | Compare the maximum available VRAM across GPUs; trigger after 20 qualifying samples, spaced at least one twentieth of the configured window apart. The first sample is immediate, so the earliest trigger is at 95% of that window. Changing the threshold or window restarts observation. |
| CPU state alerts | Configure I/O wait and steal time thresholds per system or across systems. Agents without usable CPU breakdown data retain their existing alert state. |
| SSH host management | Discover concrete hosts declared in the selected OpenSSH config, detect name conflicts, and import in batches. Discovery does not expand `Include`; connections use system OpenSSH for jump hosts and identity files. |
| Agent deployment | Upload and verify Linux amd64 / arm64 binaries over SSH; use systemd or a detached process. Native NVML collection is Linux amd64 with the `glibc` build tag. |
| Hardware information | Linux IPs, NICs, BIOS, BMC and IPMI SEL, subject to hardware, installed tools, and permissions. |
| Customizable overview | Reorder columns with mouse, touch, or keyboard; save visibility and order in the current browser. |
| Alerts and reconnection | Temporarily dismiss active alerts per user in the current browser; retry offline connections after 5 / 10 / 20 / 30 seconds. |

The table's **available VRAM** is the free memory on the GPU with the **largest total VRAM**. The **maximum available VRAM** alert compares **all GPUs**. These are different measurements.

Inherited features include system history, Docker / Podman monitoring, resource and status alerts, multiple users, OAuth / OIDC, backups, S.M.A.R.T., and systemd monitoring.

<a id="architecture"></a>

## What runs where

```mermaid
flowchart LR
    Browser[Browser] --> Hub[Hub / PocketBase]
    Config[OpenSSH config and keys] --> Hub
    Hub -->|Direct connection or SSH forwarding| Agent[Agent on each node]
    Agent --> Metrics[System / Containers / GPU]
```

| Component | Location and purpose |
| --- | --- |
| Hub | One Windows or Linux machine. Hosts the web UI, accounts, database, and alerts. |
| Agent | Every monitored node. Collects metrics and communicates with the Hub. |
| OpenSSH config and keys | On the Hub machine, accessible to the account running the Hub. Browser-side files are not used. |

Keep these ports separate: **8090** is the Hub web port, **45876** is the default Agent port, and **22** is the usual operating-system SSH port. SSH forwarding lets the Agent listen only on the node's loopback interface.

<a id="setup"></a>

## Setup

Choose one Hub path: [Windows](#windows) or [Linux / WSL](#linux). Then choose [SSH deployment](#ssh) or [manual Agent setup](#manual-agent). All build commands start in the repository root unless stated otherwise. Clone your own fork first; replace every example address, username, path, and password with your own values.

<a id="prerequisites"></a>

### Build prerequisites

- Go **1.26.1+**, as declared in [go.mod](../go.mod). Existing scripts set `GOEXPERIMENT=nojsonv2` for compatibility with PocketBase on Go 1.27.
- Bun for the frontend; the Linux bootstrap script pins **1.4.0**. Node.js **22.12+** is used by Vite and the npm build path.
- PowerShell **7** for Windows scripts; system **OpenSSH client** for SSH integration.
- Network access to dependency and toolchain downloads. The Linux script can bootstrap missing supported toolchains with pinned versions and SHA-256 checks.

These are build requirements. Running a prebuilt Hub or Agent does not require Go, Bun, or Node. The Windows launcher still needs PowerShell, and SSH integration still needs OpenSSH.

<a id="windows"></a>

### Windows: configure and start the Hub

**1. Build the portable directory** on Windows 10 / 11 x64. Build the frontend first so the package contains the current UI:

```powershell
bun install --cwd ./internal/site --frozen-lockfile
bun run --cwd ./internal/site build
pwsh -NoLogo -NoProfile -File ./deploy/windows/build.ps1
```

Output: `build/windows/`, including `beszel.exe`, `Monitor.exe`, scripts, `agents/`, and configuration templates. If `config.json` already exists, the build preserves it.

**2. Edit `build/windows/config.json` before the first start.** If it is absent, copy `config.example.json` to `config.json`. The following is a password-login example; replace the placeholder password:

```json
{
  "host": "127.0.0.1",
  "port": 8090,
  "sshConfigPath": "",
  "openBrowser": true,
  "startupTimeoutSeconds": 45,
  "tasks": ["Beszel Hub"],
  "hub": {
    "userEmail": "admin@example.com",
    "userPassword": "REPLACE_WITH_A_UNIQUE_PASSWORD",
    "autoLogin": "",
    "checkUpdates": false
  }
}
```

| Field | What to configure |
| --- | --- |
| `host` / `port` | Keep `127.0.0.1:8090` for local access. This is the Hub web address, not the Agent address. |
| `sshConfigPath` | Leave empty for discovery under the Hub user's home; or set an absolute path such as `C:/Users/operator/.ssh/config`. Forward slashes avoid JSON backslash escaping. |
| `hub.userEmail` / `hub.userPassword` | Create the initial dashboard user and PocketBase superuser when initializing a new database. Editing them later does **not** reset existing accounts. Change existing passwords through account management. |
| `hub.autoLogin` | Empty means normal authentication. An email enables passwordless access as that user for requests reaching the Hub. The shipped template has this enabled; clear it to use password login, especially before making the Hub accessible to others. |
| `tasks` | Scheduled tasks that Monitor starts; keep `["Beszel Hub"]` for the installation below. |
| `openBrowser` / `startupTimeoutSeconds` | Whether Monitor opens the browser, and how long it waits for the Hub to become ready. |
| `hub.checkUpdates` | Retained launcher field, passed as `CHECK_UPDATES`. It is not an installer or an update channel for this fork. |

**3. Register and start the task**, then open the dashboard:

```powershell
pwsh -NoLogo -NoProfile -File ./build/windows/install-task.ps1
Start-Process -FilePath ./build/windows/Monitor.exe
```

The task runs **when the current user logs on**, not before login at machine boot. `Monitor.exe` starts the configured tasks and opens the browser; it does not register a missing task. Open `http://127.0.0.1:8090` and sign in with your configured email and password.

For a foreground trial without registering a task, run this instead and open the URL manually; keep the terminal open:

```powershell
pwsh -NoLogo -NoProfile -File ./build/windows/run-hub.ps1
```

Configuration changes take effect after restarting the Hub. Avoid defining `AUTO_LOGIN` or `BESZEL_HUB_AUTO_LOGIN` in the launcher account's environment when password login is intended: an empty JSON field does not clear an inherited environment variable. Data is stored in `build/windows/beszel_data/`; logs are `hub.log` and `launcher.log` in the same package directory.

<a id="web-port"></a>

#### Change the Windows web port

1. Edit **`config.json` beside the `Monitor.exe` you actually run**. A fresh build uses `build/windows/config.json`; if you deployed the package elsewhere, edit that directory's copy. Change only the top-level `"port": 8090` to an unused port such as `"port": 8091`. Keep `"host": "127.0.0.1"` for local access and preserve the other settings. No rebuild is needed.
2. Restart the Hub. For the scheduled-task installation, run in PowerShell:

   ```powershell
   Stop-ScheduledTask -TaskName 'Beszel Hub'
   Start-ScheduledTask -TaskName 'Beszel Hub'
   ```

   Use your actual task name if customized. For a foreground `run-hub.ps1` session, press Ctrl+C and run the same script again. Merely reopening Monitor does not restart an already running task.
3. Open `http://127.0.0.1:8091`. Monitor reads the same configuration and opens the new address on its next launch. Update bookmarks and any proxy or tunnel that targets the old web port. Agent port `45876` and operating-system SSH ports do not need to change.

For Linux / WSL, change the port in both `serve --http 127.0.0.1:8091` and `APP_URL=http://127.0.0.1:8091` in the startup command or service configuration, then restart that Hub with the same data directory.

<a id="linux"></a>

### Linux / WSL: build and start the Hub

**1. Build in native Ubuntu:** this script installs dependencies, builds the frontend, runs checks, and creates binaries. You do not need to repeat the Windows frontend commands.

```bash
bash test/build-wsl.sh
```

For Ubuntu WSL, invoke it from Windows with:

```powershell
pwsh -NoLogo -NoProfile -NonInteractive -File ./test/run-wsl.ps1
```

Outputs are `build/linux/beszel`, `beszel-agent`, `beszel-agent-glibc`, and `sha256sums.txt`. The script is a build/test entry point, not an installer or a service manager. Linux amd64 was tested on real hardware; arm64 deployment support in the code is not equivalent to an arm64 runtime test.

**2. Start the Hub with an explicit persistent data directory** in a Linux terminal:

```bash
mkdir -p "$HOME/.local/share/oh-my-beszel"
export APP_URL="http://127.0.0.1:8090"
./build/linux/beszel serve --http 127.0.0.1:8090 \
  --dir "$HOME/.local/share/oh-my-beszel"
```

**3. Open `http://127.0.0.1:8090` and create the first account.** This assumes a new data directory and no preconfigured `USER_EMAIL` / `USER_PASSWORD`. Keep this terminal open; Ctrl+C stops the foreground Hub. Use the same `--dir` on every start to retain accounts, keys, and history.

The Linux Hub does **not** read the Windows `config.json`. Configure it using CLI flags and environment variables before starting it:

| Setting | Purpose |
| --- | --- |
| `serve --http` | Listening interface and web port. |
| `--dir` | Persistent Hub data directory; use a fixed path. |
| `APP_URL` | URL users/Agents can use to reach the Hub. It does not change the listening address. |
| `SSH_CONFIG_PATH` | OpenSSH config path on the Hub machine; see [SSH setup](#ssh). |
| `BESZEL_AGENT_DEPLOY_DIR` | Directory containing locally built Agents for SSH deployment. |
| `USER_EMAIL` / `USER_PASSWORD` | Optional initial credentials for a new database; omit to create the first account in the UI. |
| `AUTO_LOGIN` | Omit for normal authentication; setting an email enables passwordless access as that user. |

These shell exports only apply to that process environment. For unattended use, configure a service supervisor with the same executable, account, data path, and environment. This repository does not yet ship a Linux **Hub** systemd installer.

<a id="remote-access"></a>

### Accessing a Hub on another machine

`127.0.0.1` always means the machine making the connection. For a Hub on a remote server, keep the loopback listener and forward it from your computer (replace `hub-server` with your SSH host):

```bash
ssh -N -L 8090:127.0.0.1:8090 hub-server
```

Keep the tunnel open and browse to `http://127.0.0.1:8090` locally. If serving other users over a LAN or reverse proxy, configure the listener/firewall and the external `APP_URL`, use HTTPS at the proxy, and disable auto-login. An Agent on another machine cannot use the Hub's loopback URL for an outbound connection.

<a id="ssh"></a>

## Connect nodes through SSH deployment

Importing an SSH host can **install or replace an Agent on that node**. Hosts with an SSH config and a pending/down status are eligible for automatic deployment, including after Hub startup. Use this path for nodes you intend this Hub to manage.

**1. Prepare OpenSSH on the Hub machine**, under the same account that runs the Hub. Put a concrete alias in its SSH config:

```sshconfig
Host gpu-node-a
    HostName 192.0.2.10
    User operator
    Port 22
    IdentityFile ~/.ssh/id_ed25519
```

`192.0.2.10` is a documentation-only address. Replace it, the username, and the key path. Configure `ProxyJump` if required. Confirm the host fingerprint on your initial SSH connection, then verify noninteractive access. A passphrase-protected key must already be available through an SSH agent accessible to the Hub process:

```bash
ssh -F ~/.ssh/config -o BatchMode=yes gpu-node-a "uname -s; uname -m"
ssh -F ~/.ssh/config -o BatchMode=yes gpu-node-a "id -u; sudo -n true"
```

The target must be Linux amd64 or arm64. Deployment requires root, or a user for whom `sudo -n true` succeeds without a password. A root login does not need sudo even if the second command reports it missing. Normal password-based SSH login is insufficient for the background deployment process.

**2. Point the Hub to this config.** On Windows, set `sshConfigPath` in the package's `config.json`. On Linux, set this before restarting the Hub:

```bash
export SSH_CONFIG_PATH="$HOME/.ssh/config"
```

**3. Prepare the matching Agent artifact on the Hub.** Windows packaging already creates `agents/beszel-agent_linux_amd64` and `agents/beszel-agent_linux_arm64`. For a Linux amd64 build, prepare the directory explicitly:

```bash
mkdir -p ./build/linux/agents
cp ./build/linux/beszel-agent ./build/linux/agents/beszel-agent_linux_amd64
export BESZEL_AGENT_DEPLOY_DIR="$(pwd)/build/linux/agents"
```

For Linux amd64 NVIDIA nodes with glibc and the NVIDIA driver/NVML library, replace that artifact with the NVML build before deployment:

```bash
cp ./build/linux/beszel-agent-glibc ./build/linux/agents/beszel-agent_linux_amd64
```

Artifact lookup order is `BESZEL_AGENT_DEPLOY_DIR`, `agents/` beside the Hub executable, then `agents/` inside the Hub data directory. Filenames must be exactly `beszel-agent_linux_amd64` or `beszel-agent_linux_arm64`, matching the target. Renaming an amd64 binary does not make it an arm64 binary. The deployment code does not select a separate `_glibc` filename automatically.

**4. In the dashboard, open Add System → SSH.** Check the displayed config path and reload after changing it. Select hosts, keep Agent port `45876` unless you need another, then import. This field is **not** the operating-system SSH port. A read-only account cannot manage these settings.

The Hub uploads the local artifact, verifies SHA-256, and installs it under `/opt/beszel-agent/`. On systemd nodes it enables and starts `beszel-agent.service`; otherwise it starts a detached process, without a reboot-start guarantee. The Agent listens on `127.0.0.1:45876` by default for this path, and the Hub connects through `ssh -W`. Allow SSH TCP forwarding on the node; the Agent port need not be exposed publicly.

**5. Confirm the node becomes online** and CPU/memory charts update. On the node, check the service and Agent health as needed:

```bash
systemctl status beszel-agent --no-pager
LISTEN=127.0.0.1:45876 /opt/beszel-agent/beszel-agent health
```

The `systemctl` command applies only to systemd nodes. For NVIDIA nodes, also check `nvidia-smi` on the node and verify GPU count and VRAM capacity in the dashboard.

<a id="manual-agent"></a>

## Manual Agent setup with a direct connection

Use this path when you want to manage Agent installation yourself. The example is a **Hub → Agent** connection over a private network, with no SSH config attached to the system.

1. Copy a matching Agent built from this repository to the monitored node. For Linux amd64 use `build/linux/beszel-agent`, or `beszel-agent-glibc` for native NVML on supported NVIDIA nodes, and name the copied file `beszel-agent`.
2. In **Add System → Binary**, enter a name, the node's reachable **IP address**, and Agent port `45876`. Copy the **Hub public key** shown in the form and save the system. This public key is different from your operating-system SSH private key.
3. On the monitored node, replace the public-key placeholder below and run from the directory containing the Agent:

```bash
chmod +x ./beszel-agent
mkdir -p "$HOME/.local/share/oh-my-beszel-agent"
export DATA_DIR="$HOME/.local/share/oh-my-beszel-agent"
export LISTEN="0.0.0.0:45876"
export KEY="REPLACE_WITH_HUB_PUBLIC_KEY"
./beszel-agent
```

Restrict access to port `45876` to the Hub in the node's firewall, or bind `LISTEN` to a private interface. Keep the terminal open for this trial; configure your service manager for persistent operation. `KEY_FILE` can be used instead of `KEY` to read the Hub public key from a file.

`TOKEN` and `HUB_URL` are used for an Agent-initiated connection; they are not needed for the direct Hub-initiated example above. If using that alternative, take the token from the matching system in the dashboard and ensure `HUB_URL` is reachable **from the node**, rather than copying an inaccessible localhost URL.

The UI's inherited “Copy Linux command” and Docker buttons still reference upstream installation scripts/images. To run this fork's Agent changes, use the local binaries above or SSH deployment instead of assuming those buttons install this fork.

<a id="operations"></a>

## Data, restart, and upgrades

| Item | Location / action |
| --- | --- |
| Windows Hub data | `beszel_data/` inside the portable directory. |
| Linux Hub data | The directory passed to `--dir`; without it, the default is `beszel_data` relative to the working directory. |
| Manual Agent data | The `DATA_DIR` used above. Retain it to preserve Agent identity. |
| SSH-installed Agent | `/opt/beszel-agent/`; systemd journal, or `agent.log` for detached operation. |
| Windows restart | Restart the `Beszel Hub` task after editing config; stop/start the foreground process if no task is used. |
| Backup / migration | Stop the Hub before a filesystem copy of its complete data directory; also preserve your configuration and SSH setup. |
| Upgrade | Back up, stop the old process, replace it with this fork's new binary/package, then start with the same data directory and verify nodes. Do not overwrite your local config with the example. |

Removing the Windows task with `uninstall-task.ps1` unregisters it; it does not delete data or guarantee the running Hub has stopped. Before moving or deleting a package, stop its specific running instance.

Keep real passwords, tokens, SSH private keys, databases, and logs outside Git. The repository already ignores build directories and test evidence.

<a id="troubleshooting"></a>

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Monitor fails on the first start | Register the task with `install-task.ps1` first, or use `run-hub.ps1` directly. Check `launcher.log` and `hub.log`. |
| Login ignores the password / changing JSON password has no effect | Clear auto-login and inherited auto-login variables, restart the Hub; use account management for existing passwords. |
| Remote browser cannot open the Hub | Loopback is local to each machine. Use the SSH tunnel above, or configure the listener and firewall for remote access. |
| No SSH hosts appear | Check the path **on the Hub**, read permissions for its service account, concrete `Host` aliases, and reload after path changes. |
| SSH works interactively but deployment fails | Test `BatchMode=yes` as the Hub account; verify key/SSH-agent access and root or passwordless sudo. |
| Agent artifact not found / wrong executable format | Check the lookup directories, exact filename, and actual binary architecture. |
| Imported node stays pending/down | Check Hub deployment logs, Agent service health, port consistency, and SSH TCP forwarding. |
| Manual node stays offline | Verify node IP, firewall, `LISTEN`, and that `KEY` is the public key of this Hub. |
| GPU data is missing | Verify `nvidia-smi`, driver permissions, binary choice, and NVML availability for the glibc build. |
| Accounts/history seem lost after restart | Check that the same Hub data directory and running account are in use. |

<a id="development"></a>

## Development and verification

```powershell
pwsh -NoLogo -NoProfile -NonInteractive -File ./test/run.ps1
```

The strict Windows runner covers Go compilation, vet, shuffled uncached tests, coverage, race, two fuzz targets, six performance budgets, platform builds, frontend unit tests, TypeScript, Biome, and production build. `-Quick` skips race and active fuzzing.

Browser and live-GPU tests have separate entry points and environment requirements. Cross-compilation is not a runtime test. See the [test guide](../test/README.md) (currently in Chinese) for commands, results, and limits. Evidence lives in ignored `test/artifacts/` and `test/reports/`.


<a id="layout"></a>

## Repository layout

```text
agent/
internal/hub/
internal/alerts/
internal/site/
internal/cmd/
deploy/windows/
test/
docs/assets/
```

These contain node collection, Hub APIs/deployment, alerts, frontend, executable entry points, Windows packaging, tests, and branding assets respectively.

<a id="publishing"></a>

## Naming and publishing

The project/repository name is **oh-my-beszel**. The Go module `github.com/henrygd/beszel`, binary names `beszel` / `beszel-agent`, environment variables, and data directory conventions remain compatible. This does not imply an upstream release.

GoReleaser is configured for binary archives and a draft Release, without publishing to upstream Homebrew, Scoop, or WinGet repositories. Set Git remote to your own repository and validate the frontend and target platforms before publishing.

Before committing, inspect the files Git will include:

```powershell
git status --short
git ls-files -ci --exclude-standard
git diff --check
git diff --cached --check
```

The second command should produce no output. Commit source, examples, and tests; retain local configuration, credentials, runtime data, and generated evidence locally.

The English and Simplified Chinese READMEs have matching sections, commands, examples, and configuration semantics. Update both in the same change when behavior or setup instructions change.

<a id="license"></a>

## Credits and license

Thanks to [Beszel](https://github.com/henrygd/beszel), its contributors, PocketBase, and the open-source dependencies used here. [Upstream documentation](https://beszel.dev) covers general Beszel usage; fork-specific deployment behavior is documented above.

Licensed under [MIT](../LICENSE), retaining upstream copyright notices. The all-seeing-eye logo was made with the built-in image generation tool; its prompt is recorded in [brand assets](assets/README.md) (Chinese).
