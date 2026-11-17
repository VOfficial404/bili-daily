package engine

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
	"github.com/VOfficial404/bili-daily/bili/bilitest"
)

const coinDevicePath = "api.bilibili.com/x/frontend/finger/spi"

func TestDailyCoinsKeepSuccessBeforeAccountRestriction(t *testing.T) {
	v := dailyEnv(t)
	dailyOnlyCoins(v, 5)
	v.b.Handle(dailyAddPath, func(c bilitest.Call) bilitest.Reply {
		if v.b.Count(dailyAddPath) == 1 {
			return bilitest.OK(map[string]any{"like": true})
		}
		return bilitest.Code(-403, "账号异常,操作失败")
	})
	r := v.runTask(42, TaskDaily)
	s := step(t, r, "投币")
	if n := v.b.Count(dailyAddPath); n != 2 {
		t.Fatalf("成功后被拒绝应停止：投币请求 %d 次", n)
	}
	if s.Status != api.StatusFailed || s.Reason != string(bili.ReasonRisk) || s.Code != -403 {
		t.Fatalf("账号异常没有保留错误：%+v", s)
	}
	if !strings.Contains(s.Detail, "本轮已投 1 个") || !strings.Contains(r.Note, "今天投币 1 个") {
		t.Fatalf("丢失已成功的投币数：%+v", r)
	}
	if v.b.Count(coinDevicePath) != 0 {
		t.Fatal("已有设备标识时不应更换")
	}
}

func TestDailyCoinsPersistMissingDeviceWithoutChangingSession(t *testing.T) {
	v := dailyEnv(t)
	dailyOnlyCoins(v, 1)
	sess, err := v.e.loadSession("42")
	if err != nil {
		t.Fatal(err)
	}
	delete(sess.Cookies, "buvid3")
	sess.Cookies["buvid4"] = "existing-b4"
	if _, err := v.e.saveSession(sess); err != nil {
		t.Fatal(err)
	}
	v.b.JSON(coinDevicePath, map[string]any{"b_3": "new-b3", "b_4": "replacement-b4"})
	v.b.Handle(dailyAddPath, func(c bilitest.Call) bilitest.Reply {
		if c.Cookie["buvid3"] != "new-b3" || c.Cookie["buvid4"] != "existing-b4" || c.Cookie["SESSDATA"] != sess.Cookies["SESSDATA"] {
			t.Error("投币未使用补齐后的设备标识，或覆盖了已有 Cookie")
		}
		return bilitest.OK(nil)
	})
	r := v.runTask(42, TaskDaily)
	if step(t, r, "投币").Status != api.StatusOK {
		t.Fatalf("补齐后投币失败：%+v", r)
	}
	saved, err := v.e.loadSession("42")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Cookies["buvid3"] != "new-b3" || saved.Cookies["buvid4"] != "existing-b4" || saved.RefreshToken != sess.RefreshToken || !saved.CookieExpires.Equal(sess.CookieExpires) || saved.Cookies["bili_jct"] != sess.Cookies["bili_jct"] {
		t.Fatal("设备标识未持久化，或覆盖了会话凭据")
	}
	v.runTask(42, TaskDaily)
	if n := v.b.Count(coinDevicePath); n != 1 {
		t.Fatalf("保存后不应重新获取设备标识：%d 次", n)
	}
}

func TestDailyCoinsDoNotSpendWithoutDeviceCookie(t *testing.T) {
	for _, mode := range []string{"empty", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			v := dailyEnv(t)
			dailyOnlyCoins(v, 1)
			sess, err := v.e.loadSession("42")
			if err != nil {
				t.Fatal(err)
			}
			delete(sess.Cookies, "buvid3")
			if _, err := v.e.saveSession(sess); err != nil {
				t.Fatal(err)
			}
			v.b.Handle(coinDevicePath, func(c bilitest.Call) bilitest.Reply {
				switch mode {
				case "unavailable":
					return bilitest.Reply{Status: http.StatusServiceUnavailable}
				default:
					return bilitest.OK(map[string]any{"b_4": "only-b4"})
				}
			})
			r := v.runTask(42, TaskDaily)
			if v.b.Count(dailyAddPath) != 0 || step(t, r, "准备投币设备 Cookie").Status != api.StatusFailed {
				t.Fatalf("没有设备标识仍然投币：%+v", r)
			}
			saved, err := v.e.loadSession("42")
			if err != nil || saved.Cookies["buvid3"] != "" || saved.Cookies["buvid4"] != "" {
				t.Fatal("设备获取失败不应保存部分结果")
			}
		})
	}
}

