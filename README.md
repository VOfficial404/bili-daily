# bili-tools

每天替 B 站账号做完日常任务：看视频、分享、投币、大会员积分和权益、漫画签到、银瓜子换硬币，以及可选的粉丝牌和天选时刻。支持多个账号。

- **服务端**是一个没有依赖的可执行文件，跑在服务器上（Docker 或直接运行）。
- **桌面客户端**（Windows、macOS、Linux）和**手机 App**（Android、iOS）连上服务端查看和操作；没有服务器时，客户端也可以自己在本机运行任务。
- 每一轮运行有一份报告：每个任务的每一步是成功、跳过还是失败，失败的写明原因和该怎么办。

功能参考了 [BiliBiliToolPro](https://github.com/RayWangQvQ/BiliBiliToolPro)，实现是独立的，做法也不一样，见 [docs/DESIGN.md](docs/DESIGN.md)。

当前交付是本地源码和构建产物，尚未发布到 GitHub。按项目要求，必须先由 Claude 审查并确认代码，之后才允许创建 GitHub 仓库或上传、推送、发布。下面的 Releases、镜像和一键安装地址供获准发布后使用；现在可以按文末命令从源码构建。Windows 桌面和 Android APK 已构建；iOS 提供源码和 macOS CI 构建流程，尚未完成构建或真机验证。

本地交付文件集中在 `dist/`：源码压缩包、Windows 安装包与便携版、Android APK、服务端二进制和 SHA-256 校验清单。源码包不包含账号数据、签名私钥或 Git 历史，可直接交给 Claude 审查。

> [!WARNING]
> - 自动化请求可能触发 B 站风控，也不符合 B 站的用户协议。是否使用、后果如何，请自行判断和承担。
> - 投币会花掉硬币（默认每天 5 个，6 级之后不再投）。粉丝牌、天选时刻和 B 币券充电默认是关的：它们会在别人直播间里留言、关注陌生主播或花掉 B 币券，打开之前先看一眼选项。
> - 本项目与哔哩哔哩官方无关。

## 任务

| 任务 | 做什么 | 默认 |
|---|---|---|
| 每日任务 | 看一个视频、分享一次、投币，拿每天的经验 | 开 |
| 大会员大积分 | 签到、浏览类积分任务、领取每天的 10 点经验；完成后再查询确认 | 开（需要大会员） |
| 大会员权益 | 领每月的 B 币券、优惠券等 | 开（需要大会员） |
| B 币券充电 | 每月把 B 币券充给指定的 UP 主，免得过期 | 关（要先填给谁充） |
| 漫画 | 签到，记一次阅读 | 开 |
| 大会员漫画权益 | 领每月的漫读券 | 开（需要大会员） |
| 银瓜子换硬币 | 每天换一次 | 开 |
| 粉丝牌 | 给有粉丝牌的直播间点赞、发弹幕、挂观看时长 | 关 |
| 天选时刻 | 参加直播间的天选时刻抽奖，按奖品名筛选 | 关 |
| 批量取关 | 取关一个关注分组里的人（例如抽奖关注的主播） | 只在你点运行时执行 |

每个账号每天跑一轮：到设置的时间（默认北京时间 09:30，再随机推迟 0~60 分钟）依次做完开着的任务。到点时程序没在运行，下次启动会补上。任务先看 B 站那边今天做过没有，所以多跑几轮不会重复投币。

「观看正片 10 分钟」的 App 协议需要额外授权，本项目目前用 Cookie 请求完成后查询结果；没有入账会明确显示需要在 B 站 App 里观看。漫画阅读只确认提交了阅读历史，不保证发放奖励。其余接口也会随 B 站更新变化，模拟响应测试通过不代表真实账号一定能领到奖励。

## 服务器：Docker Compose

```bash
mkdir -p /opt/bili-tools && cd /opt/bili-tools
curl -fsSLO https://raw.githubusercontent.com/VOfficial404/bili-tools/main/compose.yaml
docker compose up -d
docker compose exec bili-tools bili-tools token   # 显示令牌
```

客户端连接时填**地址**和**令牌**。Compose 默认只监听服务器本机的 `127.0.0.1:8686`，从别处连接有两种方式：

- **SSH 隧道**：在自己电脑上执行下面的命令并保持打开，客户端里填 `http://127.0.0.1:8686`。

  ```bash
  ssh -N -L 8686:127.0.0.1:8686 root@你的服务器IP
  ```

- **HTTPS 反向代理**（有域名时；手机连服务器用它），例如 Caddy：

  ```
  bili.example.com {
      reverse_proxy 127.0.0.1:8686
  }
  ```

  客户端里填 `https://bili.example.com`。

不要把明文 HTTP 的端口直接开到公网：令牌会在传输中被看到，拿到令牌就能操作你的账号。

**添加账号**：在客户端里点「添加账号」扫码，或者在服务器上：

```bash
docker compose run --rm bili-tools login
```

**更新**：`docker compose pull && docker compose up -d`。**日志**：`docker compose logs -f`。

## 服务器：不用 Docker

```bash
curl -fsSL https://raw.githubusercontent.com/VOfficial404/bili-tools/main/scripts/install.sh | sudo sh
```

脚本下载最新版本到 `/opt/bili-tools`、校验 SHA-256、装成 systemd 服务，并显示地址和令牌。再运行一次就是更新。

也可以从 [Releases](https://github.com/VOfficial404/bili-tools/releases/latest) 下载 `bili-tools_<系统>_<架构>` 手动运行：

```
bili-tools            常驻运行
bili-tools login      在终端里扫码添加账号
bili-tools run        立即给所有账号跑一轮然后退出（可以交给 cron）
bili-tools token      显示令牌
```

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `BILI_TOOLS_DATA` | 程序所在目录下的 `data` | 数据目录 |
| `BILI_TOOLS_ADDR` | `127.0.0.1:8686` | 接口监听地址 |
| `BILI_TOOLS_TOKEN` | 自动生成，保存在数据目录里 | 接口令牌 |

其他设置（开始时间、任务开关、代理、推送）都在客户端里改，保存在数据目录的 `settings.json`。

## 桌面客户端

从 [Releases](https://github.com/VOfficial404/bili-tools/releases/latest) 下载：

| 系统 | 文件 |
|---|---|
| Windows | `bili-tools-desktop-setup-*-amd64.exe`（安装包），或 `*-windows-amd64-portable.zip`（解压即用） |
| macOS | `bili-tools-desktop-*.dmg` |
| Linux | `.deb`，或 `.tar.gz`（解压后运行 `install.sh`） |

安装包没有签名：Windows 弹出 SmartScreen 时点「更多信息 → 仍要运行」；macOS 第一次打开时在 Finder 里右键 → 打开。

打开后选择用法：

- **连接服务器**：填地址和令牌。勾选「记住令牌」后，令牌保存在系统的凭据管理器里，下次自动连接。
- **在本机运行**：没有服务器时用。任务由这台电脑执行，所以只有程序开着的时候才会跑；关掉窗口后它留在托盘里继续运行。错过的那一轮会在下次打开时补上。

页面：

- **概览**：每个账号今天各任务的结果、正在进行的进度、下次运行时间。
- **账号**：等级、硬币、大会员、Cookie 有效期；单独关掉某个账号的某些任务；重新扫码；退出账号。
- **任务**：每个任务的开关和选项；任何任务都可以单独「运行一次」。
- **报告**：每一轮的报告。失败的步骤写着原因和建议。
- **设置**：每天的开始时间、请求节奏、代理、推送、日志级别。
- **日志**。

## 手机 App

从 [Releases](https://github.com/VOfficial404/bili-tools/releases/latest) 下载：

- **Android**：`bili-tools-*-android-arm64.apk`，Android 9 及以上，arm64 设备。
- **iOS**：`bili-tools-*-ios-unsigned.ipa`，没有签名，需要用 [AltStore](https://altstore.io/)、[SideStore](https://sidestore.io/) 之类的工具自己签名安装。iOS 15 及以上。

手机 App 主要用来**连接服务器**。它也能「在本机运行」，但手机系统可能暂停或结束后台进程；本机任务只有在 App 打开时可靠运行，回到 App 时会重新检查并补跑。切换到 B 站扫码时保留登录二维码会话。

令牌保存在 App 的私有目录里（没有用系统的钥匙串），只在你选择「记住」时保存。

给账号扫码时，B 站 App 和本 App 在同一台手机上：把二维码截图，在 B 站 App 的「扫一扫」里从相册选这张截图。

## 任务失败了怎么办

打开那一轮的报告，看失败的那一步：

| 原因 | 怎么办 |
|---|---|
| 登录失效 | 给这个账号重新扫码。Cookie 平时会自动刷新，被 B 站踢下线或改了密码之后需要重新扫码 |
| 被风控拦截 | 过几小时再试。经常出现的话，在设置里把请求节奏调成「慢」，或者换一个出口 IP（设置里的代理） |
| 请求太频繁 | 会自动恢复；把请求节奏调慢 |
| 网络或代理不通 | 检查服务器的网络和代理设置。每日计划会在 30 分钟后自动重试，一天最多 3 次 |
| 接口可能变了 | 更新到最新版本；仍然出现请提 issue，附上报告里那一步的说明 |

账号不满足条件（不是大会员、硬币不够、今天已经做过）记为「跳过」，不算失败。

## 推送

在设置里选一个渠道，运行结束后推送结果，可以选「有失败时」或「每次」：Telegram、PushPlus、企业微信群机器人、钉钉群机器人（支持加签）、Server 酱、Bark、自定义 Webhook（收到 `{"title": …, "text": …}`）。「发送测试」可以先确认填得对不对。

## 安全

- `data/accounts/` 里的 `session.json` 等同于账号的登录权限，`data/token` 等同于服务端的管理权限。保护好服务器，不要把 `data/` 提交到任何地方。
- 不想用了：在客户端里对每个账号点「退出账号」（会在 B 站注销这份会话），再删除容器或程序。直接删了数据的话，到 B 站「账号安全 → 登录设备」里把对应设备下线。

## 从源码构建

需要 Go 1.27 或更新。

```bash
git clone https://github.com/VOfficial404/bili-tools.git && cd bili-tools
go test ./...
go build ./cmd/bili-tools            # 服务端
(cd desktop && go tool mygo build)   # 桌面客户端，产物在 desktop/build/
(cd mobile && go tool nsgo build -target android_arm64)   # Android，产物在 mobile/dist/
(cd mobile && go tool nsgo build -target ios)             # iOS，需要 macOS 和 Xcode
```

结构和接口见 [docs/DESIGN.md](docs/DESIGN.md)。

## 致谢

- [BiliBiliToolPro](https://github.com/RayWangQvQ/BiliBiliToolPro)：功能的参考。
- [bilibili-API-collect](https://github.com/SocialSisterYi/bilibili-API-collect)：B 站接口的资料。
- [MyGo](https://mygo.run) 和 [NSGo](https://github.com/nsgo-dev/nsgo)：桌面和手机的界面。

## 许可

[MIT](LICENSE)
