package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/VOfficial404/bili-tools/api"
)

// DefaultSettings 是没有 settings.json 时的设置：扫码之后不用改任何东西就能用。
// 会花钱、会关注别人、会在别人直播间留言的任务默认关着。
func DefaultSettings() api.Settings {
	return api.Settings{
		RunAt:    "09:30",
		Jitter:   60,
		Pace:     "normal",
		LogLevel: "info",
		Notify:   api.Notify{When: "failed"},
		Tasks: api.TaskSettings{
			Daily: api.DailyOptions{
				Enabled:     true,
				Watch:       true,
				Share:       true,
				Coins:       5,
				StopAtLevel: 6,
				Like:        true,
				UpIDs:       []int64{},
			},
			VipPoint:       api.ToggleOptions{Enabled: true},
			VipPrivilege:   api.ToggleOptions{Enabled: true},
			Charge:         api.ChargeOptions{Day: 28},
			Manga:          api.MangaOptions{Enabled: true, Read: true, ComicID: 27355, EpID: 381662},
			MangaPrivilege: api.ToggleOptions{Enabled: true},
			Silver2Coin:    api.ToggleOptions{Enabled: true},
			FansMedal:      api.FansMedalOptions{Like: true, Watch: true, Minutes: 25, Max: 10},
			LiveLottery: api.LiveLotteryOptions{
				EveryHours: 6,
				Include:    []string{},
				Exclude:    []string{"舰", "船", "航海", "代金券", "优惠券", "自拍", "照", "写真", "图", "提督"},
				Group:      true,
			},
			Unfollow: api.UnfollowOptions{Group: lotteryGroup, Count: 20, Keep: []int64{}},
		},
	}
}

// lotteryGroup 是放因为抽奖而关注的主播的分组
const lotteryGroup = "天选时刻"

var (
	runAtRe   = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	paces     = []string{"slow", "normal", "fast"}
	whens     = []string{"never", "failed", "always"}
	channels  = []string{"", "telegram", "pushplus", "wecom", "dingtalk", "serverchan", "bark", "webhook"}
	errNoPush = errors.New("还没有选择推送渠道")
)

