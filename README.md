<div align="center">

<img src="app/resources/icon.png" width="96" alt="Bili Daily">

# Bili Daily

**B 站日常，到点做完。**

签到、投币、大会员积分、漫画、银瓜子，每天到点给每个账号跑一轮。<br>
哪一步没成，报告里写着为什么、该怎么办。

[![版本](https://img.shields.io/github/v/release/VOfficial404/bili-daily?color=fb7299&label=版本)](https://github.com/VOfficial404/bili-daily/releases/latest)
[![CI](https://img.shields.io/github/actions/workflow/status/VOfficial404/bili-daily/ci.yml?label=CI)](https://github.com/VOfficial404/bili-daily/actions/workflows/ci.yml)
[![许可](https://img.shields.io/github/license/VOfficial404/bili-daily?label=许可)](LICENSE)

**[项目主页](https://vofficial404.github.io/bili-daily/)** · [下载](https://github.com/VOfficial404/bili-daily/releases/latest) · [四行命令开始](#四行命令就能开始) · [先读这些](#用之前先读这些)

</div>

<table>
  <tr>
    <td width="68%"><img src="docs/images/desktop.png" alt="桌面客户端的「今天」页"></td>
    <td><img src="docs/images/mobile.jpg" alt="手机 App 的「今天」页"></td>
  </tr>
</table>

## 该做的，每天都做完

- **一天，只跑一轮。** 只设一个开始时间，到点后依次做完开着的任务。错过了，开机就补。
- **失败了，也说得明白。** 每一轮留一份报告，失败的那一步写着原因和办法。报告默认保留 30 天，可在设置里调整，每账号最多留 60 份。
- **不会重复投币。** 每个任务先查 B 站今天做过没有，多跑几轮也只做还没做的。
- **扫一下，就添加好。** 用 B 站 App 扫码添加账号，Cookie 快过期时自己续期。
- **花钱的事，先问你。** 充电、粉丝牌、天选时刻默认关着，打开前会让你确认。
- **账号，只在你手里。** 凭据和报告都存在你自己的服务器上，这个项目不收集任何数据。

优先投币的 UP 主和充电对象不用记 UID：直接从账号的关注里点选，可以按昵称搜索，也能粘贴 UID 或空间链接。选上的显示头像和昵称，优先 UP 主可以调整先后。

## 在哪看，都一样

三个入口连着同一个服务端，一处改了，另外两处马上看到。

| | |
|---|---|
| **网页面板** | 服务端自带。浏览器打开服务器地址，用令牌登录 |
| **桌面客户端** | Windows、macOS 与 Linux。没有服务器，也能直接在本机跑 |
| **手机 App** | Android 与 iOS。出门在外看一眼，失效了就地重新扫码 |

挑你的设备下载：[Releases](https://github.com/VOfficial404/bili-daily/releases/latest) 页面写着每种设备该下哪个文件。

## 四行命令，就能开始

一台装了 Docker 的 Linux 服务器就够：

```bash
mkdir -p /opt/bili-daily && cd /opt/bili-daily
curl -fsSLO https://raw.githubusercontent.com/VOfficial404/bili-daily/main/compose.yaml
docker compose up -d
docker compose exec bili-daily bili-daily token   # 登录用的令牌
```

浏览器打开 `http://127.0.0.1:8686`，粘贴令牌，扫码添加账号。添加后马上跑第一轮，以后每天北京时间 09:30 起自己跑。更新只要 `docker compose pull && docker compose up -d`。

> [!IMPORTANT]
> 服务端默认只听本机。要从别的设备访问，用 SSH 隧道或 HTTPS 反向代理，**别把明文端口开到公网**：拿到令牌，就能操作你的账号。

<details>
<summary><b>从别的电脑或手机访问</b></summary>

- **SSH 隧道**：在自己电脑上保持运行 `ssh -N -L 8686:127.0.0.1:8686 root@服务器IP`，再打开 `http://127.0.0.1:8686`。
- **HTTPS 反向代理**（有域名时，手机用它），例如 Caddy：

  ```
  bili.example.com {
      reverse_proxy 127.0.0.1:8686
  }
  ```

</details>

<details>
<summary><b>不用 Docker</b></summary>

```bash
curl -fsSL https://raw.githubusercontent.com/VOfficial404/bili-daily/main/scripts/install.sh | sudo sh
```

装到 `/opt/bili-daily`，注册成 systemd 服务；再跑一次就是更新。也可以下载 `bili-daily_<系统>_<架构>` 自己运行：

| 命令 | 做什么 |
|---|---|
| `bili-daily` | 常驻运行，提供网页面板和接口 |
| `bili-daily login` | 在终端里扫码添加账号 |
| `bili-daily run` | 立即跑一轮然后退出，可以交给 cron |
| `bili-daily token` | 显示令牌 |

数据目录、监听地址这些启动参数用环境变量改，见下面的[环境变量](#环境变量)。

</details>

### 环境变量

大多数情况下什么都不用设。投币数、请求节奏、代理、推送这些日常设置都在网页面板或客户端的「设置」里改，不用碰环境变量。只有下面这几项要在启动服务端之前定好：

| 环境变量 | 默认 | 什么时候要改 |
|---|---|---|
| `BILI_DAILY_TOKEN` | 自动生成，存在数据目录里 | 想自己定一个好记的令牌。设了之后以它为准 |
| `BILI_DAILY_PORT` | `8686` | 只对 Docker 有用：8686 端口被别的程序占了，换一个本机端口 |
| `BILI_DAILY_NO_UPDATE_CHECK` | 不设 | 设为 `1`：不再去 GitHub 查有没有新版本 |
| `BILI_DAILY_ADDR` | `127.0.0.1:8686` | 不用 Docker 时改监听地址或端口。`127.0.0.1` 表示只有本机能连 |
| `BILI_DAILY_DATA` | 程序旁边的 `data` | 不用 Docker 时把账号和报告放到别的目录 |
| `BILI_DAILY_DIR` | `/opt/bili-daily` | 只对安装脚本有用：装到别的目录 |

改完要重启服务端才生效。按你的装法选一种：

<details>
<summary><b>Docker</b></summary>

在 `compose.yaml` 旁边新建一个 `.env` 文件，一行一个：

```bash
BILI_DAILY_TOKEN=换成你自己的令牌
BILI_DAILY_PORT=18686
BILI_DAILY_NO_UPDATE_CHECK=1
```

然后运行 `docker compose up -d`。改了端口，面板地址也跟着变成 `http://127.0.0.1:18686`。容器里的数据目录和监听地址已经定好，不要改 `BILI_DAILY_DATA` 和 `BILI_DAILY_ADDR`。

之前下载的 `compose.yaml` 里如果没有 `BILI_DAILY_NO_UPDATE_CHECK` 这一行，`.env` 里写了也不会生效。重新下载一份 `compose.yaml` 覆盖就行，数据在 `data` 目录里，不会丢。

</details>

<details>
<summary><b>安装脚本（systemd）</b></summary>

第一次安装时可以带上变量。注意要写在 `sudo` 后面，否则会被 `sudo` 丢掉：

```bash
curl -fsSL https://raw.githubusercontent.com/VOfficial404/bili-daily/main/scripts/install.sh | sudo BILI_DAILY_ADDR=127.0.0.1:18686 sh
```

装好以后再改，重跑安装脚本**不会**改动已有的服务配置，要这样改：

```bash
sudo systemctl edit bili-daily
```

在打开的编辑器里写：

```ini
[Service]
Environment=BILI_DAILY_NO_UPDATE_CHECK=1
Environment=BILI_DAILY_ADDR=127.0.0.1:18686
```

保存后运行 `sudo systemctl restart bili-daily`。

</details>

<details>
<summary><b>直接运行程序</b></summary>

写在命令前面，例如：

```bash
BILI_DAILY_DATA=/srv/bili-daily BILI_DAILY_ADDR=127.0.0.1:18686 ./bili-daily
```

之后用 `login`、`run`、`token` 也要带上同样的 `BILI_DAILY_DATA`，不然它们会去找另一个数据目录。`bili-daily help` 也能看到这份列表。

</details>

## 花钱的，默认不做

| 默认开 | 默认关 |
|---|---|
| **每日任务**：看视频、分享、投币 | **B 币券充电**：每月充给指定的 UP 主 |
| **大会员大积分**：签到和浏览任务 ¹ | **粉丝牌**：点赞、发弹幕、挂时长 |
| **大会员权益**：每月的 B 币券和优惠券 ² | **天选时刻**：按奖品名筛选抽奖 |
| **漫画**：签到，记一次阅读 | **批量取关**：只在你点运行时 |
| **大会员漫画权益**：每月的漫读券 ¹ | |
| **银瓜子换硬币**：每天一次 | |

¹ 需要大会员。² 需要有效的年度大会员；B 币券充电也会检查年度资格，不符合就自动跳过。观看正片会领取任务并提示到官方 App 连续观看 10 分钟，本工具不会把领取当作观看完成。

跑完的结果可以推送到 Telegram、企业微信、钉钉、Bark、PushPlus、Server 酱或自定义 Webhook。

<details>
<summary><b>没跑成，怎么办</b></summary>

打开那一轮的报告，失败的那一步写着原因和建议。常见的几种：

| 报告里写着 | 怎么办 |
|---|---|
| 登录失效 | 重新扫码。Cookie 平时会自己续期，被踢下线或改了密码才需要 |
| 被风控拦截 | 过几小时再试；常出现就把请求节奏调慢，或者换个出口 IP |
| 网络不通 | 检查网络和代理。每日计划会隔 30 分钟重试，一天最多 3 次 |
| 接口可能变了 | 更新到最新版；还是不行就提 issue，附上报告里那一步 |

不满足条件的（不是大会员、今天已经做过）记作「跳过」，不算失败。

</details>

## 想改代码

需要 Go 1.27 或更新。结构、接口和每日计划的规则，写在[设计文档](docs/DESIGN.md)里。

```bash
go test ./...                                               # 服务端和引擎
go build ./cmd/bili-daily                                   # 服务端，网页面板已经嵌进去
(cd app && go tool mygo build)                              # 桌面客户端
(cd app && go tool mygo build -platform android/arm64)      # Android，需要 Android SDK、NDK 和 JDK 17
(cd app && go tool mygo build -platform ios)                # iOS，需要 macOS、Xcode 和描述文件
```

桌面客户端和手机 App 用 [MyGo](https://mygo.run) 写成，共用 `app/` 一套代码。`go tool mygo doctor` 检查构建手机版缺什么。

## 用之前，先读这些

> [!WARNING]
> 本项目与哔哩哔哩官方无关。它调用 B 站的非公开接口，可能触发风控，也不符合 B 站的用户协议；投币和充电会花掉虚拟财产，而且撤不回。后果由使用者自行承担。完整的[免责声明](https://vofficial404.github.io/bili-daily/#disclaimer)在项目主页上。

功能参考了 [BiliBiliToolPro](https://github.com/RayWangQvQ/BiliBiliToolPro)（实现独立），接口资料来自 [bilibili-API-collect](https://github.com/SocialSisterYi/bilibili-API-collect)。以 [MIT](LICENSE) 许可开源。
