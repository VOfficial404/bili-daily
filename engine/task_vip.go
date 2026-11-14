package engine

import (
	"strconv"
	"strings"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
)

func init() {
	register(Task{
		Info: api.TaskInfo{
			ID:    TaskVipPoint,
			Title: "大会员大积分",
			Desc:  "每天签到，完成支持的大积分任务；领取正片任务后需在官方 App 观看，另领每日经验。",
			Group: "大会员",
			Needs: "大会员",
		},
		Enabled: func(s *api.Settings) bool { return s.Tasks.VipPoint.Enabled },
		Run:     runVipPoint,
	})
	register(Task{
		Info: api.TaskInfo{
			ID:    TaskVipPrivilege,
			Title: "大会员权益",
			Desc:  "领年度大会员每月送的 B 币券和各种优惠券。",
			Group: "大会员",
			Needs: "年度大会员",
		},
		Enabled: func(s *api.Settings) bool { return s.Tasks.VipPrivilege.Enabled },
		Run:     runVipPrivilege,
	})
	register(Task{
		Info: api.TaskInfo{
			ID:    TaskCharge,
			Title: "B 币券充电",
			Desc:  "每月把 B 币券充给指定的 UP 主，免得月底过期。",
			Group: "大会员",
			Needs: "年度大会员、B 币券",
		},
		Enabled: func(s *api.Settings) bool { return s.Tasks.Charge.Enabled },
		Run:     runCharge,
	})
}

/* 大会员权益 */

// vipPrivilegeNames 是每月可以领的权益，下标是权益的类型
var vipPrivilegeNames = [...]string{1: "B 币券", "会员购优惠券", "漫画福利券", "会员购包邮券", "漫画商城优惠券", "装扮体验卡", "课堂优惠券"}

func runVipPrivilege(x *Ctx) {
	if x.Nav.VIP() != bili.VIPAnnual {
		x.Skip("领取权益", "需要在期年度大会员")
		return
	}
	items, err := x.C.VIPPrivileges(x)
	if err != nil {
		x.Fail("查询权益", err)
		return
	}
	var claimed []string
	allClaimed := len(items) > 0
	for _, it := range items {
		if it.Type < 1 || it.Type >= len(vipPrivilegeNames) {
			allClaimed = false
			continue
		}
		name := vipPrivilegeNames[it.Type]
		switch it.State {
		case bili.VIPPrivilegeClaimed:
			x.Skip(name, "这个月已经领过")
		case bili.VIPPrivilegeUnclaimed:
			err := x.C.VIPReceivePrivilege(x, it.Type)
			switch {
			case err == nil:
				x.OK(name, "领到了")
				claimed = append(claimed, name)
			case bili.IsCode(err, 69801):
				x.Skip(name, "这个月已经领过")
			default:
				allClaimed = false
				x.Fail(name, err)
				if x.Aborted() {
					return
				}
			}
		default:
			allClaimed = false
			x.Skip(name, "条件还没满足，暂时领不了")
		}
	}
	if len(claimed) > 0 {
		x.Note("领到 %s", strings.Join(claimed, "、"))
	} else if allClaimed {
		x.Note("这个月的权益都领过了")
	}
}

/* B 币券充电 */

func runCharge(x *Ctx) {
	if x.Nav.VIP() != bili.VIPAnnual {
		x.Skip("充电", "需要在期年度大会员")
		return
	}
	opt := x.Set.Tasks.Charge
	if opt.UpID == 0 {
		x.Skip("充电", "还没有设置给谁充电")
		return
	}
	if opt.UpID == x.Mid {
		x.Failf("充电", bili.ReasonIneligible, "B 站不允许给自己充电，在设置里换一个 UP 主")
		return
	}
	// x.Day 是 2006-01-02
	if dom, _ := strconv.Atoi(x.Day[8:]); dom < opt.Day {
		x.Skip("充电", "每月 %d 号之后才充电", opt.Day)
		return
	}
	// 重新取一次余额：同一轮里前面的任务可能刚领了这个月的 B 币券
	nav, err := x.C.Nav(x)
	if err != nil {
		x.Fail("查询 B 币券", err)
		return
	}
	if nav.VIP() != bili.VIPAnnual {
		x.Skip("充电", "年度大会员已失效，跳过充电")
		return
	}
	amount := int(nav.Wallet.Coupon)
	if amount < 2 {
		x.Skip("充电", "B 币券不到 2 个（现在有 %s 个）", trimFloat(nav.Wallet.Coupon))
		return
	}
	res, err := x.C.Charge(x, opt.UpID, amount)
	if err != nil {
		x.Fail("充电", err)
		return
	}
	if res.Status != 4 {
		reason := bili.ReasonOther
		if res.Status == -4 {
			reason = bili.ReasonInsufficient
		}
		x.Failf("充电", reason, "B 站没有充上：%s", res.Message)
		return
	}
	x.OK("充电", "给 UID %d 充了 %d 个 B 币", opt.UpID, amount)
	x.Note("给 UID %d 充了 %d 个 B 币", opt.UpID, amount)

	if opt.Comment == "" {
		return
	}
	switch err := x.C.ChargeComment(x, res.OrderNo, opt.Comment); {
	case err == nil:
		x.OK("留言", "%s", opt.Comment)
	case bili.IsCode(err, 88203):
		x.Skip("留言", "已经留过言")
	default:
		x.Fail("留言", err)
	}
}

