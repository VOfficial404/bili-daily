package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
)

// handleUPProfile 只返回公开资料，账号 Cookie 始终留在服务端。
func (e *Engine) handleUPProfile(w http.ResponseWriter, r *http.Request) {
	mid := r.PathValue("mid")
	id, err := strconv.ParseInt(mid, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != mid {
		writeErr(w, http.StatusBadRequest, "UP 主 UID 必须是不含前导零的正整数")
		return
	}
	var c *bili.Client
	for _, a := range e.accountList() {
		if !a.snapshot().LoggedIn {
			continue
		}
		c, err = e.accountClient(a.mid)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "读取用于查询 UP 主资料的登录账号失败，请重新登录")
			return
		}
		c.Wait = nil
		break
	}
	if c == nil {
		c = e.anonClient()
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	p, err := c.UPProfile(ctx, id)
	if err != nil {
		switch {
		case bili.IsCode(err, -404):
			writeErr(w, http.StatusNotFound, fmt.Sprintf("UID %s 的 UP 主不存在或已注销", mid))
		case errors.Is(err, context.DeadlineExceeded):
			writeErr(w, http.StatusGatewayTimeout, "查询 UP 主资料超时，请稍后重试")
		case bili.IsCode(err, -101, -403):
			writeErr(w, http.StatusBadGateway, "无法查询 UP 主资料：B 站要求有效的登录账号，请先扫码登录或刷新登录")
		default:
			writeErr(w, http.StatusBadGateway, "查询 UP 主资料失败："+err.Error())
		}
		return
	}
	writeJSONResp(w, http.StatusOK, api.UPProfile{Mid: strconv.FormatInt(p.Mid, 10), Uname: p.Uname, Face: p.Face})
}
