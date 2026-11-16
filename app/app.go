package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/client"
	"github.com/VOfficial404/bili-daily/engine"
	"github.com/egoist/mygo/ui"
)

const version = "0.1.2"

// app 的界面状态只在主线程读写，后台请求通过 post 返回结果。
type app struct {
	post                            func(func())
	store                           store
	wg                              sync.WaitGroup
	mode                            int
	server, token, proxy            string
	remember, autoLocal, connecting bool
	connectErr, loadErr, formErr    string
	session                         *client.Session
	st                              *api.State
	form                            api.Settings
	formBase                        api.Settings // form 载入时服务端的设置，保存时只提交相对它改过的项
	chargeConfirmed                 bool
	guardOpen                       bool
	guardTitle, guardText, guardOK  string
	guardAction                     func()
	dirty, saving                   bool
	inputs                          map[string]string
	taskOptions                     map[string]bool
	busy                            map[string]bool
	newEngine                       func(engine.Config) (*engine.Engine, error)
	onLocal                         func(bool)
	page                            string
	narrow                          bool
	width, height                   float32
	epoch, ticks                    int
	loading                         bool
	ctx                             context.Context
	cancel                          context.CancelFunc
	toasts                          []string
	qrOpen, qrBusy                  bool
	qr                              api.QR
	qrImage                         *ui.Bitmap
	qrErr                           string
	qrGen                           int
	runs                            []api.RunSummary
	report                          *api.Run
	reportMid, reportErr            string
	reportsBusy                     bool
	logs                            []api.LogEntry
	logSeq                          int64
	logLevel, logMid                string
	logsBusy                        bool
	logScroll                       ui.ScrollState
	autoScroll                      bool
	confirmOpen                     bool
	confirmTitle, confirmText       string
	confirmAction                   func()
	pickOpen                        bool
	pickTask, pickMid               string
	groups                          []api.Group
	groupsBusy                      bool
	groupIdx                        int
	unfollowCount, unfollowKeep     string
	pickErr                         string
	faces                           map[string]*ui.Bitmap
	faceTried                       map[string]bool
	upProfiles                      map[string]*upProfilePreview
	upPicker                        *upPickerState
	upPaste                         [2]upPasteInput
	fetchImage                      func(string) ([]byte, error)
	updateOpen                      bool
	openURL                         func(string) error // 用系统浏览器打开网址，由平台入口设置
}

