package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/VOfficial404/bili-daily/api"
)

// 新版本从 GitHub Releases 查，只读公开的发布列表，不发送任何账号信息
const (
	releasesURL   = "https://api.github.com/repos/VOfficial404/bili-daily/releases?per_page=10"
	keepReleases  = 5    // State 里最多带几个版本
	maxNotesRunes = 1500 // 每个版本的说明最多留多少字
	// 发布说明里「下载哪个文件」及之后是固定的下载说明，不算更新内容
	notesEnd = "## 下载哪个文件"
)

// updateLoop 启动后查一次有没有新版本，之后每 6 小时查一次；查不到时半小时后再试
func (e *Engine) updateLoop() {
	for {
		ctx, cancel := context.WithTimeout(e.ctx, 30*time.Second)
		err := e.checkReleases(ctx)
		cancel()
		wait := 6 * time.Hour
		if err != nil {
			if e.ctx.Err() != nil {
				return
			}
			e.Debugf("查询新版本失败：%v", err)
			wait = 30 * time.Minute
		}
		if sleepCtx(e.ctx, wait) != nil {
			return
		}
	}
}

// checkReleases 取最近发布的正式版本，记下来给客户端看；有比正在运行的更新的版本时记一条日志
func (e *Engine) checkReleases(ctx context.Context) error {
	rt := e.cfg.Transport
	if rt == nil {
		rt = http.DefaultTransport // 走系统的 HTTPS_PROXY，不走给 B 站用的代理
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "bili-daily/"+e.cfg.Version)
	resp, err := (&http.Client{Timeout: 20 * time.Second, Transport: rt}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}
	var raw []struct {
		Tag         string    `json:"tag_name"`
		URL         string    `json:"html_url"`
		Body        string    `json:"body"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
		PublishedAt time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&raw); err != nil {
		return fmt.Errorf("发布列表的格式变了：%w", err)
	}
	var list []api.Release
	for _, r := range raw {
		if r.Draft || r.Prerelease || r.Tag == "" || !strings.HasPrefix(r.URL, "https://github.com/") {
			continue
		}
		list = append(list, api.Release{Version: r.Tag, URL: r.URL, Notes: releaseNotes(r.Body), PublishedAt: r.PublishedAt})
		if len(list) == keepReleases {
			break
		}
	}
	e.mu.Lock()
	e.releases = list
	announce := len(list) > 0 && api.NewerVersion(list[0].Version, e.cfg.Version) && e.announced != list[0].Version
	if announce {
		e.announced = list[0].Version
	}
	e.mu.Unlock()
	if announce {
		e.Infof("有新版本 %s（正在运行 %s），更新内容见 %s", list[0].Version, e.cfg.Version, list[0].URL)
	}
	return nil
}

// releaseNotes 取发布说明里「更新了什么」那一段：去掉后面的下载说明，太长时截断
func releaseNotes(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if i := strings.Index(body, notesEnd); i >= 0 {
		body = body[:i]
	}
	body = strings.TrimSpace(body)
	if utf8.RuneCountInString(body) > maxNotesRunes {
		body = strings.TrimSpace(string([]rune(body)[:maxNotesRunes])) + "…"
	}
	return body
}

// stateReleases 是给客户端的版本列表，Newer 按服务端正在运行的版本算
func (e *Engine) stateReleases() []api.Release {
	if len(e.releases) == 0 {
		return nil
	}
	out := make([]api.Release, len(e.releases))
	for i, r := range e.releases {
		r.Newer = api.NewerVersion(r.Version, e.cfg.Version)
		out[i] = r
	}
	return out
}
