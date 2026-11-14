// Package engine 是 bili-daily 的服务端：账号、每日计划、任务、运行报告和给客户端用的接口。
// 服务器上的 bili-daily 命令、桌面客户端和手机 App 的「本机运行」用的都是它。
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/bili"
)

type Config struct {
	DataDir   string
	Version   string
	Token     string            // 接口令牌。空时用 DataDir/token 里的，没有就生成一个存进去
	Local     bool              // 在桌面或手机 App 里运行，而不是服务器上
	LogOutput io.Writer         // 日志除了留在内存里给客户端看，还写到哪里；可以为 nil
	Panel     http.Handler      // 接口之外的路径交给它，服务器上是网页面板；nil 时只有接口
	Transport http.RoundTripper // 不为 nil 时代替真实网络和代理，测试用
	Now       func() time.Time  // 测试用，nil 表示 time.Now
	// Sleep 等 d 或者 ctx 取消。测试里换成不等待的；nil 表示真的等。
	Sleep func(ctx context.Context, d time.Duration) error
}

// Beijing 是北京时间。B 站按它算「每天」，所以计划和报告里的日期都用它，和服务器所在时区无关。
var Beijing = time.FixedZone("CST", 8*3600)

var (
	errNoAccount = errors.New("找不到这个账号")
	errBusy      = errors.New("这个账号正在运行，等这一轮结束再试")
)

// Engine 管理所有账号的排程。一个数据目录只能有一个 Engine 在运行。
type Engine struct {
	cfg   Config
	logs  *logBook
	token string

	mu         sync.Mutex
	settingsMu sync.Mutex // 包括部分 PUT 的读取、保存与应用
	loginMu    sync.Mutex // 串行化扫码会话提交和新账号注册
	runsMu     sync.Mutex // 串行化运行报告保存、清理和对应的 LastRun 更新
	set        api.Settings
	accounts   map[string]*account
	egress     *api.Egress
	qrs        map[string]*qrLogin
	transports map[string]http.RoundTripper
	started    bool
	srv        *http.Server
	listener   net.Listener // Serve 开始前登记，Stop 不依赖后台 goroutine 的启动时机

	egressKick chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
	stopOnce   sync.Once
	workers    sync.WaitGroup // admission is serialized with Stop by mu
}

// New 打开数据目录。还没有开始排程，要调用 Start。
func New(cfg Config) (*Engine, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("没有指定数据目录")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建数据目录失败：%w", err)
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepCtx
	}
	e := &Engine{
		cfg:        cfg,
		accounts:   map[string]*account{},
		qrs:        map[string]*qrLogin{},
		transports: map[string]http.RoundTripper{},
		egressKick: make(chan struct{}, 1),
	}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.set = e.loadSettings()
	e.logs = newLogBook(2000, cfg.LogOutput, parseLevel(e.set.LogLevel))
	token, err := e.loadToken()
	if err != nil {
		return nil, err
	}
	e.token = token
	for _, mid := range e.listAccounts() {
		e.accounts[mid] = e.newAccount(mid)
	}
	e.pruneReports()
	return e, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// loadToken 决定接口令牌：配置里给的优先，其次是数据目录里存的，都没有就生成
