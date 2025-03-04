package main

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/viper"
)

type Config struct {
	Monitor struct {
		Directories []string
		Ignore      struct {
			Files       []string
			Directories []string
		}
		Events []string
	}
	Webhook struct {
		Provider string
		Sendkey  string
		Template string
	}
}

func main() {
	// 加载配置
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 转换路径格式为当前系统格式
	for i, dir := range cfg.Monitor.Directories {
		cfg.Monitor.Directories[i] = convertPath(dir)
	}

	fmt.Printf("Starting file monitor with config: %+v\n", cfg)

	// 初始化文件监控器
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	// 添加监控目录
	for _, dir := range cfg.Monitor.Directories {
		if err := watcher.Add(dir); err != nil {
			log.Printf("Failed to watch directory %s: %v", dir, err)
		}
	}

	// 处理文件事件
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// 转换路径为统一格式
			event.Name = convertPath(event.Name)
			if shouldIgnore(event.Name, cfg) {
				continue
			}
			log.Printf("File event: %s %s", event.Op, event.Name)
			if cfg.Webhook.Provider == "serverchan" {
				if err := sendServerChanNotification(cfg, event); err != nil {
					log.Printf("Failed to send notification: %v", err)
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

// 转换路径为当前系统格式
func convertPath(path string) string {
	if runtime.GOOS == "windows" {
		// 将Linux路径转换为Windows路径
		if strings.HasPrefix(path, "/") {
			path = "C:" + strings.ReplaceAll(path, "/", "\\")
		}
		return filepath.FromSlash(path)
	}
	// 将Windows路径转换为Linux路径
	if strings.HasPrefix(path, "C:\\") {
		path = strings.ReplaceAll(path, "C:\\", "/")
	}
	return filepath.ToSlash(path)
}

func shouldIgnore(path string, cfg *Config) bool {
	// 检查是否在忽略列表中
	for _, pattern := range cfg.Monitor.Ignore.Files {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	for _, pattern := range cfg.Monitor.Ignore.Directories {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
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
