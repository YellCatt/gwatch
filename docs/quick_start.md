# 快速上手指南

## 5 分钟快速开始

### 1. 编译项目

```bash
# Windows
go build -ldflags="-s -w" -o gwatch.exe

# Linux
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o gwatch_linux_amd64
```

### 2. 准备配置

复制 `config/config_dev.yaml` 为 `config/config.yaml`，修改以下内容：

```yaml
# 修改目标服务器地址
target:
  base_url: "https://your-api-server.com"
  timeout: 30

# 配置邮件告警（可选）
email:
  enabled: true
  from: "your-email@qq.com"
  to:
    - "admin@example.com"
  auth_code: "your_smtp_auth_code"
  smtp_server: "smtp.qq.com"
  smtp_port: 465
```

### 3. 创建测试用例

在 `cases/` 目录下创建 PSV 文件（如 `api_test.psv`）：

```
id|desc|method|url|expected_status|headers|tags|monitor_enabled|monitor_interval|response_threshold|alert_on_failure|alert_on_slow
health_check|健康检查|GET|/health|200||smoke|true|60|3000|true|true
get_user|获取用户|GET|/api/user/1|200|{"Authorization":"Bearer token"}|critical|true|30|2000|true|true
create_user|创建用户|POST|/api/user|200|{"Content-Type":"application/json"}|regression|false|0|0|false|false
```

### 4. 运行

```bash
# 一次性测试所有用例
gwatch -t cases/

# 或启动持续监控（推荐生产环境）
gwatch cases/

# 运行数据采集器
gwatch scraper

# 生成系统资源报告
gwatch sys-report
```

---

## 典型使用场景

### 场景一：API 健康检查

创建最简单的健康检查用例：

```
id|desc|method|url|expected_status|monitor_enabled|alert_on_failure
api_health|API健康检查|GET|https://api.example.com/health|200|true|true
```

### 场景二：登录 → 获取用户 流程

```
id|desc|method|url|expected_status|json|extract|monitor_enabled
login|用户登录|POST|https://api.example.com/login|200|{"username":"admin","password":"123456"}|token=$.data.access_token|true
get_user|获取用户信息|GET|https://api.example.com/user/1|200|||true
```

### 场景三：主机资源监控

```yaml
# config.yaml 中的 scraper 配置
scraper:
  enabled: true
  targets:
    - name: "服务器资源"
      url: "http://server/api/status"
      method: GET
      interval: 10
      metrics:
        - name: cpu
          path: "$.cpu.usage"
          alias: "CPU使用率"
          unit: "%"
          compare_op: "gt"
          threshold: 85
          alert: true
          alert_level: "CRITICAL"
        - name: memory
          path: "$.memory.used_percent"
          alias: "内存使用率"
          unit: "%"
          compare_op: "gt"
          threshold: 90
          alert: true
```

### 场景四：标签过滤

```bash
# 只运行 smoke 和 critical 标签的用例
gwatch -t cases/ --tags smoke,critical

# 监控模式下按标签过滤
gwatch cases/ --tags api
```

---

## 辅助工具

### Probe - 探测 JSON 结构

不确定 JSONPath 怎么写？用 probe 工具自动解析：

```bash
gwatch probe http://example.com/api/status
```

输出示例：
```
{
  "status": "ok",
  "data": {
    "cpu": {
      "usage": 45.2        → $.data.cpu.usage
    },
    "memory": {
      "used_percent": 62.1  → $.data.memory.used_percent
    }
  }
}
```

### Sys-Report - 系统资源快照

```bash
gwatch sys-report
```

输出当前 CPU、内存、磁盘、网络等指标的 ASCII 图表报告。

---

## 生产部署建议

### 目录结构

```
/opt/gwatch/
├── gwatch_linux_amd64       # 可执行文件
├── config/
│   └── config.yaml          # 生产环境配置
├── cases/
│   ├── api_core.psv         # 核心接口用例
│   ├── api_user.psv         # 用户模块用例
│   └── order.psv            # 订单模块用例
├── reports/                 # 报告输出
├── data/                    # 历史数据
└── logs/                    # 日志
```

### 日志级别

生产环境建议使用 `info` 级别：

```yaml
log:
  level: "info"
  encoding: "json"
  output: "./logs/gwatch.log"
```

调试时临时切换为 `debug`：

```yaml
log:
  level: "debug"
```

### 邮件告警

确保 SMTP 配置正确，建议使用专用邮箱：

```yaml
email:
  enabled: true
  from: "monitor@example.com"
  to:
    - "oncall@example.com"
  auth_code: "your_auth_code"
  smtp_server: "smtp.example.com"
  smtp_port: 465
  # 冷却时间防止告警风暴
  scraper_cooldown: 21600
  api_cooldown: 21600
  system_cooldown: 7200
```

### 定时任务

可配合系统定时任务实现额外调度：

```cron
# 每分钟检查一次 gwatch 进程是否存活
* * * * * pgrep gwatch || /opt/gwatch/gwatch_linux_amd64 /opt/gwatch/cases/
```

---

## 常见问题

### Q: 如何添加 HTTPS 自签名证书支持？

```yaml
http:
  insecure_skip_verify: true
```

### Q: 如何设置请求超时？

在 PSV 用例的 URL 中配置：
- 全局：`target.timeout: 30`（秒）
- 单用例：`timeout: 5s`（scraper 目标中配置）

### Q: 支持哪些 HTTP 方法？

GET、POST、PUT、DELETE、PATCH、HEAD

### Q: 变量提取后如何在另一个用例中使用？

在 `extract` 字段中提取变量，在后续用例的 URL/Headers/Body 中通过 `{{变量名}}` 引用。前置条件机制可保证执行顺序。

### Q: 报告保存在哪里？

默认保存在 `reports/` 目录，可通过 `app.report_dir` 配置修改。