# 跨平台文件监控工具

## 功能特性
- 实时监控 macOS/Linux/Windows 目录文件变动（**递归监控**，含全部子目录，运行期间新建的目录会自动纳入）
- 按事件类型过滤（创建、写入、删除、重命名、权限变更）
- 多目录配置支持
- 灵活的忽略规则（文件模式、扩展名、目录模式）
- 事件防抖合并（同一文件的连续变动在时间窗口内合并为一次通知）
- 通知异步发送，带超时控制，不阻塞监控、不丢事件
- 日志轮转功能（按大小/时间自动分割压缩）
- 多种通知方式（邮件/Server酱Webhook），敏感信息支持环境变量注入
- 优雅退出（SIGINT/SIGTERM）

## 安装要求
- Go 1.24+ 开发环境
- 系统要求：macOS/Linux kernel 4.4+/Windows 10+

## 快速开始
1. 复制配置文件模板：
```bash
cp config.yaml.example config.yaml
```

2. 编辑配置文件（通知方式二选一）：
```yaml
# 选择邮件通知
email:
  enabled: true
  smtpHost: "smtp.example.com"
  smtpPort: 587              # STARTTLS 端口；465 隐式 TLS 暂不支持
  username: "your_email@example.com"
  password: "your_app_password"   # 建议用环境变量 EMAIL_PASSWORD 注入
  from: "noreply@example.com"
  to: ["admin@example.com"]

# 或选择Webhook通知（Server酱）
webhook:
  enabled: true
  provider: "serverchan"
  sendkey: "your_sendkey_here"    # 从Server酱官网获取，或用环境变量 WEBHOOK_SENDKEY 注入

monitor:
  directories:
    - "/var/www"        # Linux路径示例
    - "C:\\webroot"     # Windows路径示例（双反斜杠转义）
  ignore:
    files: ["*.tmp", ".DS_Store"]
    extensions: [".log", ".bak"]
    directories: ["/var/www/logs", "temp", "node_modules"]
  events: ["create", "write", "remove", "rename"]  # 可选 chmod；留空 = 全部
  debounceMs: 500       # 事件合并窗口，避免编辑器保存触发通知风暴

logging:
  file: "logs/monitor.log"
  maxSize: 10           # 单个日志文件最大大小(MB)
  maxBackups: 5         # 保留的旧日志文件数量
  maxAge: 30            # 保留旧日志的最大天数
  compress: true        # 是否压缩旧日志
```

3. 编译运行：
```bash
go build -o monitor .
./monitor                          # 在配置文件所在目录运行
./monitor -config /path/config.yaml  # 指定配置文件路径
go test ./...                      # 运行单元测试
```

## 配置说明

### 忽略规则
- `files`：glob 模式，只匹配最后一级文件名（如 `*.tmp`、`.DS_Store`）
- `extensions`：扩展名匹配，不区分大小写（`.JPG` 与 `.jpg` 等价）
- `directories`：两种语义
  - 绝对路径（如 `/var/www/logs`）：前缀匹配，忽略该目录及其全部子内容
  - 相对名称（如 `temp`、`node_modules`）：路径中任意一段命中即忽略

### 事件类型
`create` / `write` / `remove` / `rename` / `chmod`，未配置时默认监听全部。事件携带多个操作位（如 `CREATE|WRITE`）时，只要任一操作被监听即通过过滤。

### 邮件注意事项
- 使用 587 (STARTTLS) 端口；`net/smtp` 不支持 465 隐式 TLS
- 建议使用应用专用密码而非邮箱登录密码，密码可通过环境变量 `EMAIL_PASSWORD` 注入
- 不同邮件服务商 SMTP 配置不同，请参考对应文档

### 日志与监控目录
若日志文件位于监控目录内，请务必将日志目录加入 `ignore.directories`（模板已示例），否则日志写入会持续触发文件事件。

## 跨平台注意事项
1. Windows路径使用双反斜杠转义
2. 文件权限设置需符合操作系统规范
3. 监控系统目录需要管理员/root权限
4. 递归监控会为每个子目录占用一个文件描述符，监控超大目录树时注意调大 `ulimit -n`

## 项目结构
```
main.go      入口：配置加载、日志初始化、信号处理
config.go    配置定义、加载与启动期校验
monitor.go   监控器：递归加目录、事件循环与过滤
ignore.go    忽略规则匹配
notify.go    通知渠道（邮件/Server酱）与异步防抖分发
```