func (e *Engine) loadToken() (string, error) {
	if t := strings.TrimSpace(e.cfg.Token); t != "" {
		return t, nil
	}
	file := filepath.Join(e.cfg.DataDir, "token")
	if b, err := os.ReadFile(file); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
	}
	t := randomHex(20)
	if err := os.WriteFile(file, []byte(t+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("保存接口令牌失败：%w", err)
	}
	return t, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Token 是客户端连接时要填的令牌
func (e *Engine) Token() string { return e.token }

func (e *Engine) DataDir() string { return e.cfg.DataDir }

func (e *Engine) now() time.Time { return e.cfg.Now() }

// day 是 t 在北京时间的日期
func day(t time.Time) string { return t.In(Beijing).Format("2006-01-02") }

func (e *Engine) Settings() api.Settings {
	e.mu.Lock()
	defer e.mu.Unlock()
	return cloneSettings(e.set)
}

// Start 开始排程：每个账号按每日计划运行，定时查询出口 IP
func (e *Engine) Start() {
	e.mu.Lock()
	if e.started || e.ctx.Err() != nil {
		e.mu.Unlock()
		return
	}
	e.started = true
	accs := make([]*account, 0, len(e.accounts))
	for _, a := range e.accounts {
		accs = append(accs, a)
	}
	e.workers.Add(len(accs) + 3)
	e.mu.Unlock()

	s := e.Settings()
	where := "直连"
	if s.Proxy != "" {
		where = "经代理 " + redactProxy(s.Proxy)
	}
	e.Infof("bili-daily %s 启动：每天 %s（北京时间）开始，随机推迟 0~%d 分钟，%s，数据目录 %s", e.cfg.Version, s.RunAt, s.Jitter, where, e.cfg.DataDir)
	if len(accs) == 0 {
		e.Infof("还没有账号：在客户端里点「添加账号」，或者运行 bili-daily login")
	} else {
		e.Infof("已加载 %d 个账号", len(accs))
	}
	for _, a := range accs {
		go func() { defer e.workers.Done(); a.loop() }()
	}
	go func() { defer e.workers.Done(); e.egressLoop() }()
	go func() { defer e.workers.Done(); e.rescanLoop() }()
	go func() { defer e.workers.Done(); e.reportRetentionLoop() }()
}

// Stop 中止正在进行的运行，关闭接口，并等待后台循环和运行报告保存完毕。可以重复或并发调用。
func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		e.mu.Lock()
		e.cancel()
		srv, ln := e.srv, e.listener
		e.mu.Unlock()
		if srv != nil {
			srv.Close()
		}
		if ln != nil {
			ln.Close()
		}
	})
	e.workers.Wait()
}

// beginWork prevents a direct RunNow from entering after Stop begins waiting.
func (e *Engine) beginWork() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ctx.Err() != nil {
		return false
	}
	e.workers.Add(1)
	return true
}

func (e *Engine) account(mid string) *account {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.accounts[mid]
}

// accountList 返回所有账号，按 UID 排序
func (e *Engine) accountList() []*account {
	e.mu.Lock()
	defer e.mu.Unlock()
	list := make([]*account, 0, len(e.accounts))
	for _, a := range e.accounts {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool {
		if len(list[i].mid) != len(list[j].mid) {
			return len(list[i].mid) < len(list[j].mid)
		}
		return list[i].mid < list[j].mid
	})
	return list
}

// accountLoggedIn 在扫码成功后调用：新账号开始排程，已有账号恢复排程
func (e *Engine) accountLoggedIn(mid string) {
	e.mu.Lock()
	if e.ctx.Err() != nil {
		e.mu.Unlock()
		return
	}
	e.workers.Add(1)
	defer e.workers.Done()
	a, ok := e.accounts[mid]
	if !ok {
		a = e.newAccount(mid)
		e.accounts[mid] = a
		if e.started {
			e.workers.Add(1)
			go func() { defer e.workers.Done(); a.loop() }()
		}
	}
	e.mu.Unlock()
	a.update(func(st *accountState) { st.LoggedIn = true })
	a.poke()
}

// rescanLoop 每分钟看一次数据目录：命令行 login 新加的账号会被接上
func (e *Engine) rescanLoop() {
	// 这里是真的等一分钟，不走 cfg.Sleep：测试里 Sleep 不等待，循环会空转
	for sleepCtx(e.ctx, time.Minute) == nil {
		for _, mid := range e.listAccounts() {
			if e.account(mid) == nil {
				e.Infof("发现新账号 UID %s", mid)
				e.accountLoggedIn(mid)
			}
		}
	}
}

