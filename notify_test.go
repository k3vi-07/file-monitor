package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

type fakeNotifier struct {
	mu     sync.Mutex
	events []fsnotify.Event
}

func (f *fakeNotifier) name() string { return "fake" }

func (f *fakeNotifier) notify(_ context.Context, event fsnotify.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
	return nil
}

func (f *fakeNotifier) collected() []fsnotify.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fsnotify.Event(nil), f.events...)
}

// 同一文件在防抖窗口内的连续事件应合并为一次通知，操作位取并集
func TestDispatcherDebounce(t *testing.T) {
	fake := &fakeNotifier{}
	d := newDispatcher(fake, 50*time.Millisecond)
	d.start()
	defer d.close()

	d.enqueue(fsnotify.Event{Op: fsnotify.Create, Name: "/a.txt"})
	d.enqueue(fsnotify.Event{Op: fsnotify.Write, Name: "/a.txt"})
	d.enqueue(fsnotify.Event{Op: fsnotify.Write, Name: "/b.txt"})

	deadline := time.Now().Add(2 * time.Second)
	for len(fake.collected()) < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	got := fake.collected()
	if len(got) != 2 {
		t.Fatalf("期望合并后 2 条通知，got %d: %+v", len(got), got)
	}
	for _, ev := range got {
		if ev.Name == "/a.txt" && ev.Op != fsnotify.Create|fsnotify.Write {
			t.Errorf("/a.txt 操作应合并为 CREATE|WRITE，got %v", ev.Op)
		}
	}
}

// 未启用任何通知渠道时 dispatcher 应为空操作，不产生 goroutine
func TestDispatcherNoNotifier(t *testing.T) {
	d := newDispatcher(nil, time.Millisecond)
	d.start()
	d.enqueue(fsnotify.Event{Op: fsnotify.Create, Name: "/a.txt"})
	d.close()
}
