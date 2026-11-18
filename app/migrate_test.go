package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeLegacy(t *testing.T, legacy string, kv map[string]string, withData bool) {
	t.Helper()
	if kv != nil {
		b, _ := json.Marshal(kv)
		if err := os.WriteFile(filepath.Join(legacy, "storage.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if withData {
		if err := os.MkdirAll(filepath.Join(legacy, "data"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(legacy, "data", "accounts.json"), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func readPrefs(t *testing.T, dir string) prefs {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p prefs
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// Android 上 NSGo 的数据就在 MyGo 数据目录的上一层
func TestMigrateNSGoRememberedServer(t *testing.T) {
	legacy := t.TempDir()
	user := filepath.Join(legacy, "Bili Daily")
	writeLegacy(t, legacy, map[string]string{"address": "https://bili.example.com", "remember": "true", "token": "secret", "local-proxy": "http://127.0.0.1:7890"}, true)
	var token string
	moved, err := migrateNSGo(legacy, user, func(v string) error { token = v; return nil })
	if err != nil || !moved {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	if token != "secret" {
		t.Fatalf("token %q", token)
	}
	if p := readPrefs(t, user); p != (prefs{Mode: 0, Server: "https://bili.example.com", Remember: true, Proxy: "http://127.0.0.1:7890"}) {
		t.Fatalf("prefs %+v", p)
	}
	if _, err := os.Stat(filepath.Join(user, "data", "accounts.json")); err != nil {
		t.Fatal("data not moved:", err)
	}
	if _, err := os.Stat(filepath.Join(legacy, "storage.json")); !os.IsNotExist(err) {
		t.Fatal("plaintext token left behind:", err)
	}
	// 第二次启动什么也不做
	if moved, err := migrateNSGo(legacy, user, func(string) error { t.Fatal("token set again"); return nil }); moved || err != nil {
		t.Fatalf("second run moved=%v err=%v", moved, err)
	}
}

func TestMigrateNSGoLocalOnly(t *testing.T) {
	legacy := t.TempDir()
	user := filepath.Join(legacy, "Bili Daily")
	writeLegacy(t, legacy, map[string]string{"local-proxy": "socks5://127.0.0.1:1080"}, true)
	moved, err := migrateNSGo(legacy, user, func(string) error { t.Fatal("no token to set"); return nil })
	if err != nil || !moved {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	if p := readPrefs(t, user); p != (prefs{Mode: 1, Server: defaultServer, Proxy: "socks5://127.0.0.1:1080"}) {
		t.Fatalf("prefs %+v", p)
	}
}

func TestMigrateNSGoFreshInstall(t *testing.T) {
	legacy := t.TempDir()
	user := filepath.Join(legacy, "Bili Daily")
	if moved, err := migrateNSGo(legacy, user, nil); moved || err != nil {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	if _, err := os.Stat(user); !os.IsNotExist(err) {
		t.Fatal("fresh install should not create the data directory")
	}
}
