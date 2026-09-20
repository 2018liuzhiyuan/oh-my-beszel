# macOS 简单使用手册

适用于带有 `Open.command` 的新版 Release 包。旧版只有 `start.sh` 时不适用。

## 第一次使用

1. 从项目 Releases 下载与你的 Mac 匹配的压缩包：M 系列芯片选 `darwin_arm64`，Intel 选 `darwin_amd64`。
2. 双击压缩包解压，再双击文件夹中的 **Open.command**。
3. 等待浏览器自动打开。首次使用创建管理员账号；已有数据时使用原账号登录。

无需安装 Go、Bun 或 Node.js，也无需手动复制配置文件。启动器会检查包内校验和，安装到 `~/Applications/oh-my-beszel-darwin-<架构>`，注册登录自启动服务，确认网页正常后再打开浏览器。终端显示成功后可以关闭。

## 如果 macOS 阻止打开

当前包未经过 Apple 开发者签名和公证，因此不能保证下载后完全免确认。确认下载自可信 Release，并按 Release 中的 SHA-256 校验文件核对下载后：

打开终端，输入 `cd `（末尾有空格），将解压后的文件夹拖入终端，按回车。然后执行一次：

```bash
xattr -dr com.apple.quarantine .
/bin/sh ./Open.command
```

上述命令仅应在确认可信的解压目录执行，不需要 sudo，也不用关闭系统安全保护。

## 日常使用

打开 http://127.0.0.1:8090 即可。无需每次运行脚本；每次登录 Mac 会自动启动。

添加 Linux 服务器：先确保本机终端能通过自己的 SSH config 别名免密登录目标，再在网页选择“添加系统 → SSH”，选中服务器并导入。端口有冲突时，在该系统的“三点菜单 → 编辑 → 端口”改为空闲端口并保存，Hub 会自动重试部署。

## 更新、配置与停止

- 更新：下载新版并双击 `Open.command`。保留已注册安装的 `config.env`，原安装目录备份到 `~/Applications/oh-my-beszel-backup.*`。不要同时手动运行另一个 Hub。
- 配置：默认只供本机访问；需要修改时编辑安装目录的 `config.env`，再双击安装目录中的 `Open.command`。SSH config 默认是使用者自己的 `~/.ssh/config`。
- 数据：默认保存在 `~/Library/Application Support/oh-my-beszel`，升级保留账号和历史数据。若自行设置数据路径，请使用稳定的绝对路径。
- 停止自启动：在安装目录执行 `./uninstall-service.sh`；数据保留。重新双击 `Open.command` 可恢复。
- 网页未打开：查看安装目录的 `logs/hub-error.log`。若提示端口被占用，先在之前手动运行 Hub 的终端按 Ctrl+C，再重试。

安装完成后保留 Applications 中的安装目录；Downloads 中的解压副本可移走。不同版本使用同一数据目录时，应确保同一时间仅有一个 Hub 运行。
