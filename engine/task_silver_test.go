package engine

import (
	"regexp"
	"strings"
	"testing"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili/bilitest"
)

const (
	silver2coinStatusPath   = "api.live.bilibili.com/xlive/revenue/v1/wallet/getStatus"
	silver2coinExchangePath = "api.live.bilibili.com/xlive/revenue/v1/wallet/silver2coin"
)

func silver2coinEnv(t *testing.T, left int) *testEnv {
	v := newEnv(t)
	v.login(42)
	v.b.JSON(silver2coinStatusPath, map[string]any{"silver_2_coin_left": left})
	v.b.JSON(silver2coinExchangePath, map[string]any{"coin": 1, "silver": 4321})
	return v
}
func TestSilverExchange(t *testing.T) {
	v := silver2coinEnv(t, 3)
	r := v.runTask(42, TaskSilver2Coin)
	if r.Status != api.StatusOK || r.Note != "换到 1 个硬币" || step(t, r, "兑换").Detail != "换到 1 个硬币，还剩 4321 个银瓜子" {
		t.Fatalf("结果不对：%+v", r)
	}
	calls := v.b.Calls(silver2coinExchangePath)
	if len(calls) != 1 {
		t.Fatalf("兑换请求数：%d", len(calls))
	}
	c := calls[0]
	if c.Method != "POST" || c.Form.Get("csrf") != "csrf-42" || c.Form.Get("csrf_token") != "csrf-42" || !regexp.MustCompile(`^[1-9][a-z]{10}0$`).MatchString(c.Form.Get("visit_id")) {
		t.Errorf("兑换参数不对：%+v", c)
	}
	status := v.b.Calls(silver2coinStatusPath)[0]
	if status.Method != "GET" || len(status.Query) != 0 {
		t.Errorf("查询参数不对：%+v", status)
	}
	for _, c := range []bilitest.Call{status, c} {
		if c.Header.Get("Origin") != "https://link.bilibili.com" || c.Header.Get("Referer") != "https://link.bilibili.com/" {
			t.Errorf("请求头不对：%v", c.Header)
		}
	}
	v.b.JSON(silver2coinStatusPath, map[string]any{"silver_2_coin_left": 0})
	r = v.runTask(42, TaskSilver2Coin)
	if r.Status != api.StatusSkipped || v.b.Count(silver2coinExchangePath) != 1 {
		t.Fatalf("重复兑换：%+v", r)
	}
}
func TestSilverNoQuota(t *testing.T) {
	v := silver2coinEnv(t, 0)
	r := v.runTask(42, TaskSilver2Coin)
	if r.Status != api.StatusSkipped || v.b.Count(silver2coinExchangePath) != 0 {
		t.Fatalf("结果不对：%+v", r)
	}
}
func TestSilverInsufficientSkipped(t *testing.T) {
	v := silver2coinEnv(t, 1)
	v.b.Fail(silver2coinExchangePath, 1, "银瓜子不足")
	r := v.runTask(42, TaskSilver2Coin)
	if r.Status != api.StatusSkipped || !strings.HasPrefix(step(t, r, "兑换").Detail, "银瓜子不足") {
		t.Fatalf("结果不对：%+v", r)
	}
}
func TestSilverRiskFailed(t *testing.T) {
	v := silver2coinEnv(t, 1)
	v.b.Fail(silver2coinExchangePath, -352, "风控校验失败")
	r := v.runTask(42, TaskSilver2Coin)
	if r.Status != api.StatusFailed || step(t, r, "兑换").Reason != "risk" {
		t.Fatalf("结果不对：%+v", r)
	}
}
func TestSilverStatusFailureStopsExchange(t *testing.T) {
	v := silver2coinEnv(t, 1)
	v.b.Fail(silver2coinStatusPath, -352, "风控校验失败")
	r := v.runTask(42, TaskSilver2Coin)
	if r.Status != api.StatusFailed || v.b.Count(silver2coinExchangePath) != 0 {
		t.Fatalf("结果不对：%+v", r)
	}
}
