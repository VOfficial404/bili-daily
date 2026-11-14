package engine

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/VOfficial404/bili-daily/api"
)

// notifyRun 按设置推送一轮的结果。手动运行不推送。
func (e *Engine) notifyRun(a *account, run *api.Run, loggedOut bool) {
	n := e.Settings().Notify
	if n.Channel == "" || n.When == "never" || (n.When == "failed" && run.Status == api.StatusOK) {
		return
	}
	today := day(run.Started)
	if loggedOut {
		// 登录失效每天只提醒一次
		if a.snapshot().NotifiedOut == today {
			return
		}
		a.update(func(st *accountState) { st.NotifiedOut = today })
	}
	title, text := runMessage(run)
	go func() {
		ctx, cancel := context.WithTimeout(e.ctx, 30*time.Second)
		defer cancel()
		if err := e.push(ctx, n, title, text); err != nil {
			e.Warnf("推送运行结果失败：%v", err)
		}
	}()
}

// runMessage 把报告写成推送的标题和正文：纯文本，每个任务一行，失败的带原因和建议
func runMessage(run *api.Run) (title, text string) {
	switch run.Status {
	case api.StatusOK:
		title = run.Uname + "：今天的任务完成了"
	case api.StatusPartial:
		title = run.Uname + "：有任务没完成"
	default:
		title = run.Uname + "：任务失败"
	}
	var b strings.Builder
	b.WriteString(run.Summary)
	b.WriteString("\n")
	for _, r := range run.Tasks {
		if r.ID == "login" && r.Status == api.StatusOK {
			continue
		}
		mark := "✓"
		switch r.Status {
		case api.StatusFailed, api.StatusPartial:
			mark = "✗"
		case api.StatusSkipped:
			mark = "–"
		}
		fmt.Fprintf(&b, "\n%s %s：%s", mark, r.Title, r.Note)
	}
	return title, b.String()
}

