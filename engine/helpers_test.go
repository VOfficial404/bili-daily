package engine

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/VOfficial404/bili-tools/api"
	"github.com/VOfficial404/bili-tools/bili/bilitest"
)

// testEnv 是测试用的一套：数据目录是临时的，B 站是假的，不真的等待，时间可以拨。
type testEnv struct {
	t   *testing.T
	e   *Engine
	b   *bilitest.Server
	now time.Time
}

// 默认的测试时间：2026-11-13 12:00 北京时间
var testNow = time.Date(2026, 11, 13, 12, 0, 0, 0, Beijing)

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	v := &testEnv{t: t, b: bilitest.New(), now: testNow}
	e, err := New(Config{
		DataDir:   t.TempDir(),
		Version:   "test",
		Token:     "test-token",
		Transport: v.b,
		Now:       func() time.Time { return v.now },
		Sleep:     func(ctx context.Context, d time.Duration) error { return ctx.Err() },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Stop)
	v.e = e
	return v
}

// navData 是 nav 接口返回的 data，测试可以改里面的字段再交给 v.b.JSON
func navData(mid int64) map[string]any {
	return map[string]any{
		"isLogin":    true,
		"mid":        mid,
		"uname":      "测试用户",
		"face":       "https://i0.hdslb.com/bfs/face/x.jpg",
		"money":      100.0,
		"vipStatus":  0,
		"vipType":    0,
		"vipDueDate": 0,
		"level_info": map[string]any{"current_level": 4, "current_exp": 5000, "next_exp": 10800},
		"wallet":     map[string]any{"bcoin_balance": 0, "coupon_balance": 0},
		"wbi_img": map[string]any{
			"img_url": "https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png",
			"sub_url": "https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png",
		},
	}
}

const navPath = "api.bilibili.com/x/web-interface/nav"

// login 加一个已登录的账号（不经过扫码），并让假 B 站认它：nav 返回 navData，Cookie 不需要刷新。
func (v *testEnv) login(mid int64) *account {
	v.t.Helper()
	id := strconv.FormatInt(mid, 10)
	if _, err := v.e.saveSession(session{
		Cookies:      map[string]string{"SESSDATA": "sess-" + id, "bili_jct": "csrf-" + id, "DedeUserID": id, "buvid3": "buvid3-" + id},
		RefreshToken: "refresh-" + id,
	}); err != nil {
		v.t.Fatal(err)
	}
	v.b.JSON(navPath, navData(mid))
	v.b.JSON("passport.bilibili.com/x/passport-login/web/cookie/info", map[string]any{"refresh": false, "timestamp": v.now.UnixMilli()})
	v.e.accountLoggedIn(id)
	return v.e.account(id)
}

// vip 把 nav 改成大会员：typ 是 1（月度）或 2（年度），coupon 是 B 币券余额
func (v *testEnv) vip(mid int64, typ int, coupon float64) {
	n := navData(mid)
	n["vipStatus"], n["vipType"] = 1, typ
	n["vipDueDate"] = v.now.AddDate(0, 6, 0).UnixMilli()
	n["wallet"] = map[string]any{"bcoin_balance": 0, "coupon_balance": coupon}
	v.b.JSON(navPath, n)
}

// set 修改设置
func (v *testEnv) set(fn func(s *api.Settings)) {
	v.t.Helper()
	s := v.e.Settings()
	fn(&s)
	if _, err := v.e.UpdateSettings(s); err != nil {
		v.t.Fatal(err)
	}
}

// run 立即给账号跑一轮；ids 为空表示每日计划
func (v *testEnv) run(mid int64, ids ...string) *api.Run {
	v.t.Helper()
	run, err := v.e.RunNow(context.Background(), strconv.FormatInt(mid, 10), ids)
	if err != nil {
		v.t.Fatal(err)
	}
	return run
}

// runTask 只跑一个任务，返回它的结果
func (v *testEnv) runTask(mid int64, id string) api.TaskResult {
	v.t.Helper()
	run := v.run(mid, id)
	for _, r := range run.Tasks {
		if r.ID == id {
			return r
		}
	}
	v.t.Fatalf("报告里没有任务 %s：%+v", id, run.Tasks)
	return api.TaskResult{}
}

// step 找出结果里名字是 name 的一步
func step(t *testing.T, r api.TaskResult, name string) api.Step {
	t.Helper()
	for _, s := range r.Steps {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("任务 %s 里没有「%s」这一步：%+v", r.ID, name, r.Steps)
	return api.Step{}
}

// useTasks 在这个测试里把登记的任务换成 tasks
func useTasks(t *testing.T, tasks ...Task) {
	saved := registry
	registry = map[string]Task{}
	for _, task := range tasks {
		register(task)
	}
	t.Cleanup(func() { registry = saved })
}