/* 大积分 */

// vipPointTasks 是不用花钱的大积分任务，按这个顺序处理；观看正片只领取，需用户在官方 App 播放。
// 要购买才能完成的（vipmallbuy、tvodbuy、dressbuyamount）不在里面，连领取都不领。
var vipPointTasks = [...]struct{ code, name string }{
	{"bonus", "大会员福利"},
	{"privilege", "浏览会员权益页"},
	{"dress-view", "浏览装扮商城"},
	{"vipmallview", "浏览会员购"},
	{"animatetab", "浏览追番频道"},
	{"filmtab", "浏览影视频道"},
	{"ogvwatchnew", "观看正片"},
}

// 「观看正片」只领取任务，不自动播放，也不提交完成；领取不计入已完成任务。
const (
	vipPointWatch     = "ogvwatchnew"
	vipPointWatchHelp = "任务已领取，需要在B站官方App连续观看10分钟正片；本工具未自动播放"
)

func runVipPoint(x *Ctx) {
	if x.Nav.VIP() == bili.VIPNone {
		x.Skip("签到", "不是大会员")
		return
	}

	switch signed, err := x.C.VIPPointSigned(x, x.Now()); {
	case err != nil:
		x.Fail("签到", err)
	case signed:
		x.Skip("签到", "今天已签到")
	default:
		if err := x.C.VIPPointSign(x, x.Now()); err != nil {
			x.Fail("签到", err)
		} else {
			x.OK("签到", "签到成功")
		}
	}
	if x.Aborted() {
		return
	}

	done, point, ok := vipPointDoTasks(x)
	if x.Aborted() {
		return
	}
	vipPointExperience(x)
	if ok {
		x.Note("做完 %d 个积分任务，现在有 %d 大积分", done, point)
	}
}

// vipPointDoTasks 做大积分任务，返回这次做完了几个和现在的积分；任务列表取不到时 ok 为 false
func vipPointDoTasks(x *Ctx) (done, point int, ok bool) {
	before, err := x.C.VIPPoints(x, x.Now())
	if err != nil {
		x.Fail("查询积分任务", err)
		return 0, 0, false
	}
	states := before.States()
	var tried []int // vipPointTasks 的下标：报告过完成、等着核对的
	for i, t := range vipPointTasks {
		state, has := states[t.code]
		if !has {
			continue
		}
		if state == bili.VIPPointDone {
			x.Skip(t.name, "已完成")
			continue
		}
		if t.code == vipPointWatch && state != bili.VIPPointUnreceived && state != bili.VIPPointReceived {
			x.Skip(t.name, "B 站尚未标记任务完成（状态 %d）；请在官方 App 查看并观看正片，本工具未自动播放", state)
			continue
		}
		if state == bili.VIPPointUnreceived {
			if err := x.C.VIPPointReceive(x, t.code); err != nil {
				x.Fail(t.name, err)
				if x.Aborted() {
					return 0, 0, false
				}
				continue
			}
		}
		if t.code == vipPointWatch {
			x.Skip(t.name, vipPointWatchHelp)
			continue
		}
		if t.code == "animatetab" || t.code == "filmtab" {
			// 这两个任务要求在频道页停留 10 秒
			if !x.Sleep(10 * time.Second) {
				x.Fail(t.name, x.Err())
				return 0, 0, false
			}
		}
		if err := x.C.VIPPointComplete(x, t.code, x.Now()); err != nil {
			x.Fail(t.name, err)
			if x.Aborted() {
				return 0, 0, false
			}
			continue
		}
		tried = append(tried, i)
	}

	// 接口返回成功不等于记了完成：再取一次任务列表核对
	after, err := x.C.VIPPoints(x, x.Now())
	if err != nil {
		x.Fail("核对积分任务", err)
		return 0, 0, false
	}
	states = after.States()
	for _, i := range tried {
		t := vipPointTasks[i]
		switch {
		case states[t.code] == bili.VIPPointDone:
			x.OK(t.name, "已完成")
			done++
		default:
			x.Failf(t.name, bili.ReasonChanged, "报告了完成，但 B 站没有记为完成")
		}
	}
	return done, after.PointInfo.Point, true
}

// vipPointExperience 领大会员每天的 10 点经验（要求今天看过视频）
func vipPointExperience(x *Ctx) {
	const name = "每日经验"
	items, err := x.C.VIPPrivileges(x)
	if err != nil {
		x.Fail(name, err)
		return
	}
	for _, it := range items {
		if it.Type != bili.VIPPrivilegeExp {
			continue
		}
		switch it.State {
		case bili.VIPPrivilegeClaimed:
			x.Skip(name, "今天已经领过")
		case bili.VIPPrivilegeNotReady:
			x.Skip(name, "今天还没看视频，领不了")
		case bili.VIPPrivilegeUnclaimed:
			switch err := x.C.VIPExperience(x); {
			case err == nil:
				x.OK(name, "经验 +10")
			case bili.IsCode(err, 69198):
				x.Skip(name, "今天已经领过")
			case bili.IsCode(err, 6034005):
				x.Skip(name, "今天还没看视频，领不了")
			default:
				x.Fail(name, err)
			}
		default:
			x.Skip(name, "条件还没满足，暂时领不了")
		}
		return
	}
}
