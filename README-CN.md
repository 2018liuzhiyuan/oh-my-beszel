![oh-my-beszel](docs/assets/oh-my-beszel-logo.png)

# oh-my-beszel

看清 GPU 集群状态，通过 SSH 轻松管理。

[English](readme.md) · **简体中文**

[核心优势](#highlights) · [页面截图](#screenshots) · [快速开始](#quick-start) · 使用文档

面向 GPU 实验室与私有集群的轻量自托管监控。**oh-my-beszel** 将多 GPU 监控、SSH 主机导入和 Agent 部署集中在一个面板中，是基于 [Beszel](https://github.com/henrygd/beszel) 独立维护的非官方分支。



## 核心优势

- 🔍 **最大剩余显存检测。** 检测每台服务器所有 GPU 中的最大剩余显存，持续超过设定阈值时通知，方便发现可用单卡资源。
- 🚀 **批量导入 SSH config。** 沿用 OpenSSH 配置中的主机、跳板机和密钥，批量导入后直接在面板部署 Linux Agent。
- 📊 **多 GPU 监控。** 查看利用率、显存、温度及聚合与单卡历史曲线，配合 CPU I/O 等待和窃取时间告警。
- 🎨 **按习惯定制总览。** 拖动调整列顺序、隐藏不需要的列、切换主题，也能在手机上查看集群。

保留 Beszel 的系统历史、Docker / Podman 监控、多用户、OAuth / OIDC 和备份功能。



## 页面截图

截图来自隔离演示实例，主机与监控指标均为虚构数据。

**集群总览**

![展示 GPU 利用率、显存与服务器状态的集群总览](docs/assets/screenshots/overview-light.png)

**服务器详情**

![服务器页面中的 CPU、内存、网络与温度历史图表](docs/assets/screenshots/system-history.png)

展开多 GPU 图表、SSH 管理与深色主题

**多 GPU 历史**

![各 GPU 的利用率与显存历史图表](docs/assets/screenshots/gpu-history.png)

**SSH 主机管理**

![SSH 主机导入与 Agent 部署面板](docs/assets/screenshots/ssh-hosts.png)

**深色总览**

![深色主题集群总览](docs/assets/screenshots/overview-dark.png)



移动端总览

![移动端集群总览](docs/assets/screenshots/overview-mobile.png)





## 快速开始

运行一个 **Hub** 提供面板，在每台被监控节点运行 **Agent**。请使用本仓库构建的程序；上游二进制与镜像不包含本分支增强功能。

**已经有 `build/` 目录？** 该目录不纳入 git，但发布包或此前的构建可能已包含可直接运行的程序。若存在 `build/windows/app/beszel.exe` 和 `build/windows/Monitor.exe`，可跳过下面的构建命令，直接从 `config.json` 配置一步开始。macOS 包暂存于 `build/macos/<架构>/`，Linux 程序位于 `build/linux/`；运行前请校验包内校验和。二进制只包含其构建时间之前的改动（见 `build-info.txt`）；需要最新代码时按下面的命令重新构建。

**Windows** · 需要 Go 1.26.1+、Bun 和 PowerShell 7。在仓库根目录执行：

```powershell
bun install --cwd ./internal/site --frozen-lockfile
bun run --cwd ./internal/site build
pwsh -NoLogo -NoProfile -File ./deploy/windows/build.ps1
```

编辑 `build/windows/config.json`，设置 `hub.userEmail` 和 `hub.userPassword`，清空 `hub.autoLogin` 以使用密码登录。仅本机访问时保留 `host` 为 `127.0.0.1`。这些凭据用于初始化新数据库；已有账号和继承的自动登录设置见 [Windows 指南](docs/guide.zh-CN.md#windows)。

```powershell
Start-Process -FilePath ./build/windows/Monitor.exe
```

打开 **[http://127.0.0.1:8090](http://127.0.0.1:8090)** 登录。发布包内不含 PowerShell 脚本，也不依赖用户安装的 PowerShell 版本。准备好 Hub 的 SSH 访问后，在 **添加系统 → SSH** 中导入节点。

**修改网页端口：** 编辑实际运行的 `Monitor.exe` 同目录下 `config.json` 的 `port`（例如 `8091`），重启 Hub 后访问 `http://127.0.0.1:8091`。详见[端口与重启步骤](docs/guide.zh-CN.md#web-port)。

**macOS** · 从 Releases 下载对应压缩包（M 系列芯片选 `darwin_arm64`，Intel 选 `darwin_amd64`），解压后双击 **Open.command**。启动器会自动安装到 `~/Applications`、保留已有配置、设置登录自启动，并在确认启动成功后打开网页。首次访问创建账号即可，不需要 Go、Bun 或 Node.js。

上述步骤适用于包含 `Open.command` 的新版包。未签名下载包可能需要首次放行，见[简单使用手册](deploy/macos/package/QUICKSTART.zh-CN.md)。日常直接访问 http://127.0.0.1:8090 即可。源码构建和高级配置见 [macOS 指南](docs/guide.zh-CN.md#macos)。

**Linux / WSL** · 请参阅[构建与启动指南](docs/guide.zh-CN.md#linux)。

[SSH 部署](docs/guide.zh-CN.md#ssh) · [手动安装 Agent](docs/guide.zh-CN.md#manual-agent) · [macOS 自启动](docs/guide.zh-CN.md#macos) · [Windows 启动器与自动启动](docs/guide.zh-CN.md#windows)

### 共享服务器上的 Agent 端口冲突

如果 SSH 部署显示完成，但系统随即报告 `ssh: unable to authenticate, attempted methods [none publickey]`，请先检查配置的 Agent 端口是否已被其他用户的 Agent 或系统服务占用。Agent 端口由整台目标主机共享，因此两个用户不能同时监听默认的 `45876` 地址。

请对每台受影响的目标服务器分别执行以下步骤：

1. 使用 Hub 中配置的同一个 SSH 别名登录目标服务器，然后检查一个候选端口。下面以 `45878` 为例；请将它替换为你想使用的端口：

   ```bash
   PORT=45878
   if ss -ltnH "sport = :$PORT" | grep -q .; then
     echo "port $PORT is occupied"
   else
     echo "port $PORT is free"
   fi
   ```

   请选择显示为 `free` 的端口。每台服务器都要单独检查，因为同一个端口可能在一台服务器上空闲、在另一台服务器上已被占用。不要停止或替换其他用户的进程。

2. 打开 Hub 面板，在系统列表中找到报错的系统，点击该行右侧的 **三点菜单 → 编辑（Edit）**。
3. 在编辑窗口中，将 **端口（Port）** 从默认的 `45876` 改为刚才确认空闲的端口。保持 **主机/IP（Host / IP）** 不变，然后点击 **保存系统（Save System）**。
4. 保存后，该系统会自动变为 `pending`。如果它原本是通过 SSH config 导入的，Hub 会自动停止当前 SSH 账号自己管理的 Agent、写入新的 `LISTEN` 端口、重新启动 Agent，并通过 SSH 重新连接；界面中没有单独的“重新部署”按钮。
5. 等待系统状态变为 `up`，并确认监控图表开始更新。如果新端口也被占用，请换一个空闲端口后重复以上步骤；当前自动部署会明确报告端口冲突，不再把其他用户的监听进程误判为部署成功。

自动重新部署要求该系统仍然关联 SSH config。如果系统是手动安装且没有 SSH config，请先自行把目标 Agent 的 `LISTEN` 改成同一个新端口并重启 Agent，再在 Hub 中保存对应的 **端口（Port）**。完整诊断步骤见[SSH 部署故障排查](docs/guide.zh-CN.md#troubleshooting)。

### Agent 用户目录

SSH 自动部署不需要 root 权限，Agent 默认集中安装在一个可整体删除的用户目录：

```text
~/oh-my-beszel/
├── bin/beszel-agent
├── config/env
├── config/hub_keys
├── data/
└── logs/
```

同一个 Agent 可以同时信任多套 Hub；各 Hub 公钥按行保存在 `config/hub_keys`。卸载 SSH 自动部署的 Agent 及其数据时，先停止用户服务，再删除服务链接和整个目录：

```bash
systemctl --user disable --now oh-my-beszel-agent.service 2>/dev/null || true
rm -f "$HOME/.config/systemd/user/oh-my-beszel-agent.service"
rm -rf "$HOME/oh-my-beszel"
systemctl --user daemon-reload 2>/dev/null || true
```

删除 `data/` 会同时删除 Agent 身份。服务与后台运行回退机制详见 [SSH 部署指南](docs/guide.zh-CN.md#ssh)。

## 文档与贡献

- [配置与运维](docs/guide.zh-CN.md) · [故障排查](docs/guide.zh-CN.md#troubleshooting)
- [上游合并说明](docs/upstream-0.19.0.md)：基于 0.18.8，选择性引入 0.19.0 的改动。

欢迎通过 [GitHub](https://github.com/2018liuzhiyuan/oh-my-beszel/issues) 提交问题与 PR。修改配置或行为说明时，请同步更新中英文版本。

## 致谢与许可证

基于 [Beszel](https://github.com/henrygd/beszel) 与 [PocketBase](https://pocketbase.io)，感谢作者与贡献者。[MIT License](LICENSE)。
