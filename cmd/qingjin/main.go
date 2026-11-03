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
	"os/signal"
	"syscall"
	"time"

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
		fmt.Println("qjrxy", appVersion)
		return
	}

	log.SetFlags(log.LstdFlags)
	log.SetPrefix("[XIAOMAI] ")

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
		log.Printf("服务已启动：http://localhost%s", displayAddr(cfg.Addr))
		log.Printf("数据目录：%s", cfg.DataDir)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务异常退出：%v", err)
		}
	}()

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

func displayAddr(addr string) string {
	if addr == "" {
		return ":8090"
	}
	if addr[0] == ':' {
		return addr
	}
	return addr
}

const appVersion = "1.0.0"
