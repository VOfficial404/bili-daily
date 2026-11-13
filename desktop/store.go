package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

type prefs struct {
	Mode      int    `json:"mode"`
	Server    string `json:"server"`
	Remember  bool   `json:"remember"`
	Proxy     string `json:"proxy"`
	AutoLocal bool   `json:"auto_local"`
}

type store interface {
	load() prefs
	save(prefs) error
	token() (string, error)
	setToken(string) error
	dataDir() (string, error)
}

type systemStore struct{}

func (systemStore) dir() (string, error) { return mygo.App.Path(mygo.PathUserData) }
func (s systemStore) load() prefs {
	p := prefs{Server: "http://127.0.0.1:8686"}
	if d, e := s.dir(); e == nil {
		if b, e := os.ReadFile(filepath.Join(d, "prefs.json")); e == nil {
			_ = json.Unmarshal(b, &p)
		}
	}
	return p
}
func (s systemStore) save(p prefs) error {
	d, e := s.dir()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(d, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(d, "prefs.json"), b, 0600)
}
func (systemStore) token() (string, error) {
	b, e := mygo.Secrets.Get("server-token")
	return string(b), e
}
func (systemStore) setToken(t string) error {
	if t != "" {
		return mygo.Secrets.Set("server-token", []byte(t))
	}
	e := mygo.Secrets.Delete("server-token")
	if errors.Is(e, mygo.ErrSecretNotFound) {
		return nil
	}
	return e
}
func (s systemStore) dataDir() (string, error) { d, e := s.dir(); return filepath.Join(d, "data"), e }
