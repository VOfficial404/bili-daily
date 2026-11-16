package api

import "testing"

func TestNewerVersion(t *testing.T) {
	for _, tc := range []struct {
		v, current string
		want       bool
	}{
		{"v0.1.2", "0.1.1", true},
		{"v0.2.0", "v0.1.9", true},
		{"v0.10.0", "v0.9.0", true},
		{"v0.1.1", "v0.1.1", false},
		{"v0.1.0", "v0.1.1", false},
		// main 上构建的开发版本：比它所在的版本新，比下一个版本旧
		{"v0.1.1", "v0.1.1-3-g83eee16", false},
		{"v0.1.2", "v0.1.1-3-g83eee16", true},
		{"v0.1.1", "v0.1.1-rc1", true},
		// 认不出来的不提示
		{"v0.1.2", "latest", false},
		{"v0.1.2", "dev", false},
		{"v0.1.2", "83eee16", false},
		{"nightly", "v0.1.1", false},
	} {
		if got := NewerVersion(tc.v, tc.current); got != tc.want {
			t.Errorf("NewerVersion(%q, %q) = %v", tc.v, tc.current, got)
		}
	}
}
