package main

import (
	"testing"

	"github.com/fsnotify/fsnotify"
)

func newTestConfig() *Config {
	cfg := &Config{}
	cfg.Monitor.Ignore.Extensions = []string{".bak", ".jpg"}
	cfg.Monitor.Ignore.Files = []string{"*.tmp", ".DS_Store"}
	cfg.Monitor.Ignore.Directories = []string{"/data/logs", "temp", "node_modules"}
	return cfg
}

func TestShouldIgnore(t *testing.T) {
	cfg := newTestConfig()
	cases := []struct {
		path string
		want bool
	}{
		// 扩展名（不区分大小写）
		{"/data/src/a.bak", true},
		{"/data/src/photo.JPG", true},
		// 文件名模式
		{"/data/src/a.tmp", true},
		{"/data/src/.DS_Store", true},
		// 目录：绝对路径前缀匹配（目录自身及其全部子内容）
		{"/data/logs", true},
		{"/data/logs/monitor.log", true},
		{"/data/logs/sub/x.txt", true},
		// 目录：路径段匹配
		{"/var/www/temp/cache/x.txt", true},
		{"/var/www/site/node_modules/pkg/x.js", true},
		// 不应误伤
		{"/data/src/templates/x.txt", false},
		{"/data/logserver/x.txt", false},
		{"/data/src/main.go", false},
	}
	for _, tc := range cases {
		got, reason := shouldIgnore(tc.path, cfg)
		if got != tc.want {
			t.Errorf("shouldIgnore(%q) = %v（原因: %s），期望 %v", tc.path, got, reason, tc.want)
		}
	}
}

func TestParseEvents(t *testing.T) {
	mask, err := parseEvents([]string{"create", "WRITE", " remove "})
	if err != nil {
		t.Fatalf("parseEvents 返回错误: %v", err)
	}
	want := fsnotify.Create | fsnotify.Write | fsnotify.Remove
	if mask != want {
		t.Errorf("mask = %v，期望 %v", mask, want)
	}

	// 过滤语义：事件携带任一被监听操作即通过
	if !(fsnotify.Event{Op: fsnotify.Remove}).Op.Has(mask) {
		t.Error("remove 事件应通过过滤")
	}
	if (fsnotify.Event{Op: fsnotify.Rename}).Op.Has(mask) {
		t.Error("rename 事件应被过滤")
	}

	if _, err := parseEvents([]string{"create", "explode"}); err == nil {
		t.Error("未知事件类型应返回错误")
	}

	all, err := parseEvents(nil)
	if err != nil {
		t.Fatalf("空配置不应返回错误: %v", err)
	}
	wantAll := fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename | fsnotify.Chmod
	if all != wantAll {
		t.Errorf("空配置应默认监听全部事件，got %v", all)
	}
}

func TestConfigValidate(t *testing.T) {
	cfg := &Config{}
	cfg.Monitor.Directories = []string{"/data"}
	if err := cfg.validate(); err != nil {
		t.Fatalf("最小配置校验失败: %v", err)
	}
	if cfg.Logging.File == "" {
		t.Error("日志文件路径应填充默认值")
	}
	if cfg.Monitor.DebounceMs != 500 {
		t.Errorf("防抖窗口默认值 = %d，期望 500", cfg.Monitor.DebounceMs)
	}
	if cfg.Monitor.eventMask == 0 {
		t.Error("事件掩码应已解析")
	}

	// 邮件与 webhook 互斥
	cfg.Email.Enabled = true
	cfg.Webhook.Enabled = true
	if err := cfg.validate(); err == nil {
		t.Error("email 与 webhook 同时启用应报错")
	}

	// email 必填项
	cfg2 := &Config{}
	cfg2.Monitor.Directories = []string{"/data"}
	cfg2.Email.Enabled = true
	if err := cfg2.validate(); err == nil {
		t.Error("email 缺少必填项应报错")
	}

	// webhook provider 校验
	cfg3 := &Config{}
	cfg3.Monitor.Directories = []string{"/data"}
	cfg3.Webhook.Enabled = true
	cfg3.Webhook.Provider = "dingtalk"
	if err := cfg3.validate(); err == nil {
		t.Error("不支持的 webhook provider 应报错")
	}
}
