package bili

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

/* WBI 签名：见 bilibili-API-collect 的 docs/misc/sign/wbi.md */

var wbiMixin = [64]int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35,
	27, 43, 5, 49, 33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13,
	37, 48, 7, 16, 24, 55, 40, 61, 26, 17, 0, 1, 60, 51, 30, 4,
	22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11, 36, 20, 34, 44, 52,
}

type wbiKeys struct {
	mixin string
	at    time.Time
}

// wbiKeys 返回当天的混合密钥；密钥每天换，缓存一小时
func (c *Client) wbiKeys(ctx context.Context) (string, error) {
	c.mu.Lock()
	k := c.wbi
	c.mu.Unlock()
	if k.mixin != "" && time.Since(k.at) < time.Hour {
		return k.mixin, nil
	}
	// nav 在未登录时返回 -101，但 data 里仍然有 wbi_img
	body, err := c.Raw(ctx, Request{URL: "https://api.bilibili.com/x/web-interface/nav"})
	if err != nil {
		return "", err
	}
	img, sub := wbiURLs(body)
	if img == "" || sub == "" {
		return "", &Error{Code: CodeBadResponse, Message: "nav 里没有 WBI 密钥", Path: "api.bilibili.com/x/web-interface/nav"}
	}
	mixin := mixinKey(wbiKeyOf(img) + wbiKeyOf(sub))
	c.mu.Lock()
	c.wbi = wbiKeys{mixin: mixin, at: time.Now()}
	c.mu.Unlock()
	return mixin, nil
}

func wbiURLs(navBody []byte) (img, sub string) {
	var r struct {
		Data struct {
			WbiImg struct {
				ImgURL string `json:"img_url"`
				SubURL string `json:"sub_url"`
			} `json:"wbi_img"`
		} `json:"data"`
	}
	json.Unmarshal(navBody, &r)
	return r.Data.WbiImg.ImgURL, r.Data.WbiImg.SubURL
}

// wbiKeyOf 取地址里的文件名（不含扩展名）
func wbiKeyOf(u string) string {
	base := path.Base(u)
	return strings.TrimSuffix(base, path.Ext(base))
}

func mixinKey(raw string) string {
	if len(raw) < 64 {
		return ""
	}
	b := make([]byte, 0, 32)
	for _, i := range wbiMixin[:32] {
		b = append(b, raw[i])
	}
	return string(b)
}

// wbiSign 给参数加上 wts 和 w_rid，返回编码好的查询串
func wbiSign(v url.Values, mixin string, now time.Time) string {
	out := url.Values{}
	for k, vs := range v {
		for _, s := range vs {
			out.Add(k, strings.Map(func(r rune) rune {
				if strings.ContainsRune("!'()*", r) {
					return -1
				}
				return r
			}, s))
		}
	}
	out.Set("wts", strconv.FormatInt(now.Unix(), 10))
	// B 站按 %20 编码空格，Go 的 Encode 用 +
	q := strings.ReplaceAll(out.Encode(), "+", "%20")
	sum := md5.Sum([]byte(q + mixin))
	return q + "&w_rid=" + hex.EncodeToString(sum[:])
}

// SignWBI 给表单参数加上 wts 和 w_rid，返回签过名的参数。
// 地址里的参数用 Request.WBI；正文里的参数（直播心跳）用它。
func (c *Client) SignWBI(ctx context.Context, form url.Values) (url.Values, error) {
	keys, err := c.wbiKeys(ctx)
	if err != nil {
		return nil, err
	}
	return url.ParseQuery(wbiSign(form, keys, time.Now()))
}