// checkSettings 整理并检查设置，不对时返回给人看的原因
func checkSettings(s *api.Settings) error {
	s.RunAt = strings.TrimSpace(s.RunAt)
	if !runAtRe.MatchString(s.RunAt) {
		return errors.New("开始时间要写成 HH:MM，例如 09:30")
	}
	if s.Jitter < 0 || s.Jitter > 720 {
		return errors.New("随机推迟要在 0 到 720 分钟之间")
	}
	if !slices.Contains(paces, s.Pace) {
		return errors.New("请求节奏只能是 slow、normal 或 fast")
	}
	if !validLevel(s.LogLevel) {
		return errors.New("日志级别只能是 debug、info、warn 或 error")
	}
	s.Proxy = strings.TrimSpace(s.Proxy)
	if _, err := ParseProxy(s.Proxy); err != nil {
		return err
	}

	n := &s.Notify
	n.Token, n.Target, n.Endpoint = strings.TrimSpace(n.Token), strings.TrimSpace(n.Target), strings.TrimRight(strings.TrimSpace(n.Endpoint), "/")
	if !slices.Contains(whens, n.When) {
		return errors.New("推送时机只能是 never、failed 或 always")
	}
	if !slices.Contains(channels, n.Channel) {
		return fmt.Errorf("不支持的推送渠道 %q", n.Channel)
	}
	if n.Endpoint != "" && !strings.HasPrefix(n.Endpoint, "http://") && !strings.HasPrefix(n.Endpoint, "https://") {
		return errors.New("推送服务地址要以 http:// 或 https:// 开头")
	}

	t := &s.Tasks
	d := &t.Daily
	if d.Coins < 0 || d.Coins > 5 {
		return errors.New("每天投币数要在 0 到 5 之间（超过 5 个不再加经验）")
	}
	if d.KeepCoins < 0 {
		return errors.New("保留的硬币数不能是负数")
	}
	if d.StopAtLevel < 0 || d.StopAtLevel > 6 {
		return errors.New("停止投币的等级要在 0 到 6 之间")
	}
	d.UpIDs = cleanIDs(d.UpIDs)

	if t.Charge.Day < 1 || t.Charge.Day > 28 {
		return errors.New("充电日期要在 1 到 28 号之间")
	}
	if t.Charge.UpID < 0 {
		return errors.New("充电对象的 UID 不对")
	}
	if t.Charge.Enabled && t.Charge.UpID == 0 {
		return errors.New("打开 B 币券充电之前，先填写给谁充电（UP 主的 UID）")
	}
	t.Charge.Comment = strings.TrimSpace(t.Charge.Comment)

	if t.Manga.Read && (t.Manga.ComicID <= 0 || t.Manga.EpID <= 0) {
		return errors.New("打开漫画阅读之前，先填写漫画和章节的 ID")
	}

	f := &t.FansMedal
	f.Danmaku = strings.TrimSpace(f.Danmaku)
	if utf8.RuneCountInString(f.Danmaku) > 30 {
		return errors.New("弹幕不能超过 30 个字")
	}
	if f.Minutes < 1 || f.Minutes > 120 {
		return errors.New("每个粉丝牌的观看时长要在 1 到 120 分钟之间")
	}
	if f.Max < 0 || f.Max > 200 {
		return errors.New("每轮处理的粉丝牌数量要在 0 到 200 之间")
	}

	l := &t.LiveLottery
	if l.EveryHours < 0 || l.EveryHours > 24 {
		return errors.New("天选时刻的间隔要在 0 到 24 小时之间")
	}
	l.Include, l.Exclude = cleanWords(l.Include), cleanWords(l.Exclude)

	u := &t.Unfollow
	u.Group = strings.TrimSpace(u.Group)
	if u.Count < 0 {
		return errors.New("取关数量不能是负数")
	}
	u.Keep = cleanIDs(u.Keep)
	return nil
}

// cleanIDs 去掉不是正数的和重复的
func cleanIDs(ids []int64) []int64 {
	out := []int64{}
	for _, id := range ids {
		if id > 0 && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// cleanWords 去掉空的和重复的
func cleanWords(words []string) []string {
	out := []string{}
	for _, w := range words {
		if w = strings.TrimSpace(w); w != "" && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

func (e *Engine) settingsFile() string { return filepath.Join(e.cfg.DataDir, "settings.json") }

// loadSettings 读 settings.json；文件里没有的项用默认值，文件不对时整个用默认值
func (e *Engine) loadSettings() api.Settings {
	s := DefaultSettings()
	err := readJSON(e.settingsFile(), &s)
	if err == nil {
		err = checkSettings(&s)
	}
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			// 这时日志还没有准备好，直接写到标准错误
			fmt.Fprintf(os.Stderr, "settings.json 无法使用，改用默认设置：%v\n", err)
		}
		s = DefaultSettings()
	}
	return s
}

// UpdateSettings 检查、保存并立即应用新设置
func (e *Engine) UpdateSettings(s api.Settings) (api.Settings, error) {
	if err := checkSettings(&s); err != nil {
		return s, err
	}
	if err := writeJSON(e.settingsFile(), s); err != nil {
		return s, fmt.Errorf("保存设置失败：%w", err)
	}
	e.mu.Lock()
	old := e.set
	e.set = s
	e.mu.Unlock()
	e.logs.setLevel(parseLevel(s.LogLevel))
	if old.Proxy != s.Proxy {
		select {
		case e.egressKick <- struct{}{}:
		default:
		}
	}
	// 开始时间、任务开关可能变了，让每个账号重新算下一次运行
	for _, a := range e.accountList() {
		a.poke()
	}
	return s, nil
}
