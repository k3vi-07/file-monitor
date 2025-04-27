package main

import (
	"fmt"
	"io"
	"log"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Config struct {
	Logging struct {
		File       string
		MaxSize    int
		MaxBackups int
		MaxAge     int
		Compress   bool
	}
	Monitor struct {
		Directories []string
		Ignore      struct {
			Files       []string
			Extensions  []string
			Directories []string
		}
		Events []string
	}
	Webhook struct {
		Enabled  bool
		Provider string
		Sendkey  string
		Template string
	}
	Email struct {
		Enabled  bool
		SmtpHost string
		SmtpPort int
		Username string
		Password string
		From     string
		To       []string
	}
}

func main() {
	// 加载配置
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 确保日志目录存在
	if err := os.MkdirAll(filepath.Dir(cfg.Logging.File), 0755); err != nil {
		log.Fatalf("无法创建日志目录: %v", err)
	}

	// 初始化文件日志
	logFile := &lumberjack.Logger{
		Filename:   cfg.Logging.File,
		MaxSize:    cfg.Logging.MaxSize,
		MaxBackups: cfg.Logging.MaxBackups,
		MaxAge:     cfg.Logging.MaxAge,
		Compress:   cfg.Logging.Compress,
	}
	defer logFile.Close()

	// 设置多输出日志
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	log.Printf("启动文件监控服务，配置详情：")
	log.Printf("监控目录: %v", cfg.Monitor.Directories)
	log.Printf("忽略规则:")
	log.Printf("  - 文件模式: %v", cfg.Monitor.Ignore.Files)
	log.Printf("  - 扩展名: %v", cfg.Monitor.Ignore.Extensions)
	log.Printf("  - 目录模式: %v", cfg.Monitor.Ignore.Directories)
	log.Printf("监控事件类型: %v", cfg.Monitor.Events)

	if cfg.Email.Enabled {
		log.Printf("邮件通知已启用，收件人: %v", cfg.Email.To)
	}
	if cfg.Webhook.Enabled {
		log.Printf("Webhook通知已启用，服务商: %s", cfg.Webhook.Provider)
	}

	// 初始化文件监控器
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	// 添加监控目录
	for _, dir := range cfg.Monitor.Directories {
		if err := watcher.Add(dir); err != nil {
			log.Printf("监控目录添加失败: %s 错误: %v", dir, err)
		} else {
			log.Printf("成功添加监控目录: %s", dir)
		}
	}

	// 配置检查（webhook和email互斥）
	if cfg.Email.Enabled && cfg.Webhook.Enabled {
		log.Fatal("配置冲突：邮件和Webhook通知不能同时启用，请修改config.yaml")
	}

	// 处理文件事件
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			ignored, reason := shouldIgnore(event.Name, cfg)
			if ignored {
				log.Printf("忽略文件事件: %s (原因: %s)", event.Name, reason)
				continue
			}
			log.Printf("处理文件事件: 操作=%s 文件=%s", event.Op, event.Name)
			if cfg.Webhook.Enabled && cfg.Webhook.Provider == "serverchan" {
				if err := sendServerChanNotification(cfg, event); err != nil {
					log.Printf("Webhook通知失败: %v", err)
				} else {
					log.Printf("Webhook通知成功: %s", event.Name)
				}
			} else if cfg.Email.Enabled {
				if err := sendEmailNotification(cfg, event); err != nil {
					log.Printf("邮件通知失败: %v", err)
				} else {
					log.Printf("邮件通知成功: %s", event.Name)
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func shouldIgnore(path string, cfg *Config) (bool, string) {
	// 检查文件扩展名
	ext := filepath.Ext(path)
	for _, ignoreExt := range cfg.Monitor.Ignore.Extensions {
		if ext == ignoreExt {
			return true, fmt.Sprintf("扩展名匹配: %s", ignoreExt)
		}
	}

	// 检查文件名模式
	for _, pattern := range cfg.Monitor.Ignore.Files {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true, fmt.Sprintf("文件名匹配: %s", pattern)
		}
	}

	// 检查目录模式
	for _, pattern := range cfg.Monitor.Ignore.Directories {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true, fmt.Sprintf("目录匹配: %s", pattern)
		}
	}

	return false, ""
}

func loadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	var cfg Config
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 验证配置
	if len(cfg.Monitor.Directories) == 0 {
		return nil, fmt.Errorf("no directories to monitor")
	}

	return &cfg, nil
}

func sendEmailNotification(cfg *Config, event fsnotify.Event) error {
	auth := smtp.PlainAuth("", cfg.Email.Username, cfg.Email.Password, cfg.Email.SmtpHost)

	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: 文件变动通知\r\n"+
		"\r\n"+
		"文件路径: %s\n操作类型: %s\n时间: %s",
		strings.Join(cfg.Email.To, ","),
		event.Name,
		event.Op.String(),
		time.Now().Format("2006-01-02 15:04:05")))

	return smtp.SendMail(
		fmt.Sprintf("%s:%d", cfg.Email.SmtpHost, cfg.Email.SmtpPort),
		auth,
		cfg.Email.From,
		cfg.Email.To,
		msg,
	)
}

func sendServerChanNotification(cfg *Config, event fsnotify.Event) error {
	client := resty.New()

	resp, err := client.R().
		SetFormData(map[string]string{
			"title": "文件变动通知",
			"desp": fmt.Sprintf("文件路径: %s\n操作类型: %s\n时间: %s",
				event.Name,
				event.Op.String(),
				time.Now().Format("2006-01-02 15:04:05"),
			),
		}).
		Post(fmt.Sprintf("https://sctapi.ftqq.com/%s.send", cfg.Webhook.Sendkey))

	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return nil
}