// removeAccount 让 B 站注销会话，删除本地数据并停止排程。远端注销失败也会删除本地数据。
func (e *Engine) removeAccount(ctx context.Context, mid string) error {
	a := e.account(mid)
	if a == nil {
		return errNoAccount
	}
	// 先关准入和排程，再等当前运行全部落盘；请求取消后清理仍会完成。
	done := a.close()
	result := make(chan error, 1)
	go func() {
		a.lifecycleMu.Lock()
		defer a.lifecycleMu.Unlock()
		if done != nil {
			<-done
		}
		name := a.name()
		if c, err := e.accountClient(mid); err == nil {
			c.Wait = nil
			if err := c.Logout(ctx); err != nil {
				a.warnf("B 站注销会话失败（本地会话仍会删除）：%v", err)
			}
		}
		if err := os.RemoveAll(e.accountDir(mid)); err != nil {
			result <- err
			return
		}
		e.mu.Lock()
		delete(e.accounts, mid)
		e.mu.Unlock()
		e.Infof("已移除账号 %s", name)
		result <- nil
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Wake 让所有账号重新看一次排程。手机 App 回到前台时调用：
// 手机睡眠期间错过的那一轮立即补上，不用等计时器。
func (e *Engine) Wake() {
	for _, a := range e.accountList() {
		a.poke()
	}
}

// RunAll 让所有参加每日计划的空闲账号立即跑一轮，返回实际触发的数量
func (e *Engine) RunAll() int {
	n := 0
	for _, a := range e.accountList() {
		if a.snapshot().Enabled && a.runNow(nil) {
			n++
		}
	}
	return n
}

// RunNow 立即给一个账号跑一轮并等它结束。ids 为空表示所有开着的任务。
func (e *Engine) RunNow(ctx context.Context, mid string, ids []string) (*api.Run, error) {
	a := e.account(mid)
	if a == nil {
		return nil, errNoAccount
	}
	for _, id := range ids {
		if _, ok := registry[id]; !ok {
			return nil, fmt.Errorf("没有这个任务：%s", id)
		}
	}
	run := a.execute(ctx, "manual", ids)
	if run == nil {
		return nil, errBusy
	}
	return run, nil
}

// Accounts 返回所有账号的 UID
func (e *Engine) Accounts() []string {
	var mids []string
	for _, a := range e.accountList() {
		mids = append(mids, a.mid)
	}
	return mids
}

// State 是界面要显示的全部状态
func (e *Engine) State() api.State {
	st := api.State{
		Version:  e.cfg.Version,
		Local:    e.cfg.Local,
		Now:      e.now(),
		Accounts: []api.Account{},
		Tasks:    []api.TaskInfo{},
		Settings: e.Settings(),
	}
	for _, t := range allTasks() {
		st.Tasks = append(st.Tasks, t.Info)
	}
	for _, a := range e.accountList() {
		st.Accounts = append(st.Accounts, a.status())
	}
	e.mu.Lock()
	if e.egress != nil {
		eg := *e.egress
		st.Egress = &eg
	}
	e.mu.Unlock()
	return st
}

/* 网络 */

// ParseProxy 解析代理地址；空字符串表示直连，返回 nil
func ParseProxy(s string) (*url.URL, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return nil, errors.New("代理地址不对，应写成 http://127.0.0.1:7890 或 socks5://127.0.0.1:7891")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return u, nil
	}
	return nil, errors.New("代理只支持 http://、https:// 和 socks5://")
}

// redactProxy 去掉代理地址里的用户名和密码，给日志用
func redactProxy(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "（地址无法解析）"
	}
	u.User = nil
	return u.String()
}

