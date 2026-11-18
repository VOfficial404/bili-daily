package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// migrateNSGo 把 NSGo 版手机 App 留下的东西搬到这一版的数据目录 userData：
// 本机运行的数据目录 legacy/data（账号、报告），以及 legacy/storage.json 里的服务器地址、代理和记住的令牌。
// 令牌在 NSGo 里是明文，搬进系统的凭据存储后连同 storage.json 一起删掉。
// storage.json 不在了说明搬过，或者本来就是这一版。返回这次是否搬来了连接偏好。
func migrateNSGo(legacy, userData string, setToken func(string) error) (bool, error) {
	storage := filepath.Join(legacy, "storage.json")
	_, err := os.Stat(filepath.Join(userData, "prefs.json"))
	havePrefs := err == nil
	var kv map[string]string
	b, err := os.ReadFile(storage)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &kv); err != nil {
			return false, err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return false, err
	case havePrefs:
		return false, nil
	}
	if havePrefs {
		// 上次搬到一半，之后用户已经在这一版里连接过：以这一版的偏好和令牌为准，只删掉明文令牌
		return false, removeIfExists(storage)
	}
	oldData, newData := filepath.Join(legacy, "data"), filepath.Join(userData, "data")
	if info, err := os.Stat(oldData); err == nil && info.IsDir() {
		if _, err := os.Stat(newData); errors.Is(err, fs.ErrNotExist) {
			if err := os.MkdirAll(userData, 0700); err != nil {
				return false, err
			}
			if err := os.Rename(oldData, newData); err != nil {
				return false, err
			}
		}
	}
	// 中途失败时 data 可能已经搬过来了：看新位置有没有
	_, err = os.Stat(newData)
	hasLocal := err == nil
	if kv == nil && !hasLocal {
		return false, nil
	}
	p := prefs{Server: defaultServer, Proxy: kv["local-proxy"]}
	if kv["address"] != "" {
		p.Server = kv["address"]
	}
	if kv["remember"] == "true" && kv["token"] != "" {
		if err := setToken(kv["token"]); err != nil {
			return false, err
		}
		p.Remember = true
	} else if hasLocal {
		// 旧版在本机运行：打开就启动引擎，后台任务才能在系统新起的进程里补跑
		p.Mode, p.AutoLocal = 1, true
	}
	if err := writePrefs(userData, p); err != nil {
		return false, err
	}
	return true, removeIfExists(storage)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
