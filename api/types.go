// Package api 是服务端接口的数据类型和 Go 客户端。
// 服务端（engine）、桌面客户端和手机 App 用同一份类型，接口的说明见 docs/DESIGN.md。
package api

import "time"

// Status 是一步、一个任务或一轮运行的结果
type Status string

const (
	StatusOK      Status = "ok"
	StatusPartial Status = "partial" // 有成功也有失败
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped" // 不需要做：已经做过、没开、不满足条件
	StatusRunning Status = "running"
)

// Ping 是 GET /api/ping 的响应，不需要令牌，客户端用它确认地址对不对
type Ping struct {
	Name    string `json:"name"` // 固定为 bili-daily
	Version string `json:"version"`
}

// State 是 GET /api/state 的响应：界面要显示的全部状态
type State struct {
	Version  string     `json:"version"`
	Local    bool       `json:"local"` // 在桌面或手机 App 里运行，而不是服务器上
	Now      time.Time  `json:"now"`
	Accounts []Account  `json:"accounts"`
	Tasks    []TaskInfo `json:"tasks"`
	Settings Settings   `json:"settings"`
	Egress   *Egress    `json:"egress,omitempty"`
}

// Egress 是 B 站看到的出口 IP
type Egress struct {
	Addr      string    `json:"addr"`
	Place     string    `json:"place"` // 例如「广东 深圳」
	ISP       string    `json:"isp"`
	CheckedAt time.Time `json:"checked_at"`
	Error     string    `json:"error,omitempty"`
}

// TaskInfo 描述一个任务，界面按它列出任务
type TaskInfo struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Desc   string `json:"desc"`
	Group  string `json:"group"`  // 每日、大会员、漫画、直播、工具
	Manual bool   `json:"manual"` // 只能手动运行，不参加每日计划
	Needs  string `json:"needs"`  // 对账号的要求，例如「大会员」，没有时为空
}

type Account struct {
	Mid           string       `json:"mid"`
	Uname         string       `json:"uname"`
	Face          string       `json:"face"`
	Enabled       bool         `json:"enabled"`        // 参加每日计划
	DisabledTasks []string     `json:"disabled_tasks"` // 这个账号不做的任务
	LoggedIn      bool         `json:"logged_in"`
	Level         int          `json:"level"`
	Exp           int          `json:"exp"`
	NextExp       int          `json:"next_exp"` // 升到下一级需要的总经验，6 级时为 0
	Coins         float64      `json:"coins"`
	BCoupon       float64      `json:"b_coupon"` // B 币券余额
	VIP           string       `json:"vip"`      // 「月度大会员」「年度大会员」，不是时为空
	VIPDue        *time.Time   `json:"vip_due,omitempty"`
	CookieExpires *time.Time   `json:"cookie_expires,omitempty"`
	UpdatedAt     *time.Time   `json:"updated_at,omitempty"` // 上面这些资料的更新时间
	Running       *Progress    `json:"running,omitempty"`
	NextRun       *time.Time   `json:"next_run,omitempty"`
	LastRun       *RunSummary  `json:"last_run,omitempty"`
	Today         []TaskStatus `json:"today"` // 今天（北京时间）各任务的结果
}

// Progress 是正在进行的一轮
type Progress struct {
	RunID   string    `json:"run_id"`
	Started time.Time `json:"started"`
	Task    string    `json:"task"` // 正在做的任务的名字
	Done    int       `json:"done"`
	Total   int       `json:"total"`
}

// TaskStatus 是一个任务今天最近一次的结果
type TaskStatus struct {
	ID     string `json:"id"`
	Status Status `json:"status"`
	Note   string `json:"note"` // 一句话结果
}

/* 运行报告 */

type RunSummary struct {
	ID      string    `json:"id"`
	Mid     string    `json:"mid"`
	Uname   string    `json:"uname"`
	Trigger string    `json:"trigger"` // schedule 定时、manual 手动、catchup 补跑
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
	Status  Status    `json:"status"`
	Summary string    `json:"summary"` // 例如「6 个任务成功，1 个失败；经验 +65」
}

// Run 是一轮运行的完整报告
type Run struct {
	RunSummary
	Tasks []TaskResult `json:"tasks"`
}

type TaskResult struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Status  Status    `json:"status"`
	Note    string    `json:"note"` // 一句话结果
	Started time.Time `json:"started"`
	Millis  int64     `json:"millis"`
	Steps   []Step    `json:"steps"`
}

// Step 是任务里的一步。失败时 Reason 和 Hint 说明原因和怎么办。
type Step struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"`
	Reason string `json:"reason,omitempty"` // not_login、risk、rate、ineligible、insufficient、network、changed、canceled、other
	Hint   string `json:"hint,omitempty"`
	Code   int    `json:"code,omitempty"` // B 站返回的 code
}

/* 设置 */

type Settings struct {
	RunAt    string       `json:"run_at"`    // 每天开始运行的时间，北京时间，HH:MM
	Jitter   int          `json:"jitter"`    // 在 RunAt 之后随机推迟 0~Jitter 分钟，每个账号每天不同
	Pace     string       `json:"pace"`      // 请求节奏：slow、normal、fast
	Proxy    string       `json:"proxy"`     // 所有 B 站请求经过的代理，空表示直连
	LogLevel string       `json:"log_level"` // debug、info、warn、error
	Notify   Notify       `json:"notify"`
	Tasks    TaskSettings `json:"tasks"`
}

