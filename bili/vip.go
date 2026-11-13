package bili

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

// 大会员相关的接口，资料见 docs/research/api-vip-manga.md。都只用 Cookie。

const vipTaskReferer = "https://big.bilibili.com/mobile/bigPoint/task"

var vipTaskHeader = map[string]string{"Referer": vipTaskReferer}

// 大会员权益的状态
const (
	VIPPrivilegeUnclaimed = 0 // 可以领
	VIPPrivilegeClaimed   = 1
	VIPPrivilegeNotReady  = 2 // 条件还没满足
)

// VIPPrivilegeExp 是「每天 10 点经验」在权益列表里的类型
const VIPPrivilegeExp = 9

// VIPPrivilege 是大会员权益列表里的一项
type VIPPrivilege struct {
	Type  int `json:"type"`
	State int `json:"state"`
}

// VIPPrivileges 列出大会员权益和它们的领取状态
func (c *Client) VIPPrivileges(ctx context.Context) ([]VIPPrivilege, error) {
	var d struct {
		List []VIPPrivilege `json:"list"`
	}
	err := c.Do(ctx, Request{URL: "https://api.bilibili.com/x/vip/privilege/my", Header: vipTaskHeader}, &d)
	return d.List, err
}

// VIPReceivePrivilege 领一项每月的权益。已经领过时 B 站返回 69801。
func (c *Client) VIPReceivePrivilege(ctx context.Context, typ int) error {
	return c.PostForm(ctx, "https://api.bilibili.com/x/vip/privilege/receive", url.Values{"type": {strconv.Itoa(typ)}}, nil)
}

// VIPExperience 领大会员每天的 10 点经验。已经领过是 69198，今天还没看视频是 6034005。
func (c *Client) VIPExperience(ctx context.Context) error {
	return c.Do(ctx, Request{URL: "https://api.bilibili.com/x/vip/experience/add", Form: url.Values{}, Header: vipTaskHeader}, nil)
}

// ChargeResult 是充电的结果。Status 为 4 才是充上了；-4 是 B 币不够。
type ChargeResult struct {
	Status  int    `json:"status"`
	Message string `json:"msg"`
	OrderNo string `json:"order_no"`
}

var payHeader = map[string]string{"Origin": "https://www.bilibili.com"}

// Charge 用 B 币（优先用 B 币券）给 UP 主充电，amount 是几个 B 币，最少 2 个
func (c *Client) Charge(ctx context.Context, up int64, amount int) (ChargeResult, error) {
	var out ChargeResult
	uid := strconv.FormatInt(up, 10)
	err := c.Do(ctx, Request{
		URL: "https://api.bilibili.com/x/ugcpay/web/v2/trade/elec/pay/quick",
		Form: url.Values{
			"bp_num":              {strconv.Itoa(amount)},
			"is_bp_remains_prior": {"true"},
			"up_mid":              {uid},
			"otype":               {"up"},
			"oid":                 {uid},
		},
		Header: payHeader,
	}, &out)
	return out, err
}

// ChargeComment 给一次充电留言。重复留言时 B 站返回 88203。
func (c *Client) ChargeComment(ctx context.Context, orderNo, message string) error {
	return c.Do(ctx, Request{
		URL:    "https://api.bilibili.com/x/ugcpay/trade/elec/message",
		Form:   url.Values{"order_id": {orderNo}, "message": {message}},
		Header: payHeader,
	}, nil)
}

/* 大积分 */

// vipPointParams 是大积分页面的请求都带的参数（页面是给手机 App 里的网页用的）
func vipPointParams(now time.Time) url.Values {
	ms := now.UnixMilli()
	return url.Values{
		"build":        {"8451100"},
		"disable_rcmd": {"0"},
		"mobi_app":     {"android"},
		"platform":     {"android"},
		"statistics":   {`{"appId":1,"platform":3,"version":"8.45.1","abtest":""}`},
		"t":            {strconv.FormatInt(ms, 10)},
		"ts":           {strconv.FormatInt(ms/1000, 10)},
	}
}

// VIPPointSigned 返回今天有没有在大积分页面签到
func (c *Client) VIPPointSigned(ctx context.Context, now time.Time) (bool, error) {
	var d struct {
		Sign struct {
			Signed bool `json:"signed"`
		} `json:"three_day_sign"`
	}
	q := vipPointParams(now)
	q.Set("csrf", c.CSRF())
	err := c.Do(ctx, Request{URL: "https://api.bilibili.com/x/vip/vip_center/sign_in/three_days_sign", Query: q, Header: vipTaskHeader}, &d)
	return d.Sign.Signed, err
}

