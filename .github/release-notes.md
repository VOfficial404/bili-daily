## 下载哪个文件

| 设备 | 下载 | 说明 |
|---|---|---|
| Windows 电脑 | `bili-daily-desktop-setup-*-amd64.exe` | 安装包。不想安装就下 `*-windows-amd64-portable.zip`，解压即用；ARM 电脑选带 `arm64` 的 |
| Mac | `bili-daily-desktop-*.dmg` | Intel 和 Apple 芯片通用 |
| Linux 桌面 | `.deb`（Debian、Ubuntu） | 其他发行版下 `.tar.gz`，解压后运行 `install.sh` |
| Android 手机 | `bili-daily-*-android-arm64.apk` | Android 9 及以上，使用项目固定的发布密钥签名 |
| iPhone | `bili-daily-*-ios-unsigned.ipa` | 没有签名，用 [AltStore](https://altstore.io/) 或 [SideStore](https://sidestore.io/) 自己签名安装，iOS 15 及以上 |
| 服务器 | 推荐用 Docker：`ghcr.io/vofficial404/bili-daily` | 不用 Docker 时下 `bili-daily_<系统>_<架构>`，例如 `bili-daily_linux_amd64` |

**桌面安装包尚未做代码签名**：Windows 弹出 SmartScreen 时点「更多信息 → 仍要运行」；macOS 第一次打开时在 Finder 里右键 → 打开。Android APK 已签名，可在以后版本中覆盖更新；iOS IPA 仍需自行签名。

**校验**：`checksums.txt` 里是所有文件的 SHA-256。

**Android 签名证书 SHA-256**：`A0:80:B8:3F:7C:82:E3:9C:88:E6:71:CF:9D:E3:7F:C2:91:06:15:E9:CF:59:F3:A1:A4:96:98:4B:FE:26:17:D1`。此指纹标识项目的发布密钥，和 `checksums.txt` 中 APK 文件本身的哈希不同。
