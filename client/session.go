// Package client 提供桌面和手机共用的连接、设置与状态逻辑，不依赖界面库。
package client

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/engine"
)

// Session 管理一次连接。本机启动和关闭都是同步的；零值可以直接使用。
// 界面应为每次连接保留自己的 Session，后台结果回到界面线程时再读 Snapshot。
type Session struct {
	mu          sync.Mutex
	current     *connection
	cl          *api.Client
	local       *engine.Engine
	state       api.State
	settingsRev uint64
	saves       chan struct{}
}

// connection 的 ready 关闭后，启动过程不再访问本机引擎，关闭方才可以接手。
type connection struct {
	ctx      context.Context
	cancel   context.CancelFunc
	ready    chan struct{}
	engine   *engine.Engine
	stopOnce sync.Once
}

// LocalOptions 指定本机引擎和可选的代理恢复值。Proxy 为 nil 时保留数据目录里的设置。
type LocalOptions struct {
	Config    engine.Config
	Proxy     *string
	NewEngine func(engine.Config) (*engine.Engine, error)
}

// NewSession 接上已有客户端和状态，也供界面测试使用。
func NewSession(cl *api.Client, state api.State) *Session {
	s := &Session{}
	g, _ := s.begin()
	s.cl, s.state = cl, cloneState(state)
	close(g.ready)
	return s
}

func (s *Session) begin() (*connection, *connection) {
	ctx, cancel := context.WithCancel(context.Background())
	g := &connection{ctx: ctx, cancel: cancel, ready: make(chan struct{})}
	s.mu.Lock()
	old := s.current
	s.current = g
	s.cl, s.local, s.state = nil, nil, api.State{}
	s.settingsRev = 0
	if old != nil {
		old.cancel()
	}
	s.mu.Unlock()
	return g, old
}

func (g *connection) stop() {
	if g == nil {
		return
	}
	g.cancel()
	<-g.ready
	g.stopEngine()
}

func (g *connection) stopEngine() {
	g.stopOnce.Do(func() {
		if g.engine != nil {
			g.engine.Stop()
		}
	})
}

// operationContext 同时响应调用方取消和连接被替换、停止。
func operationContext(ctx context.Context, g *connection) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(g.ctx, cancel)
	if g.ctx.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}

func (s *Session) attach(ctx context.Context, g *connection, cl *api.Client, st api.State) (api.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return api.State{}, err
	}
	if s.current != g || g.ctx.Err() != nil {
		return api.State{}, context.Canceled
	}
	s.cl, s.local, s.state = cl, g.engine, st
	return cloneState(s.state), nil
}

// Connect 整理地址，确认服务端身份并验证令牌，成功后才发布连接。
func (s *Session) Connect(ctx context.Context, address, token string) (api.State, error) {
	base, err := api.NormalizeBase(address)
	if err != nil {
		return api.State{}, err
	}
	g, old := s.begin()
	defer close(g.ready)
	old.stop()
	ctx, cancel := operationContext(ctx, g)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return api.State{}, err
	}
	cl := api.NewClient(base, token)
	if _, err := cl.Ping(ctx); err != nil {
		return api.State{}, requestError(ctx, err)
	}
	st, err := cl.State(ctx)
	if err != nil {
		return api.State{}, requestError(ctx, err)
	}
	return s.attach(ctx, g, cl, st)
}

// StartLocal 启动本机引擎和接口；任何一步失败都会先关闭已经创建的引擎。
func (s *Session) StartLocal(ctx context.Context, options LocalOptions) (st api.State, err error) {
	g, old := s.begin()
	defer close(g.ready)
	old.stop()
	ctx, cancel := operationContext(ctx, g)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return api.State{}, err
	}
	factory := options.NewEngine
	if factory == nil {
		factory = engine.New
	}
	options.Config.Local = true
	g.engine, err = factory(options.Config)
	defer func() {
		if err != nil {
			g.stopEngine()
		}
	}()
	if err != nil {
		return api.State{}, err
	}
	if err = ctx.Err(); err != nil {
		return api.State{}, err
	}
	if options.Proxy != nil {
		settings := g.engine.Settings()
		settings.Proxy = *options.Proxy
		if _, err = g.engine.UpdateSettings(settings); err != nil {
			return api.State{}, err
		}
	}
	g.engine.Start()
	address, err := g.engine.ServeLocal()
	if err != nil {
		return api.State{}, err
	}
	cl := api.NewClient(address, g.engine.Token())
	st, err = cl.State(ctx)
	if err != nil {
		return api.State{}, requestError(ctx, err)
	}
	return s.attach(ctx, g, cl, st)
}