// transport 按代理地址缓存，换代理时用新的连接池
func (e *Engine) transport(proxy string) http.RoundTripper {
	if e.cfg.Transport != nil {
		return e.cfg.Transport
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if t, ok := e.transports[proxy]; ok {
		return t
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	if u, err := ParseProxy(proxy); err == nil && u != nil {
		t.Proxy = http.ProxyURL(u)
	}
	e.transports[proxy] = t
	return t
}

// anonClient 不带账号，用于扫码、查询出口 IP 等
func (e *Engine) anonClient() *bili.Client {
	c := bili.New(e.transport(e.Settings().Proxy), nil)
	c.Debugf = e.Debugf
	return c
}

// accountClient 返回以账号身份请求 B 站的客户端，请求之间按设置的节奏留间隔
func (e *Engine) accountClient(mid string) (*bili.Client, error) {
	sess, err := e.loadSession(mid)
	if err != nil {
		return nil, err
	}
	c := bili.New(e.transport(e.Settings().Proxy), sess.Cookies)
	c.Wait = e.pacer()
	c.Debugf = func(format string, args ...any) { e.logf(levelDebug, mid, format, args...) }
	return c, nil
}

// paceRange 是每种节奏下两个请求之间的间隔范围
func paceRange(pace string) (min, max time.Duration) {
	switch pace {
	case "slow":
		return 4 * time.Second, 9 * time.Second
	case "fast":
		return 400 * time.Millisecond, 1200 * time.Millisecond
	}
	return 1500 * time.Millisecond, 4 * time.Second
}

// pacer 返回一个客户端用的等待函数：每个请求和上一个请求之间隔一段随机时间
func (e *Engine) pacer() func(ctx context.Context) error {
	var (
		mu   sync.Mutex
		next time.Time // 下一个请求最早什么时候可以发
	)
	return func(ctx context.Context) error {
		min, max := paceRange(e.Settings().Pace)
		gap := min + time.Duration(mrand.Int64N(int64(max-min)+1))
		now := e.now()
		mu.Lock()
		wait := next.Sub(now)
		if wait < 0 {
			wait = 0
		}
		next = now.Add(wait + gap)
		mu.Unlock()
		return e.cfg.Sleep(ctx, wait)
	}
}

// probe 查询经过 proxy 时 B 站看到的出口 IP
func (e *Engine) probe(ctx context.Context, proxy string) (*api.Egress, error) {
	if _, err := ParseProxy(proxy); err != nil {
		return nil, err
	}
	z, err := bili.New(e.transport(proxy), nil).Zone(ctx)
	if err != nil {
		return nil, err
	}
	return &api.Egress{Addr: z.Addr, Place: z.Place(), ISP: z.ISP, CheckedAt: e.now()}, nil
}

// egressLoop 定时查询 B 站把出口 IP 识别为哪里；换代理后立即重新查
func (e *Engine) egressLoop() {
	for {
		ctx, cancel := context.WithTimeout(e.ctx, 30*time.Second)
		eg, err := e.probe(ctx, e.Settings().Proxy)
		cancel()
		wait := 30 * time.Minute
		if err != nil {
			if e.ctx.Err() != nil {
				return
			}
			eg = &api.Egress{CheckedAt: e.now(), Error: err.Error()}
			e.Warnf("查询出口 IP 失败：%v", err)
			wait = 5 * time.Minute
		} else {
			e.Debugf("B 站把出口 IP %s 识别为 %s %s", eg.Addr, eg.Place, eg.ISP)
		}
		e.mu.Lock()
		e.egress = eg
		e.mu.Unlock()

		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-e.egressKick:
			t.Stop()
		case <-e.ctx.Done():
			t.Stop()
			return
		}
	}
}

/* 接口服务 */

// ListenAndServe 在 addr 上提供接口，直到出错或 Stop
func (e *Engine) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv, err := e.prepareServer(ln)
	if err != nil {
		return err
	}
	return e.serve(srv, ln)
}

// ServeLocal 在本机的随机端口上提供接口，返回地址（http://127.0.0.1:端口）。
// 桌面客户端和手机 App 的本机模式用它，Stop 时一起关闭。
func (e *Engine) ServeLocal() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	srv, err := e.prepareServer(ln)
	if err != nil {
		return "", err
	}
	go e.serve(srv, ln)
	return "http://" + ln.Addr().String(), nil
}

// prepareServer 和 Stop 通过 mu 串行登记监听器，并把服务循环计入关闭等待。
func (e *Engine) prepareServer(ln net.Listener) (*http.Server, error) {
	srv := &http.Server{Handler: e.Handler(), ReadHeaderTimeout: 10 * time.Second}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ctx.Err(); err != nil {
		ln.Close()
		return nil, err
	}
	if e.srv != nil {
		ln.Close()
		return nil, errors.New("接口已经启动")
	}
	e.srv, e.listener = srv, ln
	e.workers.Add(1)
	return srv, nil
}

func (e *Engine) serve(srv *http.Server, ln net.Listener) error {
	defer e.workers.Done()
	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