// VIPPointSign 大积分签到
func (c *Client) VIPPointSign(ctx context.Context, now time.Time) error {
	ms := now.UnixMilli()
	return c.Do(ctx, Request{
		URL:    "https://api.bilibili.com/pgc/activity/score/task/sign2",
		Query:  url.Values{"mobi_app": {"android"}, "platform": {"android"}, "csrf": {c.CSRF()}},
		JSON:   map[string]any{"device": "phone", "t": ms, "ts": ms / 1000},
		Header: map[string]string{"Referer": "https://big.bilibili.com/mobile/index"},
	}, nil)
}

// 大积分任务的状态
const (
	VIPPointUnreceived = 0 // 还没领取任务
	VIPPointDone       = 3
)

// VIPPointTask 是一个大积分任务
type VIPPointTask struct {
	Code  string `json:"task_code"`
	State int    `json:"state"`
}

// VIPPoints 是大积分页面的内容：积分余额和各个任务
type VIPPoints struct {
	PointInfo struct {
		Point int `json:"point"`
	} `json:"point_info"`
	TaskInfo struct {
		Modules []struct {
			Items []VIPPointTask `json:"common_task_item"`
		} `json:"modules"`
	} `json:"task_info"`
}

// States 返回各任务的状态，键是 task_code
func (p *VIPPoints) States() map[string]int {
	states := map[string]int{}
	for _, m := range p.TaskInfo.Modules {
		for _, it := range m.Items {
			states[it.Code] = it.State
		}
	}
	return states
}

// VIPPoints 取大积分余额和任务列表
func (c *Client) VIPPoints(ctx context.Context, now time.Time) (*VIPPoints, error) {
	q := vipPointParams(now)
	q.Set("csrf", c.CSRF())
	q.Set("buvid", c.Cookie("buvid3"))
	q.Set("brand", "Samsung")
	q.Set("channel", "bili")
	q.Set("containerName", "AbstractWebActivity")
	q.Set("device", "phone")
	var out VIPPoints
	if err := c.Do(ctx, Request{URL: "https://api.bilibili.com/x/vip_point/task/combine", Query: q, Header: vipTaskHeader}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// VIPPointReceive 领取一个大积分任务，之后才能完成它
func (c *Client) VIPPointReceive(ctx context.Context, code string) error {
	return c.Do(ctx, Request{
		URL:    "https://api.bilibili.com/pgc/activity/score/task/receive/v2",
		Form:   url.Values{"taskCode": {code}},
		Header: vipTaskHeader,
	}, nil)
}

// VIPPointComplete 报告一个大积分任务做完了。每种任务报告的接口不一样；
// 接口返回成功不等于 B 站记了完成，要再看一次 VIPPoints。
func (c *Client) VIPPointComplete(ctx context.Context, code string, now time.Time) error {
	r := Request{Header: vipTaskHeader}
	switch code {
	case "bonus", "privilege":
		r.URL = "https://api.bilibili.com/pgc/activity/score/task/complete"
		r.JSON = map[string]string{"taskCode": code}
	case "dress-view", "ogvwatchnew":
		r.URL = "https://api.bilibili.com/pgc/activity/score/task/complete/v2"
		r.Form = url.Values{"taskCode": {code}}
	case "vipmallview":
		r.URL = "https://show.bilibili.com/api/activity/fire/common/event/dispatch"
		r.JSON = map[string]string{"csrf": c.CSRF(), "eventId": "hevent_oy4b7h3epeb"}
		r.Header = nil
	case "animatetab", "filmtab":
		r.URL = "https://api.bilibili.com/pgc/activity/deliver/task/complete"
		r.Form = vipPointParams(now)
		r.Form.Set("position", map[string]string{"animatetab": "jp_channel", "filmtab": "tv_channel"}[code])
		r.Form.Set("c_locale", "zh_CN")
		r.Form.Set("s_locale", "zh_CN")
		r.Form.Set("channel", "bili")
	default:
		return &Error{Code: CodeBadResponse, Message: "不认识的大积分任务 " + code}
	}
	return c.Do(ctx, r, nil)
}
