# oh-my-beszel — Windows 便携包

纯 Go 静态编译，单目录绿色部署。最外层只有 `Monitor.exe`（启动器/任务运行器）和 `config.json` 等用户文件；Hub 本体（`beszel.exe`、`run-hub.ps1`、`agents\`、运行数据）整体收纳在 `app\` 子目录，日常无需触碰。默认只监听 `127.0.0.1:8090`，数据保存在 `app\beszel_data\`。

## 系统要求

| 系统 | 支持 | 说明 |
|---|---|---|
| Windows 11 / 10 (x64) | ✅ | 官方 Go 工具链支持的最低系统即 Win10 |
| Windows 8.1 | ⚠️ 未测试 | Go 官方已不承诺，多数情况可运行 |
| Windows 7 | ❌ | Go 1.21 起工具链已放弃 Win7，本项目依赖栈（PocketBase 0.36 等）无法在旧工具链编译。Win7 机器建议只当浏览器客户端，访问部署在 Win10+/Linux 上的 Hub |

浏览器：Chrome / Edge 近两年版本即可。

## 快速开始

1. **构建**（或直接使用发布包，跳到 2）：仓库根目录运行
   ```powershell
   bun install --cwd ./internal/site --frozen-lockfile
   bun run --cwd ./internal/site build
   pwsh -NoLogo -NoProfile -File ./deploy/windows/build.ps1
   ```
   产物在 `build\windows\`。构建需要 Go 1.26.1+，前端使用 Bun 和 Node.js 22.12+；运行便携包需要 PowerShell 7，不需要这些构建工具。
2. **改配置**：编辑便携目录的 `config.json`，设置自己的 `hub.userEmail` 和 `hub.userPassword`。模板默认开启免登录；需要密码登录时将 `hub.autoLogin` 改为 `""`。
3. **启动**（三选一）：
   - 直接使用：**双击 `Monitor.exe`**。无需任何前置步骤：未注册计划任务时 Monitor 会自动在后台拉起 Hub 并打开面板；关机或注销后进程自然结束，下次使用再双击即可。
   - 登录自启：双击 `install-task.cmd`（弹窗确认注册结果）。此后 Hub 随当前用户登录自动启动；双击 `Monitor.exe` 只负责打开面板。
   - 前台试运行：用 PowerShell 7 执行 `app\run-hub.ps1`，保持终端开启，手动访问 `http://127.0.0.1:8090`。

源仓库的 `docs/guide.md`（英文）和 `docs/guide.zh-CN.md`（中文）提供完整配置、SSH 部署和故障排查步骤。

## 修改网页端口

编辑实际运行的 `Monitor.exe` 同目录下的 `config.json`，将顶层 `"port": 8090` 改为未占用的端口，例如 `"port": 8091`，保留其他配置。无需重新编译。使用计划任务时，在 PowerShell 中重启 Hub：

```powershell
Stop-ScheduledTask -TaskName 'Beszel Hub'
Start-ScheduledTask -TaskName 'Beszel Hub'
```

如果自定义了任务名，请替换为实际名称；如果通过 `run-hub.ps1` 前台运行，按 Ctrl+C 后重新运行脚本。只重新打开 Monitor 不会重启已运行的任务。随后访问 `http://127.0.0.1:8091`，Monitor 下次启动也会打开新地址。同步修改指向旧网页端口的书签、代理或隧道；Agent 端口 `45876` 和 SSH 端口无需更改。

## config.json 字段

| 字段 | 默认 | 说明 |
|---|---|---|
| host / port | 127.0.0.1 / 8090 | Hub 监听地址；仅供本机访问保持默认 |
| openBrowser | true | Monitor 启动后是否自动打开浏览器 |
| startupTimeoutSeconds | 45 | Monitor 等待面板就绪的超时 |
| tasks | ["Beszel Hub"] | Monitor 启动时要拉起的计划任务名；任务未注册时自动改为直接启动 Hub |
| hubScript | app\run-hub.ps1 | 任务未注册时 Monitor 直接运行的 Hub 启动脚本 |
| hub.userEmail / userPassword | — | **首次启动**创建的登录账号（改密码对已有库无效，需走 UI） |
| hub.autoLogin | admin@beszel.local | 模板填写了邮箱，启用以该用户身份免密码访问；需要认证时清空，并避免继承 AUTO_LOGIN / BESZEL_HUB_AUTO_LOGIN 环境变量 |
| hub.checkUpdates | false | 传给 Hub 的保留字段 CHECK_UPDATES，不是本分支的安装器或更新渠道 |
| hub.logLevel | "" | 可选，`hub.log` 的日志级别：debug / info / warn / error，默认 info；debug 会输出每次 SSH 连接的子进程诊断 |
| sshConfigPath | "" | 可选，宿主机 `~/.ssh/config` 路径，用于 SSH 地址别名解析 |

## 添加被监控机器（Linux 节点）

在“添加系统 → SSH”中选择 Hub 运行账号可读取的 OpenSSH config，再选择主机并导入。该操作会为 pending/down 的 SSH 节点安排 Agent 自动部署，需要免交互 SSH 和目标机 root 或免密码 sudo 权限。便携包的 `app\agents\` 目录提供 Linux amd64 / arm64 程序；Agent 默认监听目标机的 `127.0.0.1:45876`，与操作系统 SSH 端口区分。

也可以手动复制本分支的 Agent，使用面板公钥配置 KEY，并在“二进制”页签填写节点 IP 与 Agent 端口。界面沿用的下载脚本和 Docker 镜像指向上游，不包含本分支改动。

## 运维

- 目录结构：`Monitor.exe` + `config.json` 在最外层；`app\` 内是 Hub 本体（`beszel.exe`、`run-hub.ps1`、`agents\`、`beszel_data\`、`hub.log`）。
- 日志：`app\hub.log`（Hub，超过 10 MB 自动轮转为 `hub.log.1`；连接失败原因也会显示在面板红点悬停、系统页与“设置 → Hub 日志”）、`launcher.log`（Monitor 启动失败原因，在根目录）。
- 修改配置后重启对应 Hub 实例；初始账号字段不能重置已有数据库中的密码。
- 备份/迁移：先停止 Hub，再复制完整的 `app\beszel_data\`，并保留 `config.json` 与 SSH 设置。
- `uninstall-task.cmd` 只注销任务，不删除数据，也不保证已运行的 Hub 停止；移动便携目录前需停止对应实例。
