# 跨平台文件监控工具

## 功能特性
- 实时监控Linux/Windows系统目录文件变动
- 支持文件创建、修改、删除、重命名等事件检测
- 多目录配置支持
- 灵活的忽略规则配置（支持文件模式、扩展名、目录模式）
- 日志轮转功能（按大小/时间自动分割压缩）
- 多种通知方式（邮件/Server酱Webhook）

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
  to: ["admin@example.com"]

# 或选择Webhook通知（Server酱）
webhook:
  enabled: true
  provider: "serverchan"
  sendkey: "your_sendkey_here"  # 从Server酱官网获取
  template: "default"

monitor:
  directories:
    - "/var/www"  # Linux路径示例
    - "C:\\webroot" # Windows路径示例
  ignore:
    files: [".*", "*.tmp"]  # 忽略隐藏文件和临时文件
    extensions: [".log", ".bak"]  # 忽略日志和备份文件
    directories: ["temp", "cache"]  # 忽略临时和缓存目录
  events: ["create", "write", "remove", "rename"]

logging:
  file: "logs/monitor.log"  # 日志文件路径
  maxSize: 10  # 单个日志文件最大大小(MB)
  maxBackups: 5  # 保留的旧日志文件数量
  maxAge: 30  # 保留旧日志的最大天数
  compress: true  # 是否压缩旧日志
```

3. 配置注意事项：
   - 邮件通知：
     * 使用SSL/TLS加密端口（如587）
     * 建议使用应用专用密码而非邮箱登录密码
     * 不同邮件服务商SMTP配置不同，请参考对应文档
   - Webhook通知：
     * 需要从Server酱官网获取sendkey
     * 仅支持Server酱服务
   - 忽略规则：
     * 支持glob模式匹配
     * 多个规则用数组形式配置
   - 日志配置：
     * 自动按大小或时间分割日志
     * 旧日志会自动压缩

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