// Stop 立即使旧请求失效，并等本机启动、引擎关闭和此前的关闭调用完成。
func (s *Session) Stop() {
	s.mu.Lock()
	g := s.current
	if g != nil {
		g.cancel()
	}
	s.cl, s.local, s.state = nil, nil, api.State{}
	s.mu.Unlock()
	g.stop()
	s.mu.Lock()
	if s.current == g {
		s.current = nil
	}
	s.mu.Unlock()
}

func (s *Session) Client() *api.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cl
}

func (s *Session) LocalEngine() *engine.Engine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.local
}

// Snapshot 返回独立副本，调用方改动其中的列表或指针不会改到会话。
func (s *Session) Snapshot() api.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneState(s.state)
}

func (s *Session) operation() (*connection, *api.Client, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil || s.cl == nil || s.current.ctx.Err() != nil {
		return nil, nil, 0, errors.New("尚未连接服务端")
	}
	return s.current, s.cl, s.settingsRev, nil
}

// Refresh 更新状态。请求发出后保存过设置时，保留已经保存的新设置。
func (s *Session) Refresh(ctx context.Context) (api.State, error) {
	g, cl, rev, err := s.operation()
	if err != nil {
		return api.State{}, err
	}
	ctx, cancel := operationContext(ctx, g)
	defer cancel()
	st, err := cl.State(ctx)
	if err != nil {
		return api.State{}, requestError(ctx, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return api.State{}, err
	}
	if s.current != g || g.ctx.Err() != nil {
		return api.State{}, context.Canceled
	}
	if rev != s.settingsRev {
		st.Settings = s.state.Settings
	}
	s.state = st
	return cloneState(s.state), nil
}

// SaveSettings 串行保存表单差异，每次先取服务端最新设置，保留其他客户端的改动。
func (s *Session) SaveSettings(ctx context.Context, base, edited api.Settings) (api.Settings, error) {
	g, cl, _, err := s.operation()
	if err != nil {
		return api.Settings{}, err
	}
	ctx, cancel := operationContext(ctx, g)
	defer cancel()
	s.mu.Lock()
	if s.saves == nil {
		s.saves = make(chan struct{}, 1)
	}
	gate := s.saves
	s.mu.Unlock()
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return api.Settings{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return api.Settings{}, err
	}
	latest, err := cl.State(ctx)
	if err != nil {
		return api.Settings{}, requestError(ctx, err)
	}
	saved, err := cl.SaveSettings(ctx, api.MergeSettings(base, edited, latest.Settings))
	if err != nil {
		return api.Settings{}, requestError(ctx, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return api.Settings{}, err
	}
	if s.current != g || g.ctx.Err() != nil {
		return api.Settings{}, context.Canceled
	}
	s.settingsRev++
	s.state.Settings = saved
	return CloneSettings(saved), nil
}

// NeedsChargeConfirm 在启用 B 币券充电或更换已启用的充电对象时要求确认。
func NeedsChargeConfirm(base, edited api.Settings) bool {
	b, e := base.Tasks.Charge, edited.Tasks.Charge
	return e.Enabled && (!b.Enabled || b.UpID != e.UpID)
}

// CloneSettings 复制设置中的列表，避免表单和快照共享底层数组。
func CloneSettings(s api.Settings) api.Settings {
	s.Tasks.Daily.UpIDs = slices.Clone(s.Tasks.Daily.UpIDs)
	s.Tasks.LiveLottery.Include = slices.Clone(s.Tasks.LiveLottery.Include)
	s.Tasks.LiveLottery.Exclude = slices.Clone(s.Tasks.LiveLottery.Exclude)
	s.Tasks.Unfollow.Keep = slices.Clone(s.Tasks.Unfollow.Keep)
	return s
}

func clonePointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneState(st api.State) api.State {
	st.Settings = CloneSettings(st.Settings)
	st.Tasks = slices.Clone(st.Tasks)
	st.Egress = clonePointer(st.Egress)
	st.Accounts = slices.Clone(st.Accounts)
	for i := range st.Accounts {
		a := &st.Accounts[i]
		a.DisabledTasks = slices.Clone(a.DisabledTasks)
		a.Today = slices.Clone(a.Today)
		a.VIPDue = clonePointer(a.VIPDue)
		a.CookieExpires = clonePointer(a.CookieExpires)
		a.UpdatedAt = clonePointer(a.UpdatedAt)
		a.Running = clonePointer(a.Running)
		a.NextRun = clonePointer(a.NextRun)
		a.LastRun = clonePointer(a.LastRun)
	}
	return st
}

// api.Client 的网络错误面向用户显示；会话仍保留可用 errors.Is 判断的取消原因。
func requestError(ctx context.Context, err error) error {
	if canceled := ctx.Err(); canceled != nil {
		return canceled
	}
	return err
}
