package bili

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
)

// 不是 B 站返回的、本程序自己用的 code
const (
	CodeNetwork     = -900001 // 请求没有发出去或没有收到响应
	CodeBadResponse = -900002 // 收到了响应，但不是预期的格式
)

// Error 是一次失败的接口调用：B 站返回了非 0 的 code，或者请求本身失败
type Error struct {
	Code    int
	Key     string // 漫画等接口的 code 是字符串（例如 invalid_argument），放在这里，Code 为 CodeBadResponse
	Message string
	Path    string          // 主机和路径，不含查询参数
	Data    json.RawMessage // 有些接口失败时也在 data 里带信息
}

func (e *Error) Error() string {
	switch e.Code {
	case CodeNetwork:
		return fmt.Sprintf("网络错误：%s（%s）", e.Message, e.Path)
	case CodeBadResponse:
		if e.Key != "" {
			return fmt.Sprintf("B 站返回 %s %s（%s）", e.Key, e.Message, e.Path)
		}
		return fmt.Sprintf("%s（%s）", e.Message, e.Path)
	}
	if e.Message == "" {
		return fmt.Sprintf("B 站返回 %d（%s）", e.Code, e.Path)
	}
	return fmt.Sprintf("B 站返回 %d %s（%s）", e.Code, e.Message, e.Path)
}

// Reason 把失败归成几类，报告里按它解释原因和给出建议
type Reason string

const (
	ReasonNone         Reason = ""
	ReasonNotLogin     Reason = "not_login"    // 登录失效
	ReasonRisk         Reason = "risk"         // 被风控拦截
	ReasonRate         Reason = "rate"         // 请求太频繁
	ReasonAlready      Reason = "already"      // 已经做过，不算失败
	ReasonIneligible   Reason = "ineligible"   // 账号不满足条件
	ReasonInsufficient Reason = "insufficient" // 硬币、B 币券等不够
	ReasonNetwork      Reason = "network"      // 网络或代理问题
	ReasonChanged      Reason = "changed"      // 接口可能变了
	ReasonCanceled     Reason = "canceled"     // 运行被中止
	ReasonOther        Reason = "other"
)

// Hint 是每类原因对应的处理建议
func (r Reason) Hint() string {
	switch r {
	case ReasonNotLogin:
		return "登录已失效，请给这个账号重新扫码"
	case ReasonRisk:
		return "被 B 站风控拦截。过几小时再试；如果一直这样，把请求节奏调慢，或换一个出口 IP"
	case ReasonRate:
		return "请求太频繁，稍后会自动恢复。可以把请求节奏调慢"
	case ReasonIneligible:
		return "这个账号不满足任务的条件"
	case ReasonInsufficient:
		return "余额不够"
	case ReasonNetwork:
		return "网络不通或代理不可用，检查服务器网络和代理设置"
	case ReasonChanged:
		return "B 站接口可能已经变化，请更新到最新版本；仍然出现请反馈"
	case ReasonCanceled:
		return "运行被中止"
	}
	return ""
}

// Classify 判断一个错误属于哪类原因
func Classify(err error) Reason {
	if err == nil {
		return ReasonNone
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ReasonCanceled
	}
	var e *Error
	if !errors.As(err, &e) {
		return ReasonOther
	}
	switch e.Code {
	case CodeNetwork:
		return ReasonNetwork
	case CodeBadResponse:
		return ReasonChanged
	case -101, -111, -400101, 61000:
		// -101 未登录；-111 csrf 校验失败；61000 App 授权失效
		return ReasonNotLogin
	case -352, -412, -401, -403, 22015:
		return ReasonRisk
	case -509, -799, 22118, 1002003:
		return ReasonRate
	case -404:
		return ReasonChanged
	}
	return ReasonOther
}

// IsCode 判断错误是不是 B 站返回的某个 code
func IsCode(err error, codes ...int) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	for _, c := range codes {
		if e.Code == c {
			return true
		}
	}
	return false
}

// IsNotLogin 表示登录失效，需要重新扫码
func IsNotLogin(err error) bool { return Classify(err) == ReasonNotLogin }

func netMessage(err error) string {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "连接超时"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "域名解析失败"
	}
	s := err.Error()
	// 去掉 Go 加在前面的 `Get "https://…": `，里面可能带着查询参数
	if i := strings.LastIndex(s, `": `); i >= 0 {
		s = s[i+3:]
	}
	return s
}
