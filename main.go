package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"gopkg.in/natefinch/lumberjack.v2"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径（默认依次在当前目录、可执行文件所在目录查找 config.yaml）")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Logging.File), 0755); err != nil {
		log.Fatalf("无法创建日志目录: %v", err)
	}

	logFile := &lumberjack.Logger{
		Filename:   cfg.Logging.File,
		MaxSize:    cfg.Logging.MaxSize,
		MaxBackups: cfg.Logging.MaxBackups,
		MaxAge:     cfg.Logging.MaxAge,
		Compress:   cfg.Logging.Compress,
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	log.Printf("启动文件监控服务，配置详情：")
	log.Printf("监控目录(递归): %v", cfg.Monitor.Directories)
	log.Printf("忽略规则: 文件模式=%v 扩展名=%v 目录=%v",
		cfg.Monitor.Ignore.Files, cfg.Monitor.Ignore.Extensions, cfg.Monitor.Ignore.Directories)
	log.Printf("监控事件类型: %v", cfg.Monitor.Events)
	log.Printf("事件防抖窗口: %dms", cfg.Monitor.DebounceMs)
	if cfg.Email.Enabled {
		log.Printf("邮件通知已启用，收件人: %v", cfg.Email.To)
	}
	if cfg.Webhook.Enabled {
		log.Printf("Webhook通知已启用，服务商: %s", cfg.Webhook.Provider)
	}

	monitor, err := NewMonitor(cfg)
	if err != nil {
		log.Fatalf("初始化监控器失败: %v", err)
	}
	if monitor.watchAll() == 0 {
		monitor.Close()
		log.Fatalf("没有任何监控目录添加成功，请检查 monitor.directories 配置")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := monitor.Run(ctx); err != nil {
		log.Printf("事件循环异常退出: %v", err)
	}
	monitor.Close()
	log.Printf("文件监控服务已退出")
}
