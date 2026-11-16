package api

import (
	"regexp"
	"strconv"
	"strings"
)

// git describe 给出的「某个版本之后的第几个提交」，例如 v0.1.1-3-g83eee16
var describeSuffix = regexp.MustCompile(`^[0-9]+-g[0-9a-f]+$`)

// NewerVersion 判断 v 是否比 current 新。版本号形如 v1.2.3、1.2.3；
// 也认 git describe 的 v1.2.3-4-gabcdef，算作 1.2.3 之后、下一个版本之前的开发版本。
// 任一个认不出来（例如 dev、latest）时返回 false，不提示更新。
func NewerVersion(v, current string) bool {
	a, ok1 := parseVersion(v)
	b, ok2 := parseVersion(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// parseVersion 返回主、次、修订号和后缀的先后：-1 是预发布（1.2.3-rc1），0 是正式版，1 是之后的开发版本
func parseVersion(s string) ([4]int, bool) {
	var out [4]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	core, suffix, hasSuffix := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	switch {
	case !hasSuffix:
	case describeSuffix.MatchString(suffix):
		out[3] = 1
	default:
		out[3] = -1
	}
	return out, true
}
