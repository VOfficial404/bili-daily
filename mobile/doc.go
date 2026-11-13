// Bili Tools 手机客户端使用同一套 HTTP 接口连接服务器或进程内引擎。
//
// 手机推荐连接服务器。本机任务只能在 App 打开时可靠运行；手机系统可能暂停后台运行，
// 稳定定时请连接服务器，回到 App 时补跑。保留进程内引擎可让扫码切换 App 后继续确认。
// 网页没有本机数据目录，只提供服务器连接。没有后台保活和定时本地通知。
// 服务端令牌仅在用户勾选记住时写入 NSGo 的明文私有存储，断开连接会清除它。
//
// 在此目录运行：
//
//	go tool nsgo check
//	go tool nsgo test
//	go tool nsgo build -target windows_x64
//	go tool nsgo build -target android_arm64
//	go tool nsgo build -target web
//
// iOS 需要在装有 Xcode 的 macOS 上运行 go tool nsgo build -target ios。
// 真机安装还需在 nsgo.yaml 配置 sign.profile 和 sign.identity。
// 图标可用 go run ./internal/icongen 重新生成。
package main
