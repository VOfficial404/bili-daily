package engine

import (
	"net/http"
	"strings"
	"testing"

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
