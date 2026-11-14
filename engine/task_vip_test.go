package engine

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili/bilitest"
)

const (
	vipPrivilegePath       = "api.bilibili.com/x/vip/privilege/my"
	vipReceivePath         = "api.bilibili.com/x/vip/privilege/receive"
	chargePath             = "api.bilibili.com/x/ugcpay/web/v2/trade/elec/pay/quick"
	chargeCommentPath      = "api.bilibili.com/x/ugcpay/trade/elec/message"
	vipPointCombinePath    = "api.bilibili.com/x/vip_point/task/combine"
	vipPointReceivePath    = "api.bilibili.com/pgc/activity/score/task/receive/v2"
	vipPointCompletePath   = "api.bilibili.com/pgc/activity/score/task/complete"
	vipPointCompleteV2Path = "api.bilibili.com/pgc/activity/score/task/complete/v2"
	vipPointSignPath       = "api.bilibili.com/pgc/activity/score/task/sign2"
	vipPointSignedPath     = "api.bilibili.com/x/vip/vip_center/sign_in/three_days_sign"
)

func TestVipPrivilegeNonVIP(t *testing.T) {
	v := newEnv(t)
	v.login(42)
	r := v.runTask(42, TaskVipPrivilege)
	if r.Status != api.StatusSkipped || v.b.Count(vipPrivilegePath) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestVipPrivilegeClaimsOnlyAvailableAndContinuesAfterRisk(t *testing.T) {
	v := newEnv(t)
	v.login(42)
	v.vip(42, 1, 0)
	v.b.JSON(vipPrivilegePath, map[string]any{"list": []map[string]int{{"type": 1, "state": 0}, {"type": 2, "state": 1}, {"type": 3, "state": 2}, {"type": 4, "state": 0}, {"type": 5, "state": 0}, {"type": 6, "state": 0}, {"type": 9, "state": 0}}})
	v.b.Handle(vipReceivePath, func(c bilitest.Call) bilitest.Reply {
		if c.Method != "POST" || c.Form.Get("csrf") != "csrf-42" {
			t.Errorf("请求不对：%+v", c)
		}
		switch c.Form.Get("type") {
		case "1":
			return bilitest.Code(69801, "已领")
		case "4":
			return bilitest.Code(-352, "风控")
		default:
			return bilitest.OK(nil)
		}
	})
	r := v.runTask(42, TaskVipPrivilege)
	var types []string
	for _, c := range v.b.Calls(vipReceivePath) {
		types = append(types, c.Form.Get("type"))
	}
	if strings.Join(types, ",") != "1,4,5,6" || step(t, r, "B 币券").Status != api.StatusSkipped || step(t, r, "会员购包邮券").Reason != "risk" || r.Status != api.StatusPartial || !strings.Contains(r.Note, "漫画商城优惠券") {
		t.Fatalf("%v %+v", types, r)
	}
}

func TestVipPrivilegeAlreadyDone(t *testing.T) {
	for _, state := range []int{0, 1} {
		t.Run(strconv.Itoa(state), func(t *testing.T) {
			v := newEnv(t)
			v.login(42)
			v.vip(42, 2, 0)
			v.b.JSON(vipPrivilegePath, map[string]any{"list": []map[string]int{{"type": 1, "state": 1}, {"type": 2, "state": state}}})
			v.b.Fail(vipReceivePath, 69801, "已领")
			r := v.runTask(42, TaskVipPrivilege)
			wantCalls := 0
			if state == 0 {
				wantCalls = 1
			}
			if r.Status != api.StatusSkipped || v.b.Count(vipReceivePath) != wantCalls || r.Note != "这个月的权益都领过了" {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestVipPrivilegeDoesNotClaimAllDoneWithUnfinishedItems(t *testing.T) {
	for _, tc := range []struct {
		name   string
		items  []map[string]int
		code   int
		status api.Status
		note   string
	}{
		{"条件未满足", []map[string]int{{"type": 1, "state": 1}, {"type": 2, "state": 2}}, 0, api.StatusSkipped, "条件还没满足，暂时领不了"},
		{"领取失败", []map[string]int{{"type": 1, "state": 1}, {"type": 2, "state": 0}}, -352, api.StatusFailed, "会员购优惠券失败"},
		{"失败和未满足", []map[string]int{{"type": 1, "state": 0}, {"type": 2, "state": 2}}, -352, api.StatusFailed, "B 币券失败"},
		{"空列表", nil, 0, api.StatusSkipped, "没有要做的"},
		{"未知权益", []map[string]int{{"type": 9, "state": 0}}, 0, api.StatusSkipped, "没有要做的"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newEnv(t)
			v.login(42)
			v.vip(42, 2, 0)
			v.b.JSON(vipPrivilegePath, map[string]any{"list": tc.items})
			v.b.Fail(vipReceivePath, tc.code, "风控")
			r := v.runTask(42, TaskVipPrivilege)
			if r.Status != tc.status || !strings.HasPrefix(r.Note, tc.note) || strings.Contains(r.Note, "这个月的权益都领过了") {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func chargeEnv(t *testing.T) *testEnv {
	v := newEnv(t)
	v.login(42)
	v.vip(42, 2, 5.9)
	v.set(func(s *api.Settings) {
		s.Tasks.Charge = api.ChargeOptions{Enabled: true, UpID: 77, Day: 1, Comment: "谢谢"}
	})
	return v
}

func TestChargeGates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		up     int64
		day    int
		coupon float64
		status api.Status
		reason string
	}{{"未设置", 0, 1, 5, api.StatusSkipped, ""}, {"自己", 42, 1, 5, api.StatusFailed, "ineligible"}, {"日期", 77, 28, 5, api.StatusSkipped, ""}, {"余额", 77, 1, 1.99, api.StatusSkipped, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			v := chargeEnv(t)
			v.vip(42, 2, tc.coupon)
			v.set(func(s *api.Settings) {
				s.Tasks.Charge.UpID = tc.up
				s.Tasks.Charge.Day = tc.day
				s.Tasks.Charge.Enabled = tc.up != 0
			})
			r := v.runTask(42, TaskCharge)
			if r.Status != tc.status || v.b.Count(chargePath) != 0 || (tc.reason != "" && step(t, r, "充电").Reason != tc.reason) {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestChargeFreshBalanceAndComment(t *testing.T) {
	v := chargeEnv(t)
	count := 0
	v.b.Handle(navPath, func(c bilitest.Call) bilitest.Reply {
		count++
		n := navData(42)
		n["vipStatus"] = 1
		n["vipType"] = 2
		balance := 0.0
		if count > 1 {
			balance = 5.9
		}
		n["wallet"] = map[string]any{"coupon_balance": balance}
		return bilitest.OK(n)
	})
	v.b.JSON(chargePath, map[string]any{"status": 4, "order_no": "order-123"})
	v.b.Fail(chargeCommentPath, 88203, "重复")
	r := v.runTask(42, TaskCharge)
	calls := v.b.Calls(chargePath)
	if len(calls) != 1 {
		t.Fatalf("请求数 %d：%+v", len(calls), r)
	}
	c := calls[0]
	if c.Form.Get("bp_num") != "5" || c.Form.Get("up_mid") != "77" || c.Form.Get("oid") != "77" || c.Form.Get("otype") != "up" || c.Form.Get("is_bp_remains_prior") != "true" {
		t.Fatalf("%+v", c)
	}
	comments := v.b.Calls(chargeCommentPath)
	if len(comments) != 1 || comments[0].Form.Get("order_id") != "order-123" || comments[0].Form.Get("message") != "谢谢" || r.Note != "给 UID 77 充了 5 个 B 币" || step(t, r, "留言").Status != api.StatusSkipped {
		t.Fatalf("%+v %+v", comments, r)
	}
}

func TestChargePaymentAndCommentFailures(t *testing.T) {
	for _, status := range []int{-4, 1, 4} {
		t.Run(string(rune('a'+status+4)), func(t *testing.T) {
			v := chargeEnv(t)
			v.b.JSON(chargePath, map[string]any{"status": status, "msg": "没有付款", "order_no": "abc"})
			v.b.Fail(chargeCommentPath, -352, "风控")
			r := v.runTask(42, TaskCharge)
			if status == 4 {
				if r.Status != api.StatusPartial || step(t, r, "留言").Reason != "risk" {
					t.Fatalf("%+v", r)
				}
			} else {
				reason := "other"
				if status == -4 {
					reason = "insufficient"
				}
				if step(t, r, "充电").Reason != reason || v.b.Count(chargeCommentPath) != 0 {
					t.Fatalf("%+v", r)
				}
			}
		})
	}
}

func vipPointData(states map[string]int) map[string]any {
	var items []map[string]any
	for code, state := range states {
		items = append(items, map[string]any{"task_code": code, "state": state})
	}
	return map[string]any{"point_info": map[string]int{"point": 456}, "task_info": map[string]any{"modules": []map[string]any{{"common_task_item": items}}}}
}

func vipPointEnv(t *testing.T, signed bool) *testEnv {
	v := newEnv(t)
	v.login(42)
	v.vip(42, 1, 0)
	v.b.JSON(vipPointSignedPath, map[string]any{"three_day_sign": map[string]bool{"signed": signed}})
	v.b.JSON(vipPointSignPath, map[string]any{})
	v.b.JSON(vipPrivilegePath, map[string]any{"list": []map[string]int{{"type": 9, "state": 1}}})
	return v
}

func TestVipPointNonVIP(t *testing.T) {
	v := newEnv(t)
	v.login(42)
	r := v.runTask(42, TaskVipPoint)
	if r.Status != api.StatusSkipped || v.b.Count(vipPointSignedPath) != 0 || v.b.Count(vipPointCombinePath) != 0 || v.b.Count(vipPrivilegePath) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestVipPointReceivesCompletesAndVerifies(t *testing.T) {
	v := vipPointEnv(t, false)
	states := map[string]int{"bonus": 0, "privilege": 1, "dress-view": 0, "vipmallview": 0, "animatetab": 0, "filmtab": 0, "ogvwatchnew": 0, "vipmallbuy": 0, "tvodbuy": 0, "dressbuyamount": 0}
	n := 0
	v.b.Handle(vipPointCombinePath, func(c bilitest.Call) bilitest.Reply {
		if c.Query.Get("buvid") != "buvid3-42" || c.Query.Get("brand") != "Samsung" || c.Query.Get("csrf") != "csrf-42" || c.Header.Get("Referer") != "https://big.bilibili.com/mobile/bigPoint/task" {
			t.Errorf("%+v", c)
		}
		n++
		if n == 2 {
			for _, code := range []string{"bonus", "privilege", "dress-view", "vipmallview", "animatetab", "filmtab", "ogvwatchnew"} {
				states[code] = 3
			}
		}
		return bilitest.OK(vipPointData(states))
	})
	v.b.JSON(vipPointReceivePath, nil)
	v.b.JSON(vipPointCompletePath, nil)
	v.b.JSON(vipPointCompleteV2Path, nil)
	v.b.JSON("show.bilibili.com/api/activity/fire/common/event/dispatch", nil)
	v.b.JSON("api.bilibili.com/pgc/activity/deliver/task/complete", nil)
	r := v.runTask(42, TaskVipPoint)
	if r.Status != api.StatusOK || r.Note != "做完 7 个积分任务，现在有 456 大积分" {
		t.Fatalf("%+v", r)
	}
	var received []string
	for _, c := range v.b.Calls(vipPointReceivePath) {
		received = append(received, c.Form.Get("taskCode"))
	}
	if strings.Join(received, ",") != "bonus,dress-view,vipmallview,animatetab,filmtab,ogvwatchnew" {
		t.Fatalf("%v", received)
	}
	seen := map[string]bool{}
	for _, c := range v.b.Calls("") {
		if c.Path == vipPointReceivePath {
			seen[c.Form.Get("taskCode")] = true
		}
		if c.Path == vipPointCompletePath || c.Path == vipPointCompleteV2Path {
			code := c.Form.Get("taskCode")
			if code == "" {
				var body map[string]string
				if err := json.Unmarshal(c.Body, &body); err != nil {
					t.Fatal(err)
				}
				code = body["taskCode"]
			}
			if code != "privilege" && !seen[code] {
				t.Errorf("完成前没有领取 %s", code)
			}
		}
	}
	signs := v.b.Calls(vipPointSignPath)
	if len(signs) != 1 || signs[0].Query.Get("csrf") != "csrf-42" || signs[0].Header.Get("Referer") != "https://big.bilibili.com/mobile/index" {
		t.Fatalf("%+v", signs)
	}
	var body map[string]any
	if err := json.Unmarshal(signs[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["device"] != "phone" || int64(body["t"].(float64))/1000 != int64(body["ts"].(float64)) {
		t.Fatalf("%v", body)
	}
}

func TestVipPointAlreadyDoneNoMutations(t *testing.T) {
	v := vipPointEnv(t, true)
	v.b.JSON(vipPointCombinePath, vipPointData(map[string]int{"bonus": 3, "dress-view": 3, "vipmallbuy": 0}))
	r := v.runTask(42, TaskVipPoint)
	if r.Status != api.StatusSkipped || v.b.Count(vipPointSignPath) != 0 || v.b.Count(vipPointReceivePath) != 0 || v.b.Count(vipPointCompletePath) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestVipPointRiskDoesNotStopOthersAndUnverifiedFails(t *testing.T) {
	v := vipPointEnv(t, true)
	v.b.JSON(vipPointCombinePath, vipPointData(map[string]int{"bonus": 0, "privilege": 0, "dress-view": 1, "ogvwatchnew": 1}))
	v.b.Handle(vipPointReceivePath, func(c bilitest.Call) bilitest.Reply {
		if c.Form.Get("taskCode") == "bonus" {
			return bilitest.Code(-352, "风控")
		}
		return bilitest.OK(nil)
	})
	v.b.JSON(vipPointCompletePath, nil)
	v.b.JSON(vipPointCompleteV2Path, nil)
	r := v.runTask(42, TaskVipPoint)
	if step(t, r, "大会员福利").Reason != "risk" || step(t, r, "浏览会员权益页").Reason != "changed" || step(t, r, "浏览装扮商城").Reason != "changed" || step(t, r, "观看正片").Status != api.StatusSkipped || v.b.Count(vipPointCompleteV2Path) != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestVipPointExperienceStates(t *testing.T) {
	for _, tc := range []struct {
		state, code int
		want        api.Status
	}{{1, 0, api.StatusSkipped}, {2, 0, api.StatusSkipped}, {0, 0, api.StatusOK}, {0, 69198, api.StatusSkipped}, {0, 6034005, api.StatusSkipped}, {0, -352, api.StatusFailed}} {
		t.Run(strings.Join([]string{string(rune('a' + tc.state)), string(rune('a' + tc.code%20))}, "-"), func(t *testing.T) {
			v := vipPointEnv(t, true)
			v.b.JSON(vipPointCombinePath, vipPointData(nil))
			v.b.JSON(vipPrivilegePath, map[string]any{"list": []map[string]int{{"type": 9, "state": tc.state}}})
			path := "api.bilibili.com/x/vip/experience/add"
			v.b.Fail(path, tc.code, "结果")
			r := v.runTask(42, TaskVipPoint)
			if step(t, r, "每日经验").Status != tc.want || (tc.state != 0 && v.b.Count(path) != 0) {
				t.Fatalf("%+v", r)
			}
		})
	}
}
