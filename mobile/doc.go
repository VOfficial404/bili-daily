// Bili Daily 手机客户端使用同一套 HTTP 接口连接服务器或进程内引擎。
// 当前 NSGo 实现作为过渡版本保留，待 MyGo 的手机支持 PR #940 合入后迁移。
//
// 手机推荐连接服务器。本机任务只能在 App 打开时可靠运行；手机系统可能暂停后台运行，
// 稳定定时请连接服务器，回到 App 时补跑。保留进程内引擎可让扫码切换 App 后继续确认。
// 网页没有本机数据目录，只提供服务器连接。没有后台保活和定时本地通知。
// 服务端令牌仅在用户勾选记住时写入 NSGo 的明文私有存储，断开连接会清除它。
// 本机代理保存在私有存储，下次启动自动回填。设置表单只合并自上次保存后修改的字段；
// 开启自动充电或更换充电 UID 前会确认每月使用全部 B 币券余额。停止本机运行会等报告写入完成。
// 优先 UP 主与充电对象可从已登录账号的关注中选择，或粘贴 UID、空间链接；顺序和选择随表单保存。
// 关注支持按昵称搜索与手动加载更多，选择后立即显示资料，未保存时离开页面即放弃修改。
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
// 图标由仓库根目录的 go run ./internal/icongen 生成。
package main
