package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type LoggingConfig struct {
	File       string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
}

type IgnoreConfig struct {
	Files       []string
	Extensions  []string
	Directories []string
}

type MonitorConfig struct {
	Directories []string
	Ignore      IgnoreConfig
	Events      []string
	DebounceMs  int
	eventMask   fsnotify.Op
}

type WebhookConfig struct {
	Enabled  bool
	Provider string
	Sendkey  string
}

type EmailConfig struct {
	Enabled  bool
	SmtpHost string
	SmtpPort int
	Username string
	Password string
	From     string
	To       []string
}

type Config struct {
	Logging LoggingConfig
	Monitor MonitorConfig
	Webhook WebhookConfig
	Email   EmailConfig
}

func loadConfig(path string) (*Config, error) {
	v := viper.New()
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		if exe, err := os.Executable(); err == nil {
			v.AddConfigPath(filepath.Dir(exe))
		}
	}

	// 敏感信息支持从环境变量注入，优先级高于配置文件
	_ = v.BindEnv("email.password", "EMAIL_PASSWORD")
	_ = v.BindEnv("webhook.sendkey", "WEBHOOK_SENDKEY")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Monitor.Directories) == 0 {
		return errors.New("未配置监控目录 (monitor.directories)")
	}
	if c.Monitor.DebounceMs < 0 {
		return errors.New("monitor.debounceMs 不能为负数")
	}
	if c.Monitor.DebounceMs == 0 {
		c.Monitor.DebounceMs = 500
	}

	mask, err := parseEvents(c.Monitor.Events)
	if err != nil {
		return err
	}
	c.Monitor.eventMask = mask

	if c.Email.Enabled && c.Webhook.Enabled {
		return errors.New("email 与 webhook 不能同时启用，请只保留一种通知方式")
	}
	if c.Webhook.Enabled {
		if !strings.EqualFold(c.Webhook.Provider, "serverchan") {
			return fmt.Errorf("不支持的 webhook.provider: %q（当前仅支持 serverchan）", c.Webhook.Provider)
		}
		if c.Webhook.Sendkey == "" {
			return errors.New("webhook 已启用但缺少 sendkey")
		}
	}
	if c.Email.Enabled {
		e := c.Email
		switch {
		case e.SmtpHost == "":
			return errors.New("email 已启用但缺少 smtpHost")
		case e.SmtpPort <= 0:
			return errors.New("email 已启用但 smtpPort 无效")
		case e.Username == "" || e.Password == "":
			return errors.New("email 已启用但缺少 username/password")
		case e.From == "":
			return errors.New("email 已启用但缺少 from")
		case len(e.To) == 0:
			return errors.New("email 已启用但缺少收件人 to")
		}
	}

	if c.Logging.File == "" {
		c.Logging.File = "logs/monitor.log"
	}
	return nil
}

var eventTypes = map[string]fsnotify.Op{
	"create": fsnotify.Create,
	"write":  fsnotify.Write,
	"remove": fsnotify.Remove,
	"rename": fsnotify.Rename,
	"chmod":  fsnotify.Chmod,
}

// parseEvents 将配置中的事件名转换为 fsnotify 操作位掩码；未配置时默认监听全部事件。
func parseEvents(names []string) (fsnotify.Op, error) {
	if len(names) == 0 {
		var all fsnotify.Op
		for _, op := range eventTypes {
			all |= op
		}
		return all, nil
	}

	var mask fsnotify.Op
	for _, name := range names {
		op, ok := eventTypes[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			return 0, fmt.Errorf("未知事件类型 %q（支持: create, write, remove, rename, chmod）", name)
		}
		mask |= op
	}
	return mask, nil
}
