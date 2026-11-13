package engine

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/VOfficial404/bili-tools/api"
)

type level int

const (
	levelDebug level = iota
	levelInfo
	levelWarn
	levelError
)

var levelNames = [...]string{"debug", "info", "warn", "error"}

func (l level) String() string { return levelNames[l] }

// parseLevel 认不出来的当作 info
func parseLevel(s string) level {
	for i, n := range levelNames {
		if n == strings.ToLower(strings.TrimSpace(s)) {
			return level(i)
		}
	}
	return levelInfo
}

func validLevel(s string) bool {
	for _, n := range levelNames {
		if n == s {
			return true
		}
	}
	return false
}

// logBook 在内存里留最近的日志给客户端看，同时写到 out
type logBook struct {
	mu      sync.Mutex
	entries []api.LogEntry
	seq     int64
	max     int
	min     level
	out     io.Writer
}

func newLogBook(max int, out io.Writer, min level) *logBook {
	return &logBook{max: max, out: out, min: min}
}

func (b *logBook) setLevel(min level) {
	b.mu.Lock()
	b.min = min
	b.mu.Unlock()
}

// add 记一条日志；who 是给人看的账号名，只用在 out 里
func (b *logBook) add(now time.Time, lv level, mid, who, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if lv < b.min {
		return
	}
	b.seq++
	b.entries = append(b.entries, api.LogEntry{Seq: b.seq, Time: now, Level: lv.String(), Mid: mid, Text: text})
	if len(b.entries) > b.max {
		b.entries = append(b.entries[:0], b.entries[len(b.entries)-b.max:]...)
	}
	if b.out != nil {
		tag := ""
		if lv != levelInfo {
			tag = "[" + strings.ToUpper(lv.String()) + "] "
		}
		if who != "" {
			tag += "[" + who + "] "
		}
		fmt.Fprintf(b.out, "%s %s%s\n", now.In(Beijing).Format("2006/01/02 15:04:05"), tag, text)
	}
}

// list 返回序号大于 after、级别不低于 min 的日志；mid 不为空时只要这个账号的
func (b *logBook) list(after int64, min level, mid string) []api.LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []api.LogEntry{}
	for _, e := range b.entries {
		if e.Seq > after && parseLevel(e.Level) >= min && (mid == "" || e.Mid == mid) {
			out = append(out, e)
		}
	}
	return out
}

func (e *Engine) logf(lv level, mid, format string, args ...any) {
	who := ""
	if mid != "" {
		who = "UID " + mid
		if a := e.account(mid); a != nil {
			who = a.name()
		}
	}
	e.logs.add(e.now(), lv, mid, who, fmt.Sprintf(format, args...))
}

func (e *Engine) Debugf(format string, args ...any) { e.logf(levelDebug, "", format, args...) }
func (e *Engine) Infof(format string, args ...any)  { e.logf(levelInfo, "", format, args...) }
func (e *Engine) Warnf(format string, args ...any)  { e.logf(levelWarn, "", format, args...) }
func (e *Engine) Errorf(format string, args ...any) { e.logf(levelError, "", format, args...) }
