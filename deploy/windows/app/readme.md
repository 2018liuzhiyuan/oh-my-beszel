# Beszel 本地监控面板 — Windows 一键包

纯 Go 静态编译，单目录绿色部署：`beszel.exe`（Hub，内嵌 Web UI）+ `Monitor.exe`（启动器/任务运行器）+ `config.json`。默认只监听 `127.0.0.1:8090`，数据保存在同目录 `beszel_data\`。

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
   powershell -ExecutionPolicy Bypass -File deploy\windows\build.ps1
   ```
   产物在 `build\windows\`。需要 Go 1.26+；UI 已预构建时无需 Node.js。
2. **改配置**：编辑 `config.json`（首次从 `config.example.json` 复制），至少改掉 `hub.userPassword`。
3. **启动**（二选一）：
   - 双击 `Monitor.exe`：启动 Hub、等面板就绪、自动开浏览器；
   - 运行 `install-task.ps1`：注册计划任务 `Beszel Hub`，开机自动启动（`uninstall-task.ps1` 可卸载）。

## config.json 字段

| 字段 | 默认 | 说明 |
|---|---|---|
| host / port | 127.0.0.1 / 8090 | Hub 监听地址；仅供本机访问保持默认 |
| openBrowser | true | Monitor 启动后是否自动打开浏览器 |
| startupTimeoutSeconds | 45 | Monitor 等待面板就绪的超时 |
| tasks | ["Beszel Hub"] | Monitor 启动时要拉起的计划任务名 |
| hub.userEmail / userPassword | — | **首次启动**创建的登录账号（改密码对已有库无效，需走 UI） |
| hub.autoLogin | "" | 填邮箱则本机免登录直达面板；留空则每次输密码 |
| hub.checkUpdates | false | 是否允许联网检查上游更新 |
| sshConfigPath | "" | 可选，宿主机 `~/.ssh/config` 路径，用于 SSH 地址别名解析 |

## 添加被监控机器（Linux 节点）

在节点上部署 beszel-agent（见 `deploy/linux/`，Linux 分支），然后在面板 UI 中手动添加；或参考 `configure-example.ps1` 用脚本批量注册（name/host/port/token）。

## 运维

- 日志：`hub.log`（Hub）、`launcher.log`（Monitor 启动失败原因）。
- 备份/迁移：整个目录拷走即可，历史数据都在 `beszel_data\`。
- 内存占用：Hub 常驻约 50 MB，Monitor 任务态约 8 MB/进程。
