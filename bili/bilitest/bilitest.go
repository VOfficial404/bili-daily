// Package bilitest 是测试用的假 B 站：一个 http.RoundTripper，按「主机/路径」把请求交给登记的处理函数。
package bilitest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Call 是收到的一个请求
type Call struct {
	Method string
	Path   string     // 主机/路径，例如 api.bilibili.com/x/web-interface/nav
	Query  url.Values // 地址里的参数
	Form   url.Values // 表单正文
	Body   []byte     // 原始正文
	Header http.Header
	Cookie map[string]string
}

// Reply 是处理函数的回答。Body 为 nil 时回 {"code":0}。
type Reply struct {
	Status int // 0 表示 200
	Body   any // string、[]byte，或者会被编码成 JSON 的值
	Header http.Header
}

// Handler 回答一个请求
type Handler func(c Call) Reply

// Server 是假的 B 站
type Server struct {
	mu       sync.Mutex
	handlers map[string]Handler
	calls    []Call
}

func New() *Server { return &Server{handlers: map[string]Handler{}} }

// Handle 登记 path（主机/路径，不含 https://）的处理函数，替换之前登记的
func (s *Server) Handle(path string, h Handler) {
	s.mu.Lock()
	s.handlers[path] = h
	s.mu.Unlock()
}

// JSON 让 path 固定回答 {"code":0,"message":"0","data":data}
func (s *Server) JSON(path string, data any) {
	s.Handle(path, func(Call) Reply { return OK(data) })
}

// Fail 让 path 固定回答 {"code":code,"message":message}
func (s *Server) Fail(path string, code int, message string) {
	s.Handle(path, func(Call) Reply { return Code(code, message) })
}

// OK 是成功的回答
func OK(data any) Reply {
	return Reply{Body: map[string]any{"code": 0, "message": "0", "data": data}}
}

// Code 是带 code 的失败回答
func Code(code int, message string) Reply {
	return Reply{Body: map[string]any{"code": code, "message": message}}
}

// Calls 返回发往 path 的请求，按先后顺序；path 为空时返回全部
func (s *Server) Calls(path string) []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Call
	for _, c := range s.calls {
		if path == "" || c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

// Count 是发往 path 的请求数
func (s *Server) Count(path string) int { return len(s.Calls(path)) }

// RoundTrip 实现 http.RoundTripper。没有登记的路径回 HTTP 404。
func (s *Server) RoundTrip(req *http.Request) (*http.Response, error) {
	c := Call{
		Method: req.Method,
		Path:   req.URL.Host + req.URL.Path,
		Query:  req.URL.Query(),
		Header: req.Header.Clone(),
		Cookie: map[string]string{},
	}
	for _, ck := range req.Cookies() {
		c.Cookie[ck.Name] = ck.Value
	}
	if req.Body != nil {
		c.Body, _ = io.ReadAll(req.Body)
		req.Body.Close()
		if strings.HasPrefix(req.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			c.Form, _ = url.ParseQuery(string(c.Body))
		}
	}
	s.mu.Lock()
	s.calls = append(s.calls, c)
	h := s.handlers[c.Path]
	s.mu.Unlock()

	r := Reply{Status: http.StatusNotFound, Body: fmt.Sprintf("bilitest: 没有登记 %s", c.Path)}
	if h != nil {
		r = h(c)
	}
	if r.Status == 0 {
		r.Status = http.StatusOK
	}
	var body []byte
	switch b := r.Body.(type) {
	case nil:
		body = []byte(`{"code":0,"message":"0"}`)
	case string:
		body = []byte(b)
	case []byte:
		body = b
	default:
		body, _ = json.Marshal(b)
	}
	h2 := r.Header
	if h2 == nil {
		h2 = http.Header{}
	}
	return &http.Response{
		StatusCode: r.Status,
		Status:     http.StatusText(r.Status),
		Header:     h2,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Request:    req,
	}, nil
}