// push 通过设置的渠道发一条消息。各渠道返回的业务错误也算失败。
func (e *Engine) push(ctx context.Context, n api.Notify, title, text string) error {
	if n.Channel == "" {
		return errNoPush
	}
	if n.Token == "" {
		return fmt.Errorf("还没有填写%s", tokenLabel(n.Channel))
	}
	// 推送不经过给 B 站用的代理：那个代理的出口不一定连得上推送服务
	hc := &http.Client{Timeout: 20 * time.Second}
	if e.cfg.Transport != nil {
		hc.Transport = e.cfg.Transport
	}
	full := title + "\n\n" + text

	switch n.Channel {
	case "telegram":
		if n.Target == "" {
			return fmt.Errorf("还没有填写 Telegram 的 chat_id")
		}
		host := n.Endpoint
		if host == "" {
			host = "https://api.telegram.org"
		}
		var r struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
		}
		err := postJSON(ctx, hc, host+"/bot"+n.Token+"/sendMessage", map[string]any{
			"chat_id": n.Target, "text": full, "disable_web_page_preview": true,
		}, &r)
		// Telegram 出错时返回 4xx 和 JSON，说明在 description 里
		if r.Description != "" && !r.OK {
			return fmt.Errorf("Telegram：%s", r.Description)
		}
		if err != nil {
			return err
		}
		if !r.OK {
			return fmt.Errorf("Telegram：响应缺少成功确认")
		}
		return nil

	case "pushplus":
		var r struct {
			Code *int   `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := postJSON(ctx, hc, "https://www.pushplus.plus/send", map[string]any{
			"token": n.Token, "title": title, "content": text, "template": "txt",
		}, &r); err != nil {
			return err
		}
		if r.Code == nil {
			return fmt.Errorf("PushPlus：响应缺少 code")
		}
		if *r.Code != 200 {
			return fmt.Errorf("PushPlus：%d %s", *r.Code, r.Msg)
		}
		return nil

	case "wecom":
		u := n.Token
		if !strings.HasPrefix(u, "http") {
			u = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=" + url.QueryEscape(n.Token)
		}
		return postBot(ctx, hc, "企业微信", u, full)

	case "dingtalk":
		u := n.Token
		if !strings.HasPrefix(u, "http") {
			u = "https://oapi.dingtalk.com/robot/send?access_token=" + url.QueryEscape(n.Token)
		}
		if n.Target != "" {
			// 加签：时间戳和密钥用换行连起来，以密钥做 HMAC-SHA256
			ts := strconv.FormatInt(e.now().UnixMilli(), 10)
			mac := hmac.New(sha256.New, []byte(n.Target))
			mac.Write([]byte(ts + "\n" + n.Target))
			u += "&timestamp=" + ts + "&sign=" + url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		}
		return postBot(ctx, hc, "钉钉", u, full)

	case "serverchan":
		var r struct {
			Code    *int   `json:"code"`
			Message string `json:"message"`
		}
		// Server 酱按 Markdown 显示，两个换行才是换行
		form := url.Values{"title": {title}, "desp": {strings.ReplaceAll(text, "\n", "\n\n")}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://sctapi.ftqq.com/"+url.PathEscape(n.Token)+".send", strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if err := doPush(hc, req, &r); err != nil {
			return err
		}
		if r.Code == nil {
			return fmt.Errorf("Server 酱：响应缺少 code")
		}
		if *r.Code != 0 {
			return fmt.Errorf("Server 酱：%d %s", *r.Code, r.Message)
		}
		return nil

	case "bark":
		host := n.Endpoint
		if host == "" {
			host = "https://api.day.app"
		}
		var r struct {
			Code    *int   `json:"code"`
			Message string `json:"message"`
		}
		if err := postJSON(ctx, hc, host+"/"+url.PathEscape(n.Token), map[string]any{"title": title, "body": text, "group": "bili-daily"}, &r); err != nil {
			return err
		}
		if r.Code == nil {
			return fmt.Errorf("Bark：响应缺少 code")
		}
		if *r.Code != 200 {
			return fmt.Errorf("Bark：%d %s", *r.Code, r.Message)
		}
		return nil

	case "webhook":
		if !strings.HasPrefix(n.Token, "http://") && !strings.HasPrefix(n.Token, "https://") {
			return fmt.Errorf("Webhook 地址要以 http:// 或 https:// 开头")
		}
		return postJSON(ctx, hc, n.Token, map[string]any{"title": title, "text": text}, nil)
	}
	return fmt.Errorf("不支持的推送渠道 %q", n.Channel)
}

func tokenLabel(channel string) string {
	switch channel {
	case "telegram":
		return " Telegram 机器人的 token"
	case "wecom", "dingtalk":
		return "机器人的 Webhook 地址或 key"
	case "webhook":
		return " Webhook 地址"
	}
	return "推送密钥"
}

// postBot 发给企业微信或钉钉的群机器人，两家的格式一样
func postBot(ctx context.Context, hc *http.Client, name, u, text string) error {
	var r struct {
		ErrCode *int   `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := postJSON(ctx, hc, u, map[string]any{"msgtype": "text", "text": map[string]string{"content": text}}, &r); err != nil {
		return err
	}
	if r.ErrCode == nil {
		return fmt.Errorf("%s：响应缺少 errcode", name)
	}
	if *r.ErrCode != 0 {
		return fmt.Errorf("%s：%d %s", name, *r.ErrCode, r.ErrMsg)
	}
	return nil
}

func postJSON(ctx context.Context, hc *http.Client, u string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doPush(hc, req, out)
}

// doPush 发出请求，把 JSON 响应解析进 out（可以为 nil）。错误信息里不带地址：地址里有密钥。
func doPush(hc *http.Client, req *http.Request, out any) error {
	resp, err := hc.Do(req)
	if err != nil {
		if ue, ok := err.(*url.Error); ok {
			err = ue.Err
		}
		return fmt.Errorf("连不上推送服务：%v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取推送响应失败：%w", err)
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("解析推送响应失败：%w", err)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("推送服务返回 HTTP %d", resp.StatusCode)
	}
	return nil
}
