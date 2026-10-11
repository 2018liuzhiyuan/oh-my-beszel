# Windows 端口与交互测试

从仓库根目录运行。测试使用隔离目录、临时数据库和测试端口；不修改生产配置。

| 测试 | 覆盖内容 |
| --- | --- |
| `local-ports.test.ps1` | 自定义范围、非法范围、范围占满、隧道分配不重复、短暂读锁重试、持续文件锁告警、原记录保留和恢复 |
| `local-forward-stderr.test.ps1` | 大量 SSH 错误输出异步落盘、无管道阻塞、重试覆盖旧日志、空输出和状态清理 |
| `local-forward-failures.test.ps1` | SSH 启动失败、诊断日志被锁定、资源清理及解锁后的重试 |
| `local-hub-start.test.ps1` | 实际占用网页端口，自动切换，写回 `config.json.port`，检查 `/api/health` |
| `local-port-sync.test.ps1` | 仅唯一匹配的有效本地隧道端口写回系统；同名远程系统、歧义匹配和暂停状态保持不变 |
| `port-alert-interaction.test.ps1` | 真实 Windows 端口/文件警告窗口、具体配置项或文件路径、点击确定、重复告警节流；可保存截图 |
| Go Hub/Monitor 测试 | Agent 范围校验、错误写入及恢复清除、配置读写竞争、排除其他 HTTP 服务 |
| SSH 搜索测试 | 前缀、正则、非法表达式、无匹配、操作范围仅限匹配的已选主机 |

```powershell
$env:GOEXPERIMENT = 'nojsonv2'
go test -tags=testing ./internal/hub ./internal/hub/systems ./internal/cmd/monitor
bun test ./internal/site/src/components/ssh-host-actions.test.ts
pwsh -NoProfile -File deploy/windows/hub/local-ports.test.ps1
pwsh -NoProfile -File deploy/windows/hub/local-forward-stderr.test.ps1
pwsh -NoProfile -File deploy/windows/hub/local-forward-failures.test.ps1
```

将以下路径替换为要验证的构建产物：

```powershell
pwsh -NoProfile -File deploy/windows/hub/local-hub-start.test.ps1 -HubBinary build/windows/app/beszel.exe -MonitorBinary build/windows/Monitor.exe
pwsh -NoProfile -File deploy/windows/hub/local-port-sync.test.ps1 -HubBinary build/windows/app/beszel.exe
pwsh -NoProfile -File deploy/windows/hub/port-alert-interaction.test.ps1 -HubBinary build/windows/app/beszel.exe -MonitorBinary build/windows/Monitor.exe -EvidenceDirectory .tmp/port-range-qa
```

真实弹窗测试需要 Windows 交互桌面，会打开并自动确认测试弹窗。实际告警不会自动关闭，必须点击“确定”；可加 `-PersistenceSeconds 31` 检查超过 30 秒仍可见。脚本部署可以通过 `local-hub-start.test.ps1 -HubBinary ... -HubScript ...` 验证。Linux 专用安装脚本集成测试在 Linux 上运行，包含范围耗尽和自动换端口。

浏览器交互检查：

1. 服务器范围耗尽时，系统页显示占用范围、`portRanges.agent.start/end` 和重启建议；恢复后原因清除。
2. Hub 或隧道范围耗尽时，弹窗以中英双语显示原因、配置项和恢复步骤；Hub 提示重启，隧道提示保存后点击确定自动重试。Agent 的系统页错误也包含中英双语。
3. “添加系统 → SSH”中，前缀和 `/正则/` 过滤列表；非法正则显示错误，无匹配时禁用导入/更新。
4. 桌面及 390px 手机宽度下检查提示换行、按钮可达和无横向溢出。
5. 隧道端口记录短暂被读取锁定时，原子写回最多尝试 20 次，间隔 100ms；持续锁定时显示文件路径和双语处理建议，不建议修改端口范围。处理后手动确认再重试。

手动模拟（不会占用端口或修改配置，弹窗需手动确认）：

```powershell
pwsh -NoProfile -File deploy/windows/hub/demo-port-alert.ps1 -ConfigPath '实际部署目录/config.json'
```
