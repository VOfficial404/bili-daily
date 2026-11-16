package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/VOfficial404/bili-daily/bili/bilitest"
)

const releasesPath = "api.github.com/repos/VOfficial404/bili-daily/releases"

func TestReleasesShowNotesAndWhetherNewer(t *testing.T) {
	v := newEnv(t)
	v.e.cfg.Version = "v0.1.1-3-g83eee16"
	v.e.cfg.UpdateHint = "docker compose pull"
	long := strings.Repeat("改", maxNotesRunes+10)
	v.b.Handle(releasesPath, func(bilitest.Call) bilitest.Reply {
		return bilitest.Reply{Body: []any{
			map[string]any{"tag_name": "v0.2.0-rc1", "html_url": "https://github.com/x/releases/tag/v0.2.0-rc1", "prerelease": true},
			map[string]any{"tag_name": "v0.1.2", "html_url": "https://github.com/x/releases/tag/v0.1.2", "published_at": "2026-11-17T01:00:00Z",
				"body": "- 新增从关注里选 UP 主\r\n\r\n## 下载哪个文件\r\n\r\n| 设备 | 下载 |"},
			map[string]any{"tag_name": "v0.1.1", "html_url": "https://github.com/x/releases/tag/v0.1.1", "body": "## 下载哪个文件\n..."},
			map[string]any{"tag_name": "v0.1.0", "html_url": "https://github.com/x/releases/tag/v0.1.0", "body": long},
			map[string]any{"tag_name": "v0.0.9", "html_url": "https://evil.example/", "body": "x"},
		}}
	})
	if err := v.e.checkReleases(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := v.e.State()
	if len(st.Releases) != 3 || st.UpdateHint != "docker compose pull" {
		t.Fatalf("releases = %+v", st)
	}
	r := st.Releases[0]
	if r.Version != "v0.1.2" || !r.Newer || r.Notes != "- 新增从关注里选 UP 主" || r.PublishedAt.IsZero() {
		t.Fatalf("最新版本 = %+v", r)
	}
	if st.Releases[1].Newer || st.Releases[1].Notes != "" {
		t.Fatalf("正在运行的版本之前的发布 = %+v", st.Releases[1])
	}
	if n := []rune(st.Releases[2].Notes); len(n) != maxNotesRunes+1 || n[len(n)-1] != '…' {
		t.Fatalf("过长的说明没有截断：%d 字", len(n))
	}
	if c := v.b.Calls(releasesPath)[0]; len(c.Cookie) != 0 || c.Header.Get("User-Agent") != "bili-daily/v0.1.1-3-g83eee16" {
		t.Fatalf("查询新版本的请求 = %+v", c)
	}

	// 查询失败时保留上次的结果
	v.b.Handle(releasesPath, func(bilitest.Call) bilitest.Reply { return bilitest.Reply{Status: 403} })
	if err := v.e.checkReleases(context.Background()); err == nil || len(v.e.State().Releases) != 3 {
		t.Fatalf("err = %v, releases = %d", err, len(v.e.State().Releases))
	}
}
