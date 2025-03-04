# 跨平台文件监控工具

## 功能特性
- 实时监控Linux/Windows系统目录文件变动
- 支持创建、修改、删除事件检测
- 多目录配置支持
- 跨平台兼容性处理
- 日志记录和事件通知

## 安装要求
- Go 1.18+ 开发环境
- 系统要求：Windows 10+/Linux kernel 4.4+

## 快速开始
1. 复制配置文件模板：
```bash
cp config.yaml.example config.yaml
```

2. 编辑配置文件（二选一配置）：
```yaml
# 选择邮件通知
email:
  enabled: true
  smtpHost: "smtp.example.com"
  smtpPort: 587
  username: "your_email@example.com"
  password: "your_app_password"
  from: "noreply@example.com"
  to: "admin@example.com"

# 或选择Webhook通知
webhook:
  enabled: true
  provider: "serverchan"
  sendkey: "your_sendkey_here"
  template: "default"

monitor:
  directories:
    - "/var/www"  # Linux路径示例
    - "C:\\webroot" # Windows路径示例
  interval: 1s
  smtpHost: "smtp.example.com"
  smtpPort: 587
  username: "your_email@example.com"
  password: "your_app_password" 
  from: "noreply@example.com"
  to:
    - "admin@example.com"
```

3. 邮件配置注意事项：
   - 使用SSL/TLS加密端口（如587）
   - 建议使用应用专用密码而非邮箱登录密码
   - 不同邮件服务商SMTP配置不同，请参考对应文档

4. 编译运行：
```bash
# Linux
go build -o monitor main.go
./monitor

# Windows
go build -o monitor.exe main.go
monitor.exe
```

## 跨平台注意事项
1. Windows路径使用双反斜杠转义
2. 文件权限设置需符合操作系统规范
3. 监控系统目录需要管理员/root权限
