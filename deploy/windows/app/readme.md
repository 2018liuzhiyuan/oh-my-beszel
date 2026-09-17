# oh-my-beszel — Windows portable package

Server-monitoring hub for GPU clusters. Single folder, no installer, no
dependencies beyond stock Windows 10/11 x64. Everything the dashboard needs
is inside this package.

## Contents

| Path | Purpose |
|---|---|
| `Monitor.exe` | Launcher: starts the hub, opens the dashboard, manages autostart |
| `config.json` | The only file you may want to edit (see below) |
| `app\` | The hub itself (`beszel.exe`, Linux agents for SSH deployment) — no need to touch |

Runtime files appear next to their owners: `launcher.log` at the root,
`app\hub.log` and the database `app\beszel_data\` inside `app\`.

## Quick start

1. Unzip anywhere (a local disk path is best; avoid network shares).
2. Optional but recommended: edit `config.json` and set your own
   `hub.userEmail` / `hub.userPassword` (they create the admin account on
   the very first start; later changes have no effect — use the web UI).
   `hub.autoLogin` set to the same email enables passwordless login;
   set it to `""` to require the password.
3. Double-click `Monitor.exe`. The hub starts in the background, the
   dashboard opens at `http://127.0.0.1:8090`, and a logon task named
   after `tasks[0]` (default `Beszel Hub`) is registered so the hub starts
   automatically from the next sign-in.

To remove the autostart entry, run `Monitor.exe uninstall-task` from a terminal
(data is kept; a running hub keeps running until sign-out).

## config.json reference

| Key | Default | Meaning |
|---|---|---|
| `host` / `port` | `127.0.0.1` / `8090` | Dashboard listen address. The dashboard is only reachable from this machine unless you change `host` |
| `openBrowser` | `true` | Open the dashboard after a successful start |
| `startupTimeoutSeconds` | `45` | How long Monitor waits for the dashboard before reporting failure |
| `tasks` | `["Beszel Hub"]` | Scheduled task(s) Monitor starts; the first name is also used when registering autostart |
| `sshConfigPath` | `""` | Path to an OpenSSH config; enables importing monitored systems from it |
| `hub.userEmail` / `hub.userPassword` | placeholders | Admin account, applied on first start only |
| `hub.autoLogin` | same email | Passwordless login for that account (`""` disables) |
| `hub.checkUpdates` | `false` | Upstream update checks |
| `hub.logLevel` | `info` | `debug` / `info` / `warn` / `error` for `app\hub.log` |

## Changing the port

Edit `port` in `config.json`, then double-click `Monitor.exe` again — it
notices the hub is serving the old port, restarts its scheduled task so the
new config is picked up, and opens the new address. Note: each install on a
machine needs its own task name; if you run two copies, give the second one
a different `tasks[0]` in its config.json before first launch.

## Monitoring Linux machines

Set `sshConfigPath` to your OpenSSH config and add systems from the
dashboard (quick-add lists the hosts). The hub uploads and starts the agent
from `app\agents\` over SSH by itself — root or passwordless sudo on the
target is required. Agents are restarted automatically if they die.

## Troubleshooting

- Startup problems: read `launcher.log` (root) and `app\hub.log`. Failures
  also show a message box.
- Something already uses the port: change `port` in `config.json`.
- Offline systems: hover the red status dot, open the system page, or check
  Settings → Hub Logs in the dashboard — all three show the SSH error text.
- Verify a download: compare against `SHA256SUMS.txt` on the release page.
