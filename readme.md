# Beszel

Beszel is a lightweight server monitoring platform that includes Docker statistics, historical data, and alert functions.

It has a friendly web interface, simple configuration, and is ready to use out of the box. It supports automatic backup, multi-user, OAuth authentication, and API access.

[![agent Docker Image Size](https://img.shields.io/docker/image-size/henrygd/beszel-agent/latest?logo=docker&label=agent%20image%20size)](https://hub.docker.com/r/henrygd/beszel-agent)
[![hub Docker Image Size](https://img.shields.io/docker/image-size/henrygd/beszel/latest?logo=docker&label=hub%20image%20size)](https://hub.docker.com/r/henrygd/beszel)
[![MIT license](https://img.shields.io/github/license/henrygd/beszel?color=%239944ee)](https://github.com/henrygd/beszel/blob/main/LICENSE)
[![Crowdin](https://badges.crowdin.net/beszel/localized.svg)](https://crowdin.com/project/beszel)

![Screenshot of Beszel dashboard and system page, side by side. The dashboard shows metrics from multiple connected systems, while the system page shows detailed metrics for a single system.](https://henrygd-assets.b-cdn.net/beszel/screenshot-new.png)

## Features

- **Lightweight**: Smaller and less resource-intensive than leading solutions.
- **Simple**: Easy setup with little manual configuration required.
- **Docker stats**: Tracks CPU, memory, and network usage history for each container.
- **Alerts**: Configurable alerts for CPU, memory, disk, bandwidth, temperature, load average, and status.
- **Multi-user**: Users manage their own systems. Admins can share systems across users.
- **OAuth / OIDC**: Supports many OAuth2 providers. Password auth can be disabled.
- **Automatic backups**: Save to and restore from disk or S3-compatible storage.
<!-- - **REST API**: Use or update your data in your own scripts and applications. -->

## Architecture

Beszel consists of two main components: the **hub** and the **agent**.

- **Hub**: A web application built on [PocketBase](https://pocketbase.io/) that provides a dashboard for viewing and managing connected systems.
- **Agent**: Runs on each system you want to monitor and communicates system metrics to the hub.

## Getting started

The [quick start guide](https://beszel.dev/guide/getting-started) and other documentation is available on our website, [beszel.dev](https://beszel.dev). You'll be up and running in a few minutes.

## Deployment (this fork)

This fork ships one-click, self-hosted deployment kits for a local GPU-cluster dashboard (中文说明见各目录下的 readme）：

- **Windows 10/11**: `deploy/windows` — `build.ps1` produces a portable folder with `beszel.exe`, `Monitor.exe` (launcher) and config templates; autostart via a scheduled task. See `deploy/windows/app/readme.md`.
- **Linux / WSL**: `Linux` branch, `deploy/linux` — `./run.sh start` builds (Go only, web UI prebuilt) and starts the hub; optional systemd service via `install.sh`; agent installer for monitored GPU nodes. See `deploy/linux/readme.md`.
- Windows 7/8 are not supported: the Go toolchain requires Windows 10+ and this dependency stack cannot build on older toolchains.

Note for building on Windows from source: `go build ./...` over the whole module first needs `go generate -run fetchsmartctl ./agent` (fetches `smartctl.exe` for the agent's embed). Building just the hub (`go build ./internal/cmd/hub`) is unaffected.

## Screenshots

![Dashboard](https://beszel.dev/image/dashboard.png)
![System page](https://beszel.dev/image/system-full.png)
![Notification Settings](https://beszel.dev/image/settings-notifications.png)

## Supported metrics

- **CPU usage** - Host system and Docker / Podman containers.
- **Memory usage** - Host system and containers. Includes swap and ZFS ARC.
- **Disk usage** - Host system. Supports multiple partitions and devices.
- **Disk I/O** - Host system. Supports multiple partitions and devices.
- **Network usage** - Host system and containers.
- **Load average** - Host system.
- **Temperature** - Host system sensors.
- **GPU usage / power draw** - Nvidia, AMD, and Intel.
- **Battery** - Host system battery charge.
- **Containers** - Status and metrics of all running Docker / Podman containers.
- **S.M.A.R.T.** - Host system disk health (includes eMMC wear/EOL and Linux mdraid array health via sysfs when available).

## Help and discussion

Please search existing issues and discussions before opening a new one. I try my best to respond, but may not always have time to do so.

#### Bug reports and feature requests

Bug reports and feature requests can be posted on [GitHub issues](https://github.com/henrygd/beszel/issues).

#### Support and general discussion

Support requests and general discussion can be posted on [GitHub discussions](https://github.com/henrygd/beszel/discussions) or the community-run [Matrix room](https://matrix.to/#/#beszel:matrix.org): `#beszel:matrix.org`.

## License

Beszel is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.
