package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Monitor struct {
	cfg        *Config
	watcher    *fsnotify.Watcher
	dispatcher *dispatcher
}

func NewMonitor(cfg *Config) (*Monitor, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("创建 watcher 失败: %w", err)
	}
	m := &Monitor{
		cfg:     cfg,
		watcher: watcher,
		dispatcher: newDispatcher(
			buildNotifier(cfg),
			time.Duration(cfg.Monitor.DebounceMs)*time.Millisecond,
		),
	}
	m.dispatcher.start()
	return m, nil
}

// watchAll 递归添加所有监控目录，返回成功纳入监控的目录总数。
func (m *Monitor) watchAll() int {
	total := 0
	for _, dir := range m.cfg.Monitor.Directories {
		n := m.watchRecursive(dir)
		if n == 0 {
			log.Printf("监控目录添加失败: %s", dir)
			continue
		}
		log.Printf("监控目录就绪: %s（含 %d 个子目录）", dir, n-1)
		total += n
	}
	return total
}

// watchRecursive 遍历目录树并逐个添加 watch（fsnotify 不递归，必须显式添加
// 每个子目录）；命中忽略规则的子目录整棵跳过。返回成功添加的目录数。
func (m *Monitor) watchRecursive(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("遍历目录失败: %s 错误: %v", path, err)
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root {
			if ignored, _ := shouldIgnore(path, m.cfg); ignored {
				return filepath.SkipDir
			}
		}
		if err := m.watcher.Add(path); err != nil {
			log.Printf("添加监控失败: %s 错误: %v", path, err)
			return nil
		}
		count++
		return nil
	})
	return count
}

func (m *Monitor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-m.watcher.Events:
			if !ok {
				return nil
			}
			m.handleEvent(event)
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func (m *Monitor) handleEvent(event fsnotify.Event) {
	// 忽略事件保持静默：若日志文件位于监控目录内，记录忽略日志本身会
	// 再次触发文件事件，形成自我激励的写入循环。
	if ignored, _ := shouldIgnore(event.Name, m.cfg); ignored {
		return
	}
	if !event.Op.Has(m.cfg.Monitor.eventMask) {
		return
	}
	log.Printf("处理文件事件: 操作=%s 文件=%s", event.Op, event.Name)

	// 运行期间新建的目录动态纳入监控，保证递归语义持续生效
	if event.Has(fsnotify.Create) {
		if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
			if n := m.watchRecursive(event.Name); n > 0 {
				log.Printf("新增目录已纳入监控: %s（含 %d 个子目录）", event.Name, n-1)
			}
		}
	}

	m.dispatcher.enqueue(event)
}

func (m *Monitor) Close() {
	m.dispatcher.close()
	if err := m.watcher.Close(); err != nil {
		log.Printf("关闭 watcher 失败: %v", err)
	}
}
