// 大模型推理硬件自动化测试平台 · 单二进制服务端
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"llmhwtest/internal/auth"
	"llmhwtest/internal/httpd"
	"llmhwtest/internal/scheduler"
	"llmhwtest/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "监听地址(默认仅内网/本机)")
	dataDir := flag.String("data", "./data", "数据目录(SQLite/密钥/备份)")
	resetPassword := flag.Bool("reset-password", false, "重置登录密码为随机初始密码并退出")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}

	st, err := store.Open(filepath.Join(*dataDir, "platform.db"))
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()

	// SSH 凭据加密密钥(随机生成,0600 存数据目录)
	secretKey, err := store.LoadOrCreateSecretKey(*dataDir)
	if err != nil {
		log.Fatalf("加载密钥失败: %v", err)
	}
	st.SetSecretKey(secretKey)

	au := auth.New(st)

	if *resetPassword {
		pw, err := au.ResetPassword()
		if err != nil {
			log.Fatalf("重置密码失败: %v", err)
		}
		fmt.Printf("密码已重置,新的初始密码: %s\n首次登录后请立即修改。\n", pw)
		return
	}

	// 首次启动:生成随机初始密码打印到控制台
	if au.IsFirstRun() {
		pw, err := au.ResetPassword()
		if err != nil {
			log.Fatalf("初始化密码失败: %v", err)
		}
		fmt.Println("==================================================")
		fmt.Println("  首次启动,初始登录密码(请妥善保存):")
		fmt.Printf("    %s\n", pw)
		fmt.Println("  首次登录后系统强制要求修改密码。")
		fmt.Println("  丢失密码可用 --reset-password 重启重置。")
		fmt.Println("==================================================")
	}

	// V3.0.2:先绑定端口,再执行中断标记——端口冲突的重复实例直接退出,
	// 不会误伤正在运行的任务(169 双管理冲突事故根治:systemd 与 nohup 同库抢端口时,
	// 崩循环实例每次启动都会把执行中任务标成「中断」)
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		st.Close()
		log.Fatalf("端口 %s 绑定失败: %v(未执行中断标记)", *addr, err)
	}

	// 平台重启:执行中/排队中任务标记"中断"(F-5.4)
	interrupted := st.MarkInterruptedTasks()
	if interrupted > 0 {
		log.Printf("平台重启,%d 个未完成任务标记为「中断」", interrupted)
	}

	sched := scheduler.New(st, *dataDir)
	sched.Start()

	srv := httpd.New(st, au, sched, *dataDir)

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("服务已启动: http://%s  数据目录: %s", *addr, *dataDir)
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务失败: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("正在退出...")
	sched.Shutdown()
	_ = httpSrv.Close()
}
