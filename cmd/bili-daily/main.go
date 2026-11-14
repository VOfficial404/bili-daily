// bili-daily 的服务端。
//
//	bili-daily            常驻运行：每天按计划执行任务，提供网页面板和给客户端的接口
//	bili-daily login      在终端里扫码添加账号
//	bili-daily run        立即给所有账号跑一轮（配合 cron 使用，或者用来测试）
//	bili-daily token      显示客户端连接时要填的令牌
//	bili-daily version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"rsc.io/qr"

	"github.com/VOfficial404/bili-daily/api"
	"github.com/VOfficial404/bili-daily/engine"
	"github.com/VOfficial404/bili-daily/web"
)

// version 在发布时由 -ldflags "-X main.version=…" 写入
var version = "dev"

const usage = `bili-daily：B 站每日任务

用法：
  bili-daily [serve]         常驻运行：每天按计划执行任务，提供网页面板和给客户端的接口
  bili-daily login           在终端里扫码添加账号
  bili-daily run [选项]      立即跑一轮然后退出
      -mid UID               只跑这个账号（默认全部）
      -tasks a,b             只跑这些任务（默认每日计划里的全部）
  bili-daily token           显示客户端连接时要填的令牌
  bili-daily version

环境变量：
  BILI_DAILY_DATA    数据目录，默认是程序所在目录下的 data
  BILI_DAILY_ADDR    网页面板和接口的监听地址，默认 127.0.0.1:8686（只有本机能连）
  BILI_DAILY_TOKEN   接口令牌，默认自动生成并保存在数据目录里
`

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "login":
		err = login()
	case "run":
		err = runOnce(args)
	case "token":
		var e *engine.Engine
		if e, err = open(); err == nil {
			fmt.Println(e.Token())
		}
	case "version":
		fmt.Println(version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "出错了：", err)
		os.Exit(1)
	}
}

func dataDir() string {
	if d := os.Getenv("BILI_DAILY_DATA"); d != "" {
		return d
	}
	exe, err := os.Executable()
	if err != nil {
		return "data"
	}
	return filepath.Join(filepath.Dir(exe), "data")
}

func open() (*engine.Engine, error) {
	return engine.New(engine.Config{
		DataDir:   dataDir(),
		Version:   version,
		Token:     os.Getenv("BILI_DAILY_TOKEN"),
		LogOutput: os.Stderr,
		Panel:     web.Handler(),
	})
}

// panelURL 是启动日志里提示的面板地址；监听所有地址时（例如容器里）换成「服务器地址」让人自己填
func panelURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "服务器地址"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func interruptible() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func serve() error {
	e, err := open()
	if err != nil {
		return err
	}
	addr := os.Getenv("BILI_DAILY_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8686"
	}
	e.Start()
	e.Infof("网页面板和接口监听 %s：浏览器打开 %s 用下面的令牌登录，客户端连接时也填这个地址和令牌", addr, panelURL(addr))
	e.Infof("令牌：%s", e.Token())

	ctx, cancel := interruptible()
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- e.ListenAndServe(addr) }()
	select {
	case err := <-errc:
		e.Stop()
		return err
	case <-ctx.Done():
		e.Infof("正在停止")
		e.Stop()
		return nil
	}
}

func login() error {
	e, err := open()
	if err != nil {
		return err
	}
	defer e.Stop()
	ctx, cancel := interruptible()
	defer cancel()

	q, err := e.StartQR(ctx)
	if err != nil {
		return err
	}
	if err := printQR(q.URL); err != nil {
		return err
	}
	fmt.Println("用 B 站 App 扫上面的二维码，然后在手机上确认。")
	fmt.Println("二维码显示不全时，把终端窗口拉大，或者在客户端里添加账号。")
	last := ""
	for {
		select {
		case <-ctx.Done():
			return errors.New("已取消")
		case <-time.After(2 * time.Second):
		}
		st, err := e.PollQR(ctx, q.ID)
		if err != nil {
			return err
		}
		switch st.State {
		case "scanned":
			if last != st.State {
				fmt.Println("已扫码，请在手机上确认。")
			}
		case "expired":
			return errors.New("二维码过期了，请重新运行 bili-daily login")
		case "success":
			fmt.Printf("登录成功：UID %s。正在运行的 bili-daily 会在一分钟内接上这个账号。\n", st.Mid)
			return nil
		}
		last = st.State
	}
}

// printQR 用半高的方块字符在终端里画二维码
func printQR(content string) error {
	code, err := qr.Encode(content, qr.L)
	if err != nil {
		return err
	}
	const quiet = 2
	at := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < code.Size && y < code.Size && code.Black(x, y)
	}
	var b strings.Builder
	for y := -quiet; y < code.Size+quiet; y += 2 {
		for x := -quiet; x < code.Size+quiet; x++ {
			// 终端一般是深色底：亮的方块当作二维码的白色
			switch top, bottom := at(x, y), at(x, y+1); {
			case top && bottom:
				b.WriteByte(' ')
			case top:
				b.WriteString("▄")
			case bottom:
				b.WriteString("▀")
			default:
				b.WriteString("█")
			}
		}
		b.WriteByte('\n')
	}
	fmt.Print(b.String())
	return nil
}

func runOnce(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	mid := fs.String("mid", "", "只跑这个账号")
	tasks := fs.String("tasks", "", "只跑这些任务，用逗号分隔")
	fs.Parse(args)

	e, err := open()
	if err != nil {
		return err
	}
	defer e.Stop()
	ctx, cancel := interruptible()
	defer cancel()

	mids := e.Accounts()
	if *mid != "" {
		mids = []string{*mid}
	}
	if len(mids) == 0 {
		return errors.New("还没有账号，先运行 bili-daily login")
	}
	var ids []string
	for _, id := range strings.Split(*tasks, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	failed := 0
	for _, m := range mids {
		run, err := e.RunNow(ctx, m, ids)
		if err != nil {
			return fmt.Errorf("UID %s：%w", m, err)
		}
		if run.Status != api.StatusOK {
			failed++
		}
		if ctx.Err() != nil {
			break
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d 个账号有任务没完成，详情见上面的日志", failed)
	}
	return nil
}
