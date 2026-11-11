// Command qjrxy 启动 Qjrxy 服务：HTTP API + 内嵌前端 + 自动查寝调度器。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"net"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"qjrxy/internal/buildinfo"
	"qjrxy/internal/config"
	"qjrxy/internal/scheduler"
	"qjrxy/internal/secure"
	"qjrxy/internal/server"
	"qjrxy/internal/service"
	"qjrxy/internal/store"
	"qjrxy/web"
)

func main() {
	var (
		addr    = flag.String("addr", "", "监听地址，默认读取 QINGJIN_ADDR 或 :8090")
		dataDir = flag.String("data", "", "数据目录，默认读取 QINGJIN_DATA_DIR 或 ./data")
		version = flag.Bool("version", false, "打印版本号后退出")
	)
	flag.Parse()

	if *version {
		fmt.Println("qjrxy", buildinfo.Version)
		return
	}

	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[Qjrxy] ")

	// 固定时区为北京时间（东八区）。
	// 签到窗口、日志「今天」的边界、统计口径都依赖本地时区；
	// 部署在 UTC 主机上若不固定，22:00 的查寝会被算成 06:00，全部错位。
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		time.Local = loc
	} else {
		// 容器里常缺 tzdata，退回到固定偏移（中国无夏令时，等价）
		time.Local = time.FixedZone("CST", 8*3600)
	}

	if *addr != "" {
		os.Setenv("QINGJIN_ADDR", *addr)
	}
	if *dataDir != "" {
		os.Setenv("QINGJIN_DATA_DIR", *dataDir)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败：%v", err)
	}

	db, err := store.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("打开数据库失败：%v", err)
	}
	defer db.Close()

	svc := service.New(cfg, db)
	if err := bootstrapAdmin(context.Background(), svc, cfg); err != nil {
		log.Printf("初始化管理员失败：%v", err)
	}

	static, err := web.Handler()
	if err != nil {
		log.Fatalf("加载前端资源失败：%v", err)
	}
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.New(svc).Handler(static),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	sched := scheduler.New(svc)
	sched.Start()

	go func() {
		if lan := server.LANBaseURLs(cfg.Addr); len(lan) > 0 {
			log.Printf("服务已启动：%s（局域网）或 %s", lan[0], localURL(cfg.Addr))
		} else {
			log.Printf("服务已启动：%s", localURL(cfg.Addr))
		}
		for _, u := range server.LANBaseURLs(cfg.Addr) {
			log.Printf("局域网访问：%s", u)
		}
		if cfg.SingleUser {
			log.Printf("提示：单用户模式无账号鉴权，请仅在可信网络（家庭 Wi-Fi）下使用；" +
				"只允许本机访问请设 QINGJIN_ADDR=127.0.0.1:8090")
		}
		log.Printf("数据目录：%s", cfg.DataDir)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务异常退出：%v", err)
		}
	}()

	// 单用户模式（本地 exe）：启动后自动打开浏览器，双击即用。
	// 优先用局域网地址——这样二维码直接可用，不用再手动切换第二遍；
	// 没有局域网（无网卡/只绑回环）时回退 localhost。
	// 服务器多人部署请设 QINGJIN_MULTI_USER=true；QINGJIN_NO_OPEN=1 可关闭自动打开。
	if cfg.SingleUser && os.Getenv("QINGJIN_NO_OPEN") != "1" {
		openURL := localURL(cfg.Addr)
		if lan := server.LANBaseURLs(cfg.Addr); len(lan) > 0 {
			openURL = lan[0]
		}
		go func() {
			time.Sleep(400 * time.Millisecond) // 等监听就绪
			if err := openBrowser(openURL); err != nil {
				log.Printf("自动打开浏览器失败（可手动访问 %s）：%v", openURL, err)
			}
		}()
	}

	// 优雅退出
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("正在关闭服务……")
	sched.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("关闭 HTTP 服务出错：%v", err)
	}
	_ = db.PurgeExpiredSessions(context.Background())
	log.Println("已退出")
}

// bootstrapAdmin 在配置了初始管理员且库中无用户时创建管理员。
func bootstrapAdmin(ctx context.Context, svc *service.Service, cfg *config.Config) error {
	if cfg.AdminUser == "" {
		return nil
	}
	count, err := svc.DB().CountUsers(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	password := cfg.AdminPass
	if password == "" {
		password = secure.RandomID(12)
		log.Printf("未配置 QINGJIN_ADMIN_PASS，已生成初始密码：%s", password)
	}
	user, err := svc.Register(ctx, cfg.AdminUser, password, "管理员")
	if err != nil {
		return err
	}
	log.Printf("已创建初始管理员：%s（id=%d）", user.Username, user.ID)
	return nil
}

// localURL 计算本机可访问的地址：未绑定具体 IP 时用 127.0.0.1。
func localURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host, port = "localhost", "8090"
	}
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

// openBrowser 用系统默认浏览器打开页面（Windows/macOS/Linux）。
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

