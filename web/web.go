// Package web 是服务器自带的网页面板：打开服务端的地址，用令牌登录后管理账号、任务和设置。
// 面板只是静态文件，数据都通过 /api 取，和桌面、手机客户端用同一套接口。
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static
var static embed.FS

// Handler 提供面板的页面和脚本
func Handler() http.Handler {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// 页面只加载自己的脚本和样式；头像来自 B 站的图片服务器，二维码是 data: 图片
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: https:; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		// 不带 Referer：B 站的头像图床拒绝外站 Referer，也不把面板地址透露出去
		h.Set("Referrer-Policy", "no-referrer")
		// 改版后浏览器立刻用上新的脚本
		h.Set("Cache-Control", "no-cache")
		if strings.HasSuffix(r.URL.Path, ".js") {
			h.Set("Content-Type", "text/javascript; charset=utf-8")
		}
		files.ServeHTTP(w, r)
	})
}