func TestDailyCoinsWaitBeforeSpendingAndCancel(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cancelAfter int
	}{
		{"all_coins", -1},
		{"cancel_before_first", 0},
		{"cancel_after_first", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := dailyEnv(t)
			dailyOnlyCoins(v, 3)
			waits := 0
			v.e.cfg.Sleep = func(ctx context.Context, d time.Duration) error {
				if d >= 20*time.Second && d <= 40*time.Second {
					if got := v.b.Count(dailyAddPath); got != waits {
						t.Fatalf("等待前已发 %d 次投币请求，期望 %d 次", got, waits)
					}
					waits++
					if tc.cancelAfter == v.b.Count(dailyAddPath) {
						return context.Canceled
					}
				}
				v.now = v.now.Add(d)
				return ctx.Err()
			}
			r := v.runTask(42, TaskDaily)
			s := step(t, r, "投币")
			wantCoins, wantWaits := 3, 3
			if tc.cancelAfter >= 0 {
				wantCoins, wantWaits = tc.cancelAfter, tc.cancelAfter+1
				if s.Status != api.StatusFailed || s.Reason != string(bili.ReasonCanceled) {
					t.Fatalf("等待取消后结果 = %+v", s)
				}
				if !strings.Contains(r.Note, fmt.Sprintf("今天投币 %d 个", wantCoins)) {
					t.Fatalf("等待取消后丢失成功数量：%+v", r)
				}
			} else if s.Status != api.StatusOK {
				t.Fatalf("等待后投币失败：%+v", s)
			}
			if got := v.b.Count(dailyAddPath); got != wantCoins || waits != wantWaits {
				t.Fatalf("投币 %d 次、等待 %d 次，期望 %d 次、%d 次", got, waits, wantCoins, wantWaits)
			}
		})
	}
}

func TestDailyCoinDelaySetting(t *testing.T) {
	v := dailyEnv(t)
	for _, bad := range [][2]int{{-1, 10}, {30, 20}, {0, 601}} {
		s := v.e.Settings()
		s.Tasks.Daily.CoinDelayMin, s.Tasks.Daily.CoinDelayMax = bad[0], bad[1]
		if _, err := v.e.UpdateSettings(s); err == nil {
			t.Errorf("等待 %v 秒应被拒绝", bad)
		}
	}
	dailyOnlyCoins(v, 2)
	v.set(func(s *api.Settings) { s.Tasks.Daily.CoinDelayMin, s.Tasks.Daily.CoinDelayMax = 0, 0 })
	var waits []time.Duration
	v.e.cfg.Sleep = func(ctx context.Context, d time.Duration) error {
		if d >= 20*time.Second {
			waits = append(waits, d)
		}
		v.now = v.now.Add(d)
		return ctx.Err()
	}
	if s := step(t, v.runTask(42, TaskDaily), "投币"); s.Status != api.StatusOK || v.b.Count(dailyAddPath) != 2 || len(waits) != 0 {
		t.Fatalf("关闭等待后：%+v，等待 %v", s, waits)
	}
	v.set(func(s *api.Settings) {
		s.Tasks.Daily.Coins = 5
		s.Tasks.Daily.CoinDelayMin, s.Tasks.Daily.CoinDelayMax = 90, 90
	})
	v.b.JSON(dailyExpPath, 20)
	v.runTask(42, TaskDaily)
	if len(waits) != 3 || waits[0] != 90*time.Second {
		t.Fatalf("自定义等待 = %v", waits)
	}
}
