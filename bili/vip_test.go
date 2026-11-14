package bili

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/VOfficial404/bili-daily/bili/bilitest"
)

func TestVIPPointReceiveIncludesExistingClientMetadata(t *testing.T) {
	const path = "api.bilibili.com/pgc/activity/score/task/receive/v2"
	for _, code := range []string{"bonus", "dress-view", "ogvwatchnew"} {
		t.Run(code, func(t *testing.T) {
			srv := bilitest.New()
			srv.JSON(path, nil)
			c := New(srv, map[string]string{"bili_jct": "csrf", "SESSDATA": "session"})
			if err := c.VIPPointReceive(context.Background(), code); err != nil {
				t.Fatal(err)
			}
			calls := srv.Calls(path)
			if len(calls) != 1 {
				t.Fatalf("领取请求数：%d", len(calls))
			}
			call := calls[0]
			for key, want := range map[string]string{
				"build":        "8451100",
				"statistics":   `{"appId":1,"platform":3,"version":"8.45.1","abtest":""}`,
				"disable_rcmd": "0",
				"mobi_app":     "android",
				"platform":     "android",
				"taskCode":     code,
				"csrf":         "csrf",
			} {
				if got := call.Form.Get(key); got != want {
					t.Errorf("%s = %q，应为 %q", key, got, want)
				}
			}
			ms, msErr := strconv.ParseInt(call.Form.Get("t"), 10, 64)
			sec, secErr := strconv.ParseInt(call.Form.Get("ts"), 10, 64)
			if msErr != nil || secErr != nil || ms <= 0 || ms/1000 != sec || call.Method != "POST" || call.Form.Get("TaskCode") != "" || call.Header.Get("Referer") != vipTaskReferer || call.Cookie["SESSDATA"] != "session" {
				t.Fatalf("领取请求不完整：%+v", call)
			}
		})
	}
}

func TestVIPPointReceivePreservesServerFailure(t *testing.T) {
	const path = "api.bilibili.com/pgc/activity/score/task/receive/v2"
	srv := bilitest.New()
	srv.Fail(path, 6007000, "版本不支持")
	c := New(srv, map[string]string{"bili_jct": "csrf"})
	if err := c.VIPPointReceive(context.Background(), "ogvwatchnew"); !IsCode(err, 6007000) {
		t.Fatalf("领取错误未保留：%v", err)
	}
}

func TestVIPPointCompleteRejectsWatchWithoutRequest(t *testing.T) {
	srv := bilitest.New()
	c := New(srv, nil)
	if err := c.VIPPointComplete(context.Background(), "ogvwatchnew", time.Unix(1, 0)); err == nil {
		t.Fatal("观看正片不能通过通用接口报告完成")
	}
	if calls := srv.Calls(""); len(calls) != 0 {
		t.Fatalf("不应提交播放完成请求：%+v", calls)
	}
}