// Notify 是运行结果推送
type Notify struct {
	When     string `json:"when"`     // never、failed（有失败时）、always
	Channel  string `json:"channel"`  // telegram、pushplus、wecom、dingtalk、serverchan、bark、webhook
	Token    string `json:"token"`    // 各渠道的密钥、机器人 token 或 webhook 地址
	Target   string `json:"target"`   // Telegram 的 chat_id；钉钉的加签密钥（没开加签时留空）；其他渠道不用
	Endpoint string `json:"endpoint"` // 自建服务的地址（Telegram 反代、Bark 服务器），空表示官方
}

type TaskSettings struct {
	Daily          DailyOptions       `json:"daily"`
	VipPoint       ToggleOptions      `json:"vip_point"`
	VipPrivilege   ToggleOptions      `json:"vip_privilege"`
	Charge         ChargeOptions      `json:"charge"`
	Manga          MangaOptions       `json:"manga"`
	MangaPrivilege ToggleOptions      `json:"manga_privilege"`
	Silver2Coin    ToggleOptions      `json:"silver2coin"`
	FansMedal      FansMedalOptions   `json:"fans_medal"`
	LiveLottery    LiveLotteryOptions `json:"live_lottery"`
	Unfollow       UnfollowOptions    `json:"unfollow"`
}

// ToggleOptions 是只有开关的任务
type ToggleOptions struct {
	Enabled bool `json:"enabled"`
}

type DailyOptions struct {
	Enabled     bool    `json:"enabled"`
	Watch       bool    `json:"watch"`
	Share       bool    `json:"share"`
	Coins       int     `json:"coins"`         // 每天投几个币，0~5
	KeepCoins   int     `json:"keep_coins"`    // 至少留下这么多硬币
	StopAtLevel int     `json:"stop_at_level"` // 到这个等级后不再投币，0 表示一直投
	Like        bool    `json:"like"`          // 投币时顺便点赞
	UpIDs       []int64 `json:"up_ids"`        // 优先看、投这些 UP 主的视频；空时从关注和排行榜里选
}

type ChargeOptions struct {
	Enabled bool   `json:"enabled"`
	UpID    int64  `json:"up_id"`   // 给谁充电；0 时不充
	Day     int    `json:"day"`     // 每月几号之后充，1~28
	Comment string `json:"comment"` // 充电留言，空表示不留言
}

type MangaOptions struct {
	Enabled bool  `json:"enabled"`
	Read    bool  `json:"read"`     // 签到之外再记一次阅读
	ComicID int64 `json:"comic_id"` // 阅读哪部漫画
	EpID    int64 `json:"ep_id"`    // 的哪一话
}

type FansMedalOptions struct {
	Enabled bool   `json:"enabled"`
	Like    bool   `json:"like"`    // 给直播间点赞
	Danmaku string `json:"danmaku"` // 发的弹幕，空表示不发
	Watch   bool   `json:"watch"`   // 挂在直播间里攒观看时长
	Minutes int    `json:"minutes"` // 每个粉丝牌最多挂几分钟
	Max     int    `json:"max"`     // 每轮最多处理几个粉丝牌，0 表示全部
}

type LiveLotteryOptions struct {
	Enabled    bool     `json:"enabled"`
	EveryHours int      `json:"every_hours"` // 每隔几小时找一次，0 表示只在每日运行时找
	Include    []string `json:"include"`     // 奖品名包含任一关键词才参加；空表示都参加
	Exclude    []string `json:"exclude"`     // 奖品名包含任一关键词就不参加
	Follow     bool     `json:"follow"`      // 参加需要关注主播的抽奖
	Group      bool     `json:"group"`       // 把因为抽奖关注的主播放进「天选时刻」分组，方便之后批量取关
}

// UnfollowOptions 是批量取关，只能手动运行
type UnfollowOptions struct {
	Group string  `json:"group"` // 取关哪个分组里的人
	Count int     `json:"count"` // 最多取关几个，0 表示全部
	Keep  []int64 `json:"keep"`  // 不取关这些人
}

/* 请求和响应 */

// QR 是一张二维码。扫码登录和状态查询共用。
type QR struct {
	ID    string `json:"id"`
	PNG   []byte `json:"png,omitempty"` // 二维码图片
	URL   string `json:"url,omitempty"` // 二维码内容，手机上可以直接用它打开 B 站 App
	State string `json:"state"`         // waiting、scanned、expired、success
	Mid   string `json:"mid,omitempty"` // 成功时是登录的账号
}

// RunRequest 是 POST /api/accounts/{mid}/run 的请求。Tasks 为空表示所有开着的任务。
type RunRequest struct {
	Tasks []string `json:"tasks"`
}

// AccountPatch 是 PATCH /api/accounts/{mid} 的请求，nil 的字段不改
type AccountPatch struct {
	Enabled       *bool     `json:"enabled,omitempty"`
	DisabledTasks *[]string `json:"disabled_tasks,omitempty"`
}

type LogEntry struct {
	Seq   int64     `json:"seq"`
	Time  time.Time `json:"time"`
	Level string    `json:"level"`
	Mid   string    `json:"mid,omitempty"`
	Text  string    `json:"text"`
}

// Group 是关注分组，批量取关时选
type Group struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Error 是出错时的响应，也是客户端返回的错误
type Error struct {
	Status  int    `json:"-"` // HTTP 状态码
	Message string `json:"error"`
}

func (e *Error) Error() string { return e.Message }
