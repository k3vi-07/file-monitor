package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-resty/resty/v2"
)

const (
	notifyTimeout      = 10 * time.Second
	dispatcherQueueLen = 1024
)

type notifier interface {
	name() string
	notify(ctx context.Context, event fsnotify.Event) error
}

func buildNotifier(cfg *Config) notifier {
	switch {
	case cfg.Webhook.Enabled:
		return newServerChanNotifier(cfg.Webhook.Sendkey)
	case cfg.Email.Enabled:
		return &emailNotifier{cfg: &cfg.Email}
	default:
		return nil
	}
}

func formatEventBody(event fsnotify.Event) string {
	return fmt.Sprintf("文件路径: %s\n操作类型: %s\n时间: %s",
		event.Name, event.Op.String(), time.Now().Format("2006-01-02 15:04:05"))
}

// emailNotifier 基于 net/smtp 手工实现会话流程，以支持整体超时控制
// 和 STARTTLS；465 隐式 TLS 端口不被 net/smtp 支持，请使用 587。
type emailNotifier struct {
	cfg *EmailConfig
}

func (n *emailNotifier) name() string { return "邮件" }

func (n *emailNotifier) notify(ctx context.Context, event fsnotify.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	addr := net.JoinHostPort(n.cfg.SmtpHost, strconv.Itoa(n.cfg.SmtpPort))
	conn, err := net.DialTimeout("tcp", addr, notifyTimeout)
	if err != nil {
		return fmt.Errorf("连接 SMTP 服务器失败: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(notifyTimeout))

	client, err := smtp.NewClient(conn, n.cfg.SmtpHost)
	if err != nil {
		return fmt.Errorf("创建 SMTP 会话失败: %w", err)
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: n.cfg.SmtpHost}); err != nil {
			return fmt.Errorf("STARTTLS 升级失败: %w", err)
		}
	}
	if err := client.Auth(smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.SmtpHost)); err != nil {
		return fmt.Errorf("SMTP 认证失败: %w", err)
	}
	if err := client.Mail(n.cfg.From); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	for _, rcpt := range n.cfg.To {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("设置收件人 %s 失败: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("打开邮件数据通道失败: %w", err)
	}
	if _, err := w.Write(n.buildMessage(event)); err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("发送邮件内容失败: %w", err)
	}
	return client.Quit()
}

// buildMessage 组装符合 RFC 5322 的邮件：补全 From 头（缺失易被判为垃圾邮件），
// 非 ASCII 主题按 RFC 2047 编码，正文声明 UTF-8 字符集。
func (n *emailNotifier) buildMessage(event fsnotify.Event) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", n.cfg.From)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(n.cfg.To, ","))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", "文件变动通知"))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	for _, line := range strings.Split(formatEventBody(event), "\n") {
		b.WriteString(line)
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

type serverChanNotifier struct {
	sendkey string
	client  *resty.Client
}

type serverChanResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newServerChanNotifier(sendkey string) *serverChanNotifier {
	return &serverChanNotifier{
		sendkey: sendkey,
		client:  resty.New().SetTimeout(notifyTimeout),
	}
}

func (n *serverChanNotifier) name() string { return "Server酱" }

func (n *serverChanNotifier) notify(ctx context.Context, event fsnotify.Event) error {
	var result serverChanResp
	resp, err := n.client.R().
		SetContext(ctx).
		SetFormData(map[string]string{
			"title": "文件变动通知",
			"desp":  formatEventBody(event),
		}).
		SetResult(&result).
		Post(fmt.Sprintf("https://sctapi.ftqq.com/%s.send", n.sendkey))
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("意外的状态码: %d", resp.StatusCode())
	}
	// Server酱出错时仍可能返回 HTTP 200，需解析 body 中的 code
	if result.Code != 0 {
		return fmt.Errorf("Server酱返回错误: %s", result.Message)
	}
	return nil
}

// dispatcher 将事件异步投递给通知渠道：事件先进入带缓冲队列，由单个 worker
// 消费并按防抖窗口合并，避免同步通知阻塞事件循环（导致事件丢失），以及
// 编辑器保存产生的事件风暴触发大量重复通知。
type dispatcher struct {
	notify   notifier
	events   chan fsnotify.Event
	debounce time.Duration
	wg       sync.WaitGroup
}

func newDispatcher(n notifier, debounce time.Duration) *dispatcher {
	return &dispatcher{
		notify:   n,
		events:   make(chan fsnotify.Event, dispatcherQueueLen),
		debounce: debounce,
	}
}

func (d *dispatcher) start() {
	if d.notify == nil {
		return
	}
	d.wg.Add(1)
	go d.run()
}

func (d *dispatcher) enqueue(event fsnotify.Event) {
	if d.notify == nil {
		return
	}
	select {
	case d.events <- event:
	default:
		log.Printf("通知队列已满，已丢弃事件: %s", event.Name)
	}
}

func (d *dispatcher) run() {
	defer d.wg.Done()

	pending := make(map[string]fsnotify.Event)
	timer := time.NewTimer(0)
	<-timer.C // 立即耗尽，使 timer 处于已停止且已排空状态

	for {
		select {
		case event, ok := <-d.events:
			if !ok {
				d.flush(pending)
				return
			}
			// 同一文件的连续事件按操作位合并，窗口结束时只通知一次
			if prev, exists := pending[event.Name]; exists {
				event.Op |= prev.Op
			}
			pending[event.Name] = event
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(d.debounce)
		case <-timer.C:
			d.flush(pending)
			pending = make(map[string]fsnotify.Event)
		}
	}
}

func (d *dispatcher) flush(pending map[string]fsnotify.Event) {
	if len(pending) == 0 {
		return
	}
	ctx := context.Background()
	for _, event := range pending {
		if err := d.notify.notify(ctx, event); err != nil {
			log.Printf("%s通知失败: %v", d.notify.name(), err)
		} else {
			log.Printf("%s通知成功: %s", d.notify.name(), event.Name)
		}
	}
}

func (d *dispatcher) close() {
	close(d.events)
	d.wg.Wait()
}