func newApp(s store, post func(func())) *app {
	p := s.load()
	ctx, cancel := context.WithCancel(context.Background())
	return &app{post: post, store: s, session: &client.Session{}, mode: p.Mode, server: p.Server, remember: p.Remember, proxy: p.Proxy, autoLocal: p.AutoLocal, page: "今天", busy: map[string]bool{}, inputs: map[string]string{}, newEngine: engine.New, ctx: ctx, cancel: cancel, autoScroll: true, faces: map[string]*ui.Bitmap{}, faceTried: map[string]bool{}, fetchImage: downloadImage}
}
func (a *app) async(work func() func()) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if apply := work(); apply != nil {
			a.post(apply)
		}
	}()
}
func (a *app) toast(s string) { a.toasts = append(a.toasts, s) }
func (a *app) savePrefs() error {
	p := prefs{Mode: a.mode, Server: a.server, Remember: a.remember, Proxy: a.proxy, AutoLocal: a.autoLocal}
	if err := a.store.save(p); err != nil {
		return err
	}
	token := ""
	if a.remember && a.mode == 0 {
		token = a.token
	}
	return a.store.setToken(token)
}
func (a *app) autoConnect() {
	if a.mode == 1 {
		if a.autoLocal {
			a.startLocal()
		}
		return
	}
	if a.remember {
		if t, e := a.store.token(); e == nil {
			a.token = t
			a.connect()
		}
	}
}
func (a *app) connected(session *client.Session, st api.State) {
	a.session, a.st = session, &st
	a.form = client.CloneSettings(st.Settings)
	a.formBase = client.CloneSettings(st.Settings)
	a.dirty = false
	a.inputs = map[string]string{}
	a.upProfiles = nil
	a.upPicker = nil
	a.upPaste = [2]upPasteInput{}
	a.connectErr = ""
	a.loadErr = ""
	if err := a.savePrefs(); err != nil {
		a.toast("连接成功，但保存连接信息失败：" + err.Error())
	}
	a.loadFaces()
	a.preloadSavedUPProfiles()
}
func (a *app) connect() {
	if a.connecting {
		return
	}
	base, err := api.NormalizeBase(a.server)
	if err != nil {
		a.connectErr = err.Error()
		return
	}
	a.server = base
	a.connecting = true
	a.connectErr = ""
	session := &client.Session{}
	a.session = session
	ctx, token := a.ctx, a.token
	epoch := a.epoch
	a.async(func() func() {
		st, err := session.Connect(ctx, base, token)
		return func() {
			if epoch != a.epoch {
				return
			}
			a.connecting = false
			if err != nil {
				if api.IsUnauthorized(err) {
					a.connectErr = "令牌不对，请检查服务器令牌"
				} else {
					a.connectErr = "连接失败：" + err.Error()
				}
				return
			}
			a.connected(session, st)
		}
	})
}
func (a *app) startLocal() {
	if a.connecting {
		return
	}
	if _, err := engine.ParseProxy(a.proxy); err != nil {
		a.connectErr = err.Error()
		return
	}
	a.connecting = true
	a.connectErr = ""
	proxy := strings.TrimSpace(a.proxy)
	factory := a.newEngine
	session := &client.Session{}
	a.session = session
	ctx := a.ctx
	epoch := a.epoch
	a.async(func() func() {
		dir, err := a.store.dataDir()
		var st api.State
		if err == nil {
			st, err = session.StartLocal(ctx, client.LocalOptions{
				Config: engine.Config{DataDir: dir, Version: version, LogOutput: os.Stderr},
				Proxy:  &proxy, NewEngine: factory,
			})
		}
		return func() {
			if epoch != a.epoch {
				return
			}
			a.connecting = false
			if err != nil {
				a.connectErr = "本机运行失败：" + err.Error()
				return
			}
			a.connected(session, st)
			if a.onLocal != nil {
				a.onLocal(true)
			}
		}
	})
}
func (a *app) disconnect() {
	a.epoch++
	a.cancel()
	a.ctx, a.cancel = context.WithCancel(context.Background())
	session := a.session
	local := session.LocalEngine() != nil
	a.st = nil
	a.loading = false
	a.connecting = false
	a.qrOpen = false
	a.qrGen++
	a.qrBusy = false
	a.saving = false
	a.logs = nil
	a.logSeq = 0
	a.report = nil
	a.runs = nil
	a.busy = map[string]bool{}
	a.logsBusy = false
	a.reportsBusy = false
	a.pickOpen = false
	a.groupsBusy = false
	a.upProfiles = nil
	a.upPicker = nil
	a.upPaste = [2]upPasteInput{}
	if a.onLocal != nil {
		a.onLocal(false)
	}
	a.connecting = true
	epoch := a.epoch
	a.async(func() func() {
		session.Stop()
		return func() {
			if epoch != a.epoch {
				return
			}
			a.session = &client.Session{}
			a.connecting = false
			if local {
				a.toast("本机运行已停止")
			}
		}
	})
}
func (a *app) tick() {
	a.ticks++
	if a.st == nil || a.session.Client() == nil {
		return
	}
	interval := 3
	for _, acc := range a.st.Accounts {
		if acc.Running != nil {
			interval = 1
		}
	}
	if a.ticks%interval == 0 {
		a.poll()
	}
	if a.qrOpen && a.ticks%2 == 0 {
		a.pollQR()
	}
	if a.page == "日志" {
		a.loadLogs()
	}
}
func (a *app) poll() {
	if a.loading || a.session.Client() == nil {
		return
	}
	a.loading = true
	session, ctx, epoch := a.session, a.ctx, a.epoch
	a.async(func() func() {
		_, err := session.Refresh(ctx)
		return func() {
			if epoch != a.epoch {
				return
			}
			a.loading = false
			if err != nil {
				a.loadErr = "无法更新状态：" + err.Error()
				return
			}
			a.loadErr = ""
			// 应用回调执行前可能保存过设置，以会话当前快照为准。
			st := session.Snapshot()
			a.st = &st
			if !a.dirty && !a.saving {
				a.form = client.CloneSettings(st.Settings)
				a.formBase = client.CloneSettings(st.Settings)
				a.inputs = map[string]string{}
			}
			a.loadFaces()
		}
	})
}
func (a *app) action(key, success string, work func(*api.Client, context.Context) error) {
	if a.session.Client() == nil || a.busy[key] {
		return
	}
	a.busy[key] = true
	cl, ctx, epoch := a.session.Client(), a.ctx, a.epoch
	a.async(func() func() {
		err := work(cl, ctx)
		return func() {
			if epoch != a.epoch {
				return
			}
			a.busy[key] = false
			if err != nil {
				a.toast(err.Error())
			} else {
				a.toast(success)
				a.poll()
			}
		}
	})
}
func (a *app) runAll() {
	a.action("all", "已安排全部账号运行", func(c *api.Client, ctx context.Context) error { return c.RunAll(ctx) })
}
func (a *app) run(mid string, tasks []string) {
	a.action(mid, "已安排运行", func(c *api.Client, ctx context.Context) error { return c.Run(ctx, mid, tasks) })
}
func (a *app) saveSettings() {
	if a.session.Client() == nil || a.saving {
		return
	}
	if a.needsChargeConfirm() {
		return
	}
	a.chargeConfirmed = false
	a.saving = true
	a.formErr = ""
	base, edited := client.CloneSettings(a.formBase), client.CloneSettings(a.form)
	session, ctx, epoch := a.session, a.ctx, a.epoch
	a.async(func() func() {
		saved, err := session.SaveSettings(ctx, base, edited)
		return func() {
			if epoch != a.epoch {
				return
			}
			a.saving = false
			if err != nil {
				a.formErr = err.Error()
				return
			}
			a.form = saved
			a.formBase = client.CloneSettings(saved)
			a.st.Settings = client.CloneSettings(saved)
			if a.session.LocalEngine() != nil && a.proxy != saved.Proxy {
				// 下次本机启动用这里保存的代理
				a.proxy = saved.Proxy
				if err := a.savePrefs(); err != nil {
					a.toast("保存代理到本机设置失败：" + err.Error())
				}
			}
			a.inputs = map[string]string{}
			a.dirty = false
			a.preloadSavedUPProfiles()
			a.toast("设置已保存")
		}
	})
}
func (a *app) testProxy(proxy string) {
	if a.session.Client() == nil {
		if _, err := engine.ParseProxy(proxy); err != nil {
			a.connectErr = err.Error()
		} else {
			a.connectErr = "地址格式正确；开始本机运行后可测试出口 IP"
		}
		return
	}
	epoch := a.epoch
	a.action("proxy", "代理测试完成", func(c *api.Client, ctx context.Context) error {
		eg, err := c.TestProxy(ctx, proxy)
		if err == nil && eg.Error != "" {
			return fmt.Errorf("%s", eg.Error)
		}
		if err == nil {
			a.post(func() {
				if epoch == a.epoch {
					a.toast("出口 IP：" + eg.Addr + " " + eg.Place)
				}
			})
		}
		return err
	})
}
func (a *app) testNotify() {
	n := a.form.Notify
	a.action("notify", "测试推送已发送", func(c *api.Client, ctx context.Context) error { return c.TestNotify(ctx, n) })
}
func (a *app) startQR() {
	if a.session.Client() == nil || a.qrBusy {
		return
	}
	a.qrOpen = true
	a.qrBusy = true
	a.qrErr = ""
	a.qrImage = nil
	a.qr = api.QR{}
	a.qrGen++
	gen, cl, ctx, epoch := a.qrGen, a.session.Client(), a.ctx, a.epoch
	a.async(func() func() {
		q, err := cl.StartQR(ctx)
		return func() {
			if gen != a.qrGen || epoch != a.epoch {
				return
			}
			a.qrBusy = false
			if err == nil {
				a.qrImage, err = ui.DecodeBitmap(q.PNG)
			}
			if err != nil {
				a.qrErr = err.Error()
				return
			}
			a.qr = q
		}
	})
}
func (a *app) pollQR() {
	if !a.qrOpen || a.qrBusy || a.qr.ID == "" || a.qr.State == "expired" {
		return
	}
	a.qrBusy = true
	gen, cl, ctx, id, epoch := a.qrGen, a.session.Client(), a.ctx, a.qr.ID, a.epoch
	a.async(func() func() {
		q, err := cl.PollQR(ctx, id)
		return func() {
			if gen != a.qrGen || epoch != a.epoch {
				return
			}
			a.qrBusy = false
			if err != nil {
				a.qrErr = err.Error()
				return
			}
			a.qrErr = ""
			a.qr.State = q.State
			if q.State == "success" {
				a.qrOpen = false
				a.page = "账号"
				a.toast("账号已添加")
				a.poll()
			}
		}
	})
}
func (a *app) loadReports() {
	if a.session.Client() == nil || a.reportsBusy {
		return
	}
	a.reportsBusy = true
	cl, ctx, mid, epoch := a.session.Client(), a.ctx, a.reportMid, a.epoch
	a.async(func() func() {
		runs, err := cl.Runs(ctx, mid, 60)
		return func() {
			if epoch != a.epoch {
				return
			}
			a.reportsBusy = false
			if mid != a.reportMid {
				a.loadReports()
				return
			}
			if err != nil {
				a.reportErr = err.Error()
				return
			}
			a.reportErr = ""
			a.runs = runs
		}
	})
}
func (a *app) openReport(id string) {
	a.page = "报告"
	a.report = nil
	a.reportsBusy = true
	cl, ctx, epoch := a.session.Client(), a.ctx, a.epoch
	a.async(func() func() {
		r, err := cl.RunDetail(ctx, id)
		return func() {
			if epoch != a.epoch {
				return
			}
			a.reportsBusy = false
			if err != nil {
				a.reportErr = err.Error()
				return
			}
			a.reportErr = ""
			a.report = &r
		}
	})
}
func (a *app) loadLogs() {
	if a.session.Client() == nil || a.logsBusy {
		return
	}
	a.logsBusy = true
	cl, ctx, seq, level, mid, epoch := a.session.Client(), a.ctx, a.logSeq, a.logLevel, a.logMid, a.epoch
	a.async(func() func() {
		logs, err := cl.Logs(ctx, seq, level, mid)
		return func() {
			if epoch != a.epoch {
				return
			}
			a.logsBusy = false
			if level != a.logLevel || mid != a.logMid {
				a.loadLogs()
				return
			}
			if err != nil {
				a.loadErr = "无法读取日志：" + err.Error()
				return
			}
			for _, l := range logs {
				if l.Seq > a.logSeq {
					a.logs = append(a.logs, l)
					a.logSeq = l.Seq
				}
			}
			if len(a.logs) > 2000 {
				a.logs = slices.Clone(a.logs[len(a.logs)-2000:])
			}
			if a.autoScroll {
				a.logScroll.Y = math.MaxFloat32
			}
		}
	})
}
func (a *app) runTask(id string) {
	if len(a.st.Accounts) == 0 {
		a.toast("请先添加账号")
		return
	}
	a.pickTask = id
	a.pickMid = ""
	a.pickErr = ""
	a.groups = nil
	a.groupIdx = 0
	a.unfollowCount = fmt.Sprint(a.form.Tasks.Unfollow.Count)
	a.unfollowKeep = joinIDs(a.form.Tasks.Unfollow.Keep)
	if len(a.st.Accounts) == 1 && id != "unfollow" {
		a.run(a.st.Accounts[0].Mid, []string{id})
		return
	}
	a.pickOpen = true
}
func (a *app) chooseRunAccount(mid string) {
	a.pickMid = mid
	if a.pickTask != "unfollow" {
		a.pickOpen = false
		a.run(mid, []string{a.pickTask})
		return
	}
	a.groupsBusy = true
	cl, ctx, epoch := a.session.Client(), a.ctx, a.epoch
	a.async(func() func() {
		groups, err := cl.Groups(ctx, mid)
		return func() {
			if epoch != a.epoch {
				return
			}
			a.groupsBusy = false
			if a.pickMid != mid {
				return
			}
			if err != nil {
				a.pickErr = err.Error()
				return
			}
			a.groups = groups
			a.groupIdx = 0
			for i, g := range groups {
				if g.Name == a.form.Tasks.Unfollow.Group {
					a.groupIdx = i
				}
			}
		}
	})
}
func downloadImage(u string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return nil, e
	}
	resp, e := http.DefaultClient.Do(r)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("头像暂时不可用")
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}
func (a *app) loadFaces() {
	for _, acc := range a.st.Accounts {
		a.loadFace(acc.Face)
	}
}

func (a *app) loadFace(u string) {
	if u == "" || a.faceTried[u] {
		return
	}
	a.faceTried[u] = true
	fetch := a.fetchImage
	a.async(func() func() {
		b, err := fetch(u)
		return func() {
			if err == nil {
				if bm, err := ui.DecodeBitmap(b); err == nil {
					a.faces[u] = bm
				}
			}
		}
	})
}
