# 上游 0.19.0 选择性合并

本批基于现有 0.18.8 定制工作树移植，经逐项核对后只引入本地仍缺少的修复和 CPU 状态告警，不代表完成整版升级。版本号与依赖版本保持原值。

## 已引入

| 来源 | 本地改动 |
| --- | --- |
| [SSH 并发关闭修复 #2277](https://github.com/henrygd/beszel/pull/2277) | 客户端指针原子读写，会话使用局部快照，关闭时先解除引用；本地 transport 层发现同类缺陷并一并修复。保留 SSH config、ProxyJump 与重连流程。 |
| [TOKEN_FILE #2276](https://github.com/henrygd/beszel/pull/2276) | 忽略空行和整行注释，只允许一个 Token。多 Token 文件明确报错，错误不包含 Token 内容；环境变量优先级、空文件行为保持原值。 |
| [电池名称 #2241](https://github.com/henrygd/beszel/pull/2241) | 移除固件名称中的非法 UTF-8 字节，避免整个 CBOR 指标负载被拒绝；保留名称回退与重名处理。 |
| [Intel GPU #2256](https://github.com/henrygd/beszel/pull/2256) | 两次采样之间沿用最近平均值，避免功耗与引擎利用率归零；保留离散 GPU 的零读数休眠处理。 |
| [CPU 分项告警 #2249](https://github.com/henrygd/beszel/pull/2249) | 新增 `CPUIOWait`、`CPUSteal`，支持单机、全局配置和中英文名称；缺失或无效采样不会误触发或误恢复。 |

在系统告警配置中开启 **CPU I/O 等待** 或 **CPU 窃取时间**，设置百分比阈值与观察分钟数。它们复用现有普通 CPU 告警的时间窗口规则；剩余显存告警仍使用独立的持续采样规则。新类型默认关闭。

数据库通过 `3_cpu_state_alerts.go` 增量添加两个告警选项，保留已有选项、字段、权限规则和记录。该迁移不提供自动删除选项的回退操作，以免已有新类型告警失效。

## 已跳过或延后

- 上游活跃告警 key 修复：本地已经使用 `alert.id` 渲染卡片，暂时忽略状态按系统与告警类型区分，因此无需重复移植。
- SwapCached 修正：本地已有扣除缓存和防下溢实现。
- HTTPS 证书验证默认切换、依赖更新、ZFS、systemd／容器健康告警、磁盘累计读写量不在本批范围。上游证书验证变更可能影响自签证书连接，后续应配合 CA 配置单独迁移。

## 验证

验证针对本批工作树；旧的发布检查记录不能替代本次结果。

后续严格复测已完成：完整套件 **19/19 阶段通过**，包括全包单测、vet、完整 race、两项模糊测试、原性能预算、Windows／Linux／FreeBSD／Darwin 构建和前端检查。完整 race 在原有 12 分钟包级上限内通过（阶段用时约 422 秒），补齐了首轮告警包超时的验证缺口；没有放宽预算、拆包或跳过测试。完整报告为 `test/reports/20260909-122850/report.md`，原始日志位于 `test/artifacts/20260909-122850/`。以下保留首轮验证与排查记录。

- 仓库 Quick 套件：16 个阶段中 15 个首次通过。唯一失败为测试清单仍将 `internal/migrations` 列为无直接测试的豁免模块；新增迁移测试后移除这条过时豁免，完整 `test/contracts` 包复测通过。原始失败报告保留在 `test/reports/20260909-104743/`，复测日志为 `test/artifacts/upstream-019-contracts-rerun.log`。
- 已通过全包编译、vet、其余 Go 包测试、性能预算；Windows Hub／Agent／Monitor 构建，以及 Linux Hub／Agent、FreeBSD／Darwin Agent 交叉编译。
- 前端 60 个单元测试、98 次断言、TypeScript、Biome 和生产构建通过。原有 5 个 Windows 浏览器回归场景通过，覆盖列重排、键盘、Esc 取消及告警布局。
- SSH systems／transport 包 race 测试通过；真实本地 SSH 服务验证 CBOR 取数、断线重连，以及共享客户端并发关闭后重新连接。
- Agent／Battery 包测试、vet 与针对性 race 通过；另验证真实 WebSocket 握手的 Token 头，以及电池数据的 CBOR 往返编解码。
- CPU 告警通过真实 PocketBase 记录路径验证触发、恢复、历史窗口和无效采样；增量迁移验证旧记录、规则和定制字段保留，迁移 race 通过。
- 新增浏览器场景 2/2 通过（1280×900、375×900）：真实登录、两类告警开启／编辑／关闭及刷新持久化、跨两系统批量配置、独立暂时忽略，已有显存告警不受影响；浏览器无未处理页面异常。最终 20 张中英文截图及布局数据位于 `test/artifacts/cpu-state-alerts-e2e/20260909-192255/`。活跃卡片使用 API 设置触发标志的展示 fixture，真实指标触发由 Go 集成测试单独验证。
- 独立代码审查批准，无阻塞项；两份独立界面审查均对最终 20 张截图给出 PASS。报告保存在 `.omo/evidence/upstream-019-selective-integration-code-review.md`、`upstream-019-visual-functional.md`、`upstream-019-visual-cjk.md`。

完整告警 race 套件和 CPU 定向 race 分别触及 10 分钟、5 分钟上限，超时栈均位于测试 fixture 的既有 PocketBase 集合导入／SQLite 初始化过程。另行隔离执行单个真实 CPU I/O 等待触发、恢复和通知计数场景，race 通过；它不能替代完整告警 race 套件。日志为 `test/artifacts/upstream-019-cpu-backend-race.log`、`upstream-019-cpu-backend-targeted-race.log` 和 `upstream-019-cpu-backend-single-race.log`。

本批未部署到运行中的 Hub／Agent，未在实际 Intel GPU、电池设备或远程集群上采样。交叉编译不代表对应操作系统的运行验证。其他语言的新增告警文本使用 Lingui 英文回退，原有翻译未覆盖。

浏览器检查另发现既有 375px 告警导航的 `Copy from`／“复制自”区域可能横向溢出，本批没有修改该导航布局。新增两类 CPU 卡片的中英文控件均通过各自边界检查；缩短新增中文说明，修复窄屏单字换行。测试节点创建钩子会将状态设为 `pending`，浏览器 fixture 在创建后显式暂停节点，避免断线状态更新干扰告警面板；原始失败运行记录保留。

复跑新增浏览器检查：

```powershell
pwsh -NoLogo -NoProfile -NonInteractive -File ./test/run-cpu-state-alerts-e2e.ps1
```
