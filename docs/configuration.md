# 配置文件详解

## 配置加载

gwatch 使用 YAML 格式配置文件，默认路径为 `./config/config.yaml`，可通过 `--config` 参数指定其他路径。

```bash
gwatch --config /path/to/config.yaml cases/
```

## 完整配置项

### target - 全局请求配置

```yaml
target:
  base_url: "https://api.example.com"   # 基础 URL，用例中的相对路径会拼接到此 URL
  timeout: 30                            # 全局请求超时（秒）
  authorization: ""                      # 全局 Authorization 请求头
  user_id: ""                            # 用户标识
```

### log - 日志配置

```yaml
log:
  level: "info"                          # 日志级别: debug, info, warn, error
  encoding: "json"                       # 日志编码: json, console
  output: "./logs/gwatch.log"            # 日志输出路径
```

### app - 应用配置

```yaml
app:
  report_dir: "./reports"               # 报告输出目录
  case_dir: "./cases"                    # 测试用例目录
  data_dir: "./data"                     # 数据存储目录
  severe_status: [500]                   # 严重状态码列表
  global_pre: []                         # 全局前置条件用例 ID
  global_post: []                        # 全局后置条件用例 ID
  host_name: ""                          # 自定义主机名（为空则使用系统名）
```

### http - HTTP 客户端配置

```yaml
http:
  insecure_skip_verify: false            # 跳过 HTTPS 证书验证
```

### vars - 全局变量

```yaml
vars:
  api_key: "your_api_key"                # 可在 PSV 用例中通过 {{api_key}} 引用
  # ... 其他全局变量
```

### monitor - 接口监控配置

```yaml
monitor:
  enabled: true
  default_interval: 60                   # 默认监控间隔（秒）
  alert_on_failure: true                 # 全局失败告警开关
  alert_on_slow: true                    # 全局慢响应告警开关
  max_workers: 1                         # 最大并发执行数
  alert_interval: 21600                  # 告警冷却时间（秒）
  daily_report: true                     # 启用日报
  weekly_report: false                   # 启用周报
  monthly_report: false                  # 启用月报
  yearly_report: false                   # 启用年报
  report_time: "07:00"                   # 报告生成时间（东八区）
```

### scraper - 数据采集器配置

```yaml
scraper:
  enabled: true
  targets:
    - name: "目标名称"
      url: "http://example.com/api"
      method: GET
      timeout: 5s
      interval: 10
      enabled: true
      headers: {}
      body: ""
      proxy: ""
      metrics:
        - name: "metric_name"
          path: "$.path.to.value"
          alias: "指标别名"
          unit: "%"
          scale: 1
          auto_percent: false
          compare_op: "gt"
          threshold: 85
          warning_threshold: 70
          alert: true
          alert_level: "CRITICAL"
          consecutive: 1
          optional: false
```

### email - 邮件配置

```yaml
email:
  enabled: true
  from: "sender@example.com"
  to:
    - "recipient@example.com"
  auth_code: "your_auth_code"
  smtp_server: "smtp.qq.com"
  smtp_port: 465
  error_subject: "[gwatch] 监控系统错误告警"
  scraper_cooldown: 21600                # 采集告警冷却（秒）
  api_cooldown: 21600                    # 接口告警冷却（秒）
  system_cooldown: 7200                  # 系统告警冷却（秒）
```

### cleaner - 自动清理配置

```yaml
cleaner:
  enabled: true
  retention_days: 30
  log_dir: "./logs"
  report_dir: "./reports"
  # data_dir 已废弃：数据存储目录下的系统指标/告警/汇总 CSV 属于业务数据，不参与清理
  include_patterns: ["*.log", "*.json", "*.txt"]
  exclude_patterns: []
  interval_hours: 24
```

### sys_monitor - 系统监控配置

```yaml
sys_monitor:
  enabled: true
  interval: 10
  chart_enabled: true
  email_enabled: true
  cpu_threshold: 85
  memory_threshold: 90
  disk_usage_threshold: 90
  network_down_threshold: 3072
  network_up_threshold: 1024
  network_down_warn_threshold: 2048
  network_up_warn_threshold: 512
  alert_cooldown: 7200
```