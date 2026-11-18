package engine

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/api"
)

// started 启动排程循环；出口 IP 的查询也给一个回答
func (v *testEnv) started() {
	v.b.JSON("api.bilibili.com/x/web-interface/zone", map[string]any{"addr": "203.0.113.7", "country": "中国", "province": "广东", "city": "深圳", "isp": "电信"})
	v.e.Start()
}

// waitFor 等 cond 成立，最多 5 秒
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("没有等到：%s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStartCatchesUpMissedRunOnce(t *testing.T) {
	runs := make(chan struct{}, 8)
	useTasks(t, toggle(TaskDaily, "每日任务", func(x *Ctx) {
		runs <- struct{}{}
		x.OK("观看视频", "ok")
	}))
	v := newEnv(t) // 现在是 12:00，默认 09:30 开始、最多推迟一小时：今天的已经错过
	v.login(42)
	v.started()

	waitFor(t, "补跑结束", func() bool {
		a := v.e.State().Accounts[0]
		return a.LastRun != nil && a.Running == nil
	})
	a := v.e.State().Accounts[0]
	if a.LastRun.Trigger != "catchup" || a.LastRun.Status != api.StatusOK {
		t.Errorf("last run = %+v", a.LastRun)
	}
	if a.NextRun == nil || day(*a.NextRun) != "2026-11-14" {
		t.Errorf("下次运行应该在明天：%v", a.NextRun)
	}
	waitFor(t, "出口 IP", func() bool { return v.e.State().Egress != nil })
	if eg := v.e.State().Egress; eg.Addr != "203.0.113.7" || eg.Place != "广东 深圳" {
		t.Errorf("egress = %+v", eg)
	}
	time.Sleep(100 * time.Millisecond)
	if len(runs) != 1 {
		t.Errorf("今天跑了 %d 轮", len(runs))
	}
}

func TestManualRunOverHTTPAndCancel(t *testing.T) {
	entered := make(chan struct{}, 1)
	useTasks(t,
		toggle(TaskDaily, "每日任务", func(x *Ctx) {
			entered <- struct{}{}
			<-x.Done() // 一直做到被中止
			x.Fail("观看视频", x.Err())
		}),
		toggle(TaskManga, "漫画", func(x *Ctx) { t.Error("中止之后还在跑后面的任务") }),
	)
	v := newEnv(t)
	a := v.login(42)
	a.update(func(st *accountState) { st.DailyDate = day(v.now) }) // 今天的已经跑过，不会自动开始
	v.started()
	srv := httptest.NewServer(v.e.Handler())
	defer srv.Close()
	c := api.NewClient(srv.URL, "test-token")
	ctx := context.Background()

	if err := c.Cancel(ctx, "42"); err == nil {
		t.Error("没有在运行时中止应该报错")
	}
	if err := c.Run(ctx, "42", []string{"nope"}); err == nil {
		t.Error("不存在的任务没有被拒绝")
	}
	if err := c.Run(ctx, "42", nil); err != nil {
		t.Fatal(err)
	}
	<-entered
	st, _ := c.State(ctx)
	if p := st.Accounts[0].Running; p == nil || p.Task != "每日任务" || p.Total != 3 {
		t.Fatalf("progress = %+v", p)
	}
	if err := c.Run(ctx, "42", nil); err == nil {
		t.Error("正在运行时又开始了一轮")
	}
	if err := c.Cancel(ctx, "42"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "运行结束", func() bool { return v.e.State().Accounts[0].Running == nil })
	last := v.e.State().Accounts[0].LastRun
	if last == nil || last.Trigger != "manual" || !strings.HasPrefix(last.Summary, "已中止") || last.Status == api.StatusOK {
		t.Errorf("last run = %+v", last)
	}
}

// 手机后台任务靠 Settle 补跑：到点的那一轮做完才返回，系统给的时间用完时先返回
func TestSettleWaitsForDueRun(t *testing.T) {
	release := make(chan struct{})
	useTasks(t, toggle(TaskDaily, "每日任务", func(x *Ctx) {
		select {
		case <-release:
			x.OK("观看视频", "ok")
		case <-x.Done():
			x.Fail("观看视频", x.Err())
		}
	}))
	v := newEnv(t) // 今天 09:30 的那一轮已经错过
	v.login(42)
	v.started()

	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := v.e.Settle(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("补跑还没做完，Settle 应该等到超时：%v", err)
	}
	close(release)
	ctx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if err := v.e.Settle(ctx); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if a := v.e.State().Accounts[0]; a.LastRun == nil || a.LastRun.Trigger != "catchup" || a.LastRun.Status != api.StatusOK || a.Running != nil {
		t.Errorf("account = %+v", a)
	}
}
