# gwatch

一个功能强大的企业级 API 监控与系统监控工具，使用 Go 语言编写，支持 RESTful API 监控、远程主机指标采集和本机系统资源监控。

## 功能特性

### API 监控
- RESTful API 接口监控与测试
- PSV（管道分隔值）监控任务管理
- YAML 配置管理
- 串行执行（确保依赖顺序）
- 基于标签的任务过滤
- 变量提取和替换
- 流式（SSE）断言支持
- 正则表达式断言支持（支持匹配数字、布尔等非字符串类型）
- CSV 历史执行时间存储和平均值计算
- 任务延迟控制（执行前/后延迟）
- 并发执行支持（max_workers 配置）

### 远程指标采集（Scraper）
- 远程主机指标数据采集（CPU、内存、磁盘、GPU、网络等）
- 灵活的 JSONPath 指标提取
- 多级告警阈值（CRITICAL / WARNING）
- 连续触发次数控制（防抖动）
- 告警冷却机制（避免重复告警）

### 本机系统监控（SysMonitor）
- 本机 CPU、内存、磁盘、网络实时监控
- ASCII 图表生成
- 多级阈值告警
- 邮件告警通知

### 报告与告警
- 邮件告警通知（支持设备名标识）
- 定期报告生成（日/周/月/年）
- 告警冷却机制（Scraper/API/System 独立冷却）
- **自定义设备名**（host_name 配置，用于告警邮件标识）

### 运维
- 自动清理机制（定期清理日志和报告文件）
- 全局前置/后置条件（所有任务执行前后运行）
- 热重载配置
- Zap 结构化日志（支持日志分割）

## 环境要求

- Go 1.22+

## 安装

### 从源码编译

```bash
go mod download
go build -ldflags="-s -w" -o gwatch.exe
```

## 运行时文件结构

编译后的 `gwatch.exe` 运行时需要以下文件结构：

```
gwatch.exe           # 可执行文件
config/             # 配置目录
  └── config.yaml   # 配置文件
cases/              # 测试用例目录（可选）
  └── *.psv         # PSV 格式的监控任务文件
reports/            # 报告输出目录（自动创建）
sql/                # CSV 数据目录（自动创建）
logs/               # 日志目录（自动创建）
```

## 配置

编辑 `config/config.yaml` 文件，以下为完整配置项：

```yaml
target:
  base_url: "https://httpbin.org"       # 基础URL，用于测试用例中的相对URL拼接
  timeout: 30                           # 请求超时时间（秒）
  authorization: ""                     # 全局 Authorization 请求头（可选）
  user_id: ""                           # 用户ID标识（可选）

log:
  level: "info"                         # 日志级别: debug, info, warn, error
  encoding: "json"                      # 日志编码: json, console
  output: "./logs/gwatch.log"           # 日志输出路径
  max_size_mb: 20                       # 单个日志文件最大大小（MB），超过后自动分割

app:
  report_dir: "./reports"               # 报告输出目录
  case_dir: "./cases"                   # 测试用例目录
  data_dir: "./sql"                     # 数据存储目录（CSV/SQLite）
  severe_status: [500]                  # 严重状态码列表，触发时会发送告警
  global_pre: []                        # 全局前置条件：执行所有测试用例前先执行这些用例
  global_post: []                       # 全局后置条件：执行所有测试用例后执行这些用例
  host_name: ""                         # 自定义设备名（为空则使用系统主机名）

http:
  insecure_skip_verify: false           # 是否跳过HTTPS证书验证

vars: {}                                # 全局变量，可在测试用例中通过 {{key}} 引用

monitor:
  enabled: true                         # 是否启用监控模式
  default_interval: 60                  # 默认监控间隔（秒）
  alert_on_failure: true                # 接口失败时是否发送告警邮件
  alert_on_slow: true                   # 响应用超时时是否发送告警邮件
  max_workers: 1                        # 最大并发执行任务数
  alert_interval: 21600                 # 同一接口异常后再次发送邮件的最小间隔（秒），默认6小时
  daily_report: true                    # 是否启用每日报告
  weekly_report: true                   # 是否启用每周报告
  monthly_report: true                  # 是否启用每月报告
  yearly_report: true                   # 是否启用每年报告
  report_time: "07:00"                  # 报告生成时间（HH:MM格式）

scraper:
  enabled: true                         # 是否启用数据采集器
  targets:                              # 采集目标列表
    - name: "远程主机监控"               # 目标名称
      url: "http://192.168.1.100/api/status" # 采集接口URL
      method: GET                       # HTTP方法: GET, POST, PUT等
      timeout: 5s                       # 请求超时时间
      interval: 10                      # 采集间隔（秒）
      enabled: true                     # 是否启用此目标
      headers: {}                       # 自定义请求头
      body: ""                          # 请求体（POST/PUT时使用）
      proxy: ""                         # 代理地址
      metrics:                          # 采集指标列表
        - name: cpu_usage               # 指标名称
          path: "$.info.cpu.use_percent" # JSONPath路径
          alias: "CPU使用率"            # 指标别名（用于报告显示）
          unit: "%"                     # 单位
          scale: 1                      # 缩放倍数
          auto_percent: false           # 自动百分比转换
          compare_op: "gt"              # 比较操作符: gt, lt, eq, ge, le
          threshold: 85                 # 告警阈值
          warning_threshold: 70         # 警告阈值
          alert: true                   # 是否启用此指标的告警
          alert_level: "CRITICAL"       # 告警级别: CRITICAL, WARNING
          consecutive: 1                # 连续触发次数才告警
          optional: false               # 可选指标：接口不返回时不报错

email:
  enabled: true                         # 是否启用邮件通知
  from: "sender@example.com"            # 发件人地址
  to:                                   # 收件人列表
    - "recipient@example.com"
  auth_code: "your_auth_code"           # SMTP授权码
  smtp_server: "smtp.example.com"       # SMTP服务器地址
  smtp_port: 465                        # SMTP端口
  error_subject: "[gwatch] 监控系统错误告警" # 错误告警邮件主题
  scraper_cooldown: 21600               # 远程采集告警冷却时间（秒）
  api_cooldown: 21600                   # 接口监控告警冷却时间（秒）
  system_cooldown: 7200                 # 系统资源告警冷却时间（秒）

cleaner:
  enabled: true                         # 是否启用自动清理
  retention_days: 30                    # 数据保留天数
  log_dir: "./logs"                     # 日志目录
  report_dir: ""                        # 报告目录（留空则不清理）
  data_dir: ""                          # 数据目录（留空则不清理）
  include_patterns: ["*.log", "*.json", "*.csv", "*.txt"]
  exclude_patterns: []
  interval_hours: 24                    # 清理执行间隔（小时）

sys_monitor:
  enabled: true                         # 是否启用系统资源监控
  interval: 10                          # 采集间隔（秒）
  chart_enabled: true                   # 是否生成ASCII图表
  email_enabled: true                   # 是否启用邮件告警
  cpu_threshold: 85                     # CPU使用率严重告警阈值（%）
  memory_threshold: 90                  # 内存使用率严重告警阈值（%）
  disk_usage_threshold: 90              # 磁盘使用率严重告警阈值（%）
  network_down_threshold: 3072          # 网络下行速度严重告警阈值（KB/s）
  network_up_threshold: 1024            # 网络上行速度严重告警阈值（KB/s）
  network_down_warn_threshold: 2048     # 网络下行速度警告阈值（KB/s）
  network_up_warn_threshold: 512        # 网络上行速度警告阈值（KB/s）
  alert_cooldown: 7200                  # 告警冷却时间（秒）
```

### 必需文件

| 文件/目录 | 说明 | 是否必需 |
|-----------|------|----------|
| `gwatch.exe` | 主程序可执行文件 | **是** |
| `config/config.yaml` | 配置文件 | **是** |
| `tasks/` | 监控任务目录 | 否（运行时指定路径则不需要） |
| `reports/` | 报告输出目录 | 否（自动创建） |
| `sql/` | CSV 数据目录 | 否（自动创建） |
| `logs/` | 日志目录 | 否（自动创建） |

## 项目结构

```
gwatch/
├── cmd/
│   ├── root.go          # 主命令入口
│   ├── scraper.go       # 采集器命令
│   └── sysreport.go     # 系统报告命令
├── config/
│   ├── config.go        # 配置加载/重载核心逻辑
│   ├── types.go         # 配置类型定义
│   └── defaults.go      # 默认值设置
├── internal/
│   ├── assert/          # 断言逻辑
│   ├── bootstrap/       # 启动引导
│   ├── cleaner/         # 清理器
│   ├── email/           # 邮件发送（含模板）
│   ├── httpclient/      # HTTP 客户端
│   ├── logger/          # 日志（含日志分割）
│   ├── monitor/         # API 监控模式（含热重载）
│   ├── psv/             # PSV 解析
│   ├── report/          # 报告生成（日/周/月/年/启动报告）
│   ├── scheduler/       # 调度器
│   ├── scraper/         # 远程指标采集器
│   ├── storage/         # 数据存储（CSV/SQLite）
│   ├── sysmon/          # 本机系统资源监控
│   ├── testcase/        # 测试用例执行
│   ├── timeutil/        # 时间工具
│   ├── util/            # 工具函数（设备名、格式化）
│   └── vars/            # 变量管理
├── go.mod
└── go.sum
```

### 配置说明

#### 基础配置
- **target.base_url**: API 目标地址，测试用例中可用 `{{base_url}}` 引用
- **target.timeout**: 请求超时时间（秒）
- **target.authorization**: 全局 Authorization 请求头
- **log.level**: 日志级别（debug, info, warn, error）
- **log.encoding**: 日志格式（json, console）
- **log.output**: 日志输出路径
- **log.max_size_mb**: 单个日志文件最大大小（MB），超过后自动分割

#### 应用配置
- **app.report_dir**: 监控报告输出目录
- **app.case_dir**: 默认测试用例目录
- **app.data_dir**: 数据存储目录（CSV/SQLite）
- **app.severe_status**: 严重错误状态码列表，触发时发送告警
- **app.global_pre**: 全局前置条件测试用例 ID 列表
- **app.global_post**: 全局后置条件测试用例 ID 列表
- **app.host_name**: 自定义设备名，用于告警邮件标识（留空则使用系统主机名）

#### 监控配置
- **monitor.enabled**: 启用 API 监控模式
- **monitor.default_interval**: 默认监控间隔（秒）
- **monitor.alert_on_failure**: 接口失败时发送告警邮件
- **monitor.alert_on_slow**: 响应超时时发送告警邮件
- **monitor.max_workers**: 最大并发执行任务数
- **monitor.alert_interval**: 同一接口告警最小发送间隔（秒）
- **monitor.daily_report / weekly_report / monthly_report / yearly_report**: 定期报告开关
- **monitor.report_time**: 报告生成时间（HH:MM 格式）

#### 采集器配置（Scraper）
- **scraper.enabled**: 启用远程指标采集
- **scraper.targets**: 采集目标列表
  - **name**: 目标名称
  - **url**: 采集接口 URL
  - **method / timeout / interval**: HTTP 方法、超时、采集间隔
  - **metrics**: 指标列表，支持以下字段：
    - **name**: 指标唯一标识
    - **path**: JSONPath 提取路径
    - **alias**: 指标别名（用于报告展示）
    - **unit / scale**: 单位与缩放倍数
    - **compare_op**: 比较操作符（gt/lt/eq/ge/le）
    - **threshold / warning_threshold**: 告警阈值与警告阈值
    - **alert / alert_level**: 是否告警与告警级别（CRITICAL/WARNING）
    - **consecutive**: 连续触发次数才告警
    - **optional**: 可选指标（接口不返回时不报错）

#### 系统监控配置（SysMonitor）
- **sys_monitor.enabled**: 启用本机系统资源监控
- **sys_monitor.interval**: 采集间隔（秒）
- **sys_monitor.chart_enabled**: 生成 ASCII 图表
- **sys_monitor.email_enabled**: 启用邮件告警
- **sys_monitor.cpu_threshold / memory_threshold / disk_usage_threshold**: 严重告警阈值（%）
- **sys_monitor.network_down_threshold / network_up_threshold**: 网络告警阈值（KB/s）
- **sys_monitor.network_down_warn_threshold / network_up_warn_threshold**: 网络警告阈值（KB/s）
- **sys_monitor.alert_cooldown**: 告警冷却时间（秒）

#### 邮件配置
- **email.enabled**: 启用邮件通知
- **email.from / to**: 发件人 / 收件人列表
- **email.auth_code**: SMTP 授权码
- **email.smtp_server / smtp_port**: SMTP 服务器地址 / 端口
- **email.error_subject**: 告警邮件主题
- **email.scraper_cooldown / api_cooldown / system_cooldown**: 三类告警独立冷却时间

#### 清理配置
- **cleaner.enabled**: 启用自动清理
- **cleaner.retention_days**: 数据保留天数
- **cleaner.log_dir / report_dir / data_dir**: 待清理目录
- **cleaner.include_patterns / exclude_patterns**: 文件匹配模式
- **cleaner.interval_hours**: 清理执行间隔（小时）

## 使用方法

### 运行 API 监控

```bash
# 运行默认目录下的所有监控任务
./gwatch.exe

# 运行特定的 PSV 文件
./gwatch.exe cases/task_data.psv cases/task_data2.psv

# 运行目录下的所有监控任务
./gwatch.exe cases

# 标签过滤
./gwatch.exe --tags=health           # 只运行 health 标签的任务
./gwatch.exe --tags=health,api       # 运行 health 和 api 标签的任务
```

### 监控模式（持续运行）

```bash
# 启动监控模式（持续执行）
./gwatch.exe --monitor

# 监控模式 + 标签过滤
./gwatch.exe --monitor --tags=health

# 指定监控间隔
./gwatch.exe --monitor --interval=30
```

### 启动数据采集器

```bash
# 启动远程指标采集器
./gwatch.exe --scraper

# 指定配置文件
./gwatch.exe --config /path/to/config.yaml --scraper
```

### 生成系统报告

```bash
# 生成本机系统资源报告
./gwatch.exe --sysreport
```

## PSV 监控任务格式

```psv
id|skip|desc|method|url|headers|params|form|json|body|expected_status|expected_body|tags|extract|stream_mode|stream_assert|match_mode|body_regex|pre|post|fail_mode|keep_vars|delay_ms|delay_after_ms
```

### 列说明

| 列名 | 描述 |
|------|------|
| `id` | 监控任务唯一标识 |
| `skip` | 是否跳过任务（0/1 或 true/false） |
| `desc` | 监控任务描述 |
| `method` | HTTP 方法（GET, POST, PUT, DELETE, PATCH, HEAD） |
| `url` | API 端点 URL |
| `headers` | 请求头（JSON 对象） |
| `params` | URL 查询参数 |
| `form` | 表单数据 |
| `json` | JSON 请求体 |
| `body` | 原始请求体 |
| `expected_status` | 期望的 HTTP 状态码 |
| `expected_body` | 期望的 JSON 响应 |
| `tags` | 用于过滤的标签（逗号分隔） |
| `extract` | 从响应中提取变量（例如：`var=path`） |
| `stream_mode` | 启用 SSE 流式模式（0/1） |
| `stream_assert` | 流式断言规则（JSON 数组） |
| `match_mode` | 匹配模式：`exact`（精确匹配，默认）或 `subset`（子集匹配） |
| `body_regex` | 响应体的正则表达式模式 |
| `pre` | 前置条件测试 ID（分号分隔） |
| `post` | 后置条件测试 ID（分号分隔） |
| `fail_mode` | 失败模式：`stop`（默认，前置条件失败则停止）或 `continue` |
| `keep_vars` | 是否保留提取的变量（0/1，默认 0，执行后自动清理） |
| `delay_ms` | 执行前延迟时间（毫秒） |
| `delay_after_ms` | 执行后延迟时间（毫秒） |

### 示例

```psv
get_ip|0|获取IP地址|GET|{{base_url}}/ip|{}|||{}||200|{"origin":"{{regex:^[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}$}}"}|api|ip=origin|||exact|||
post_json|0|POST JSON|POST|{{base_url}}/post|{}|||{"name":"test"}||200|{"json":{"name":"test"}}|api|||subset|||
```

## 变量替换

所有字段都支持 `{{var}}` 变量替换：

```psv
# 在配置中定义 base_url 或从响应中提取
get_users|0|获取用户列表|GET|{{base_url}}/users|{}|||{}||200|{}|api|||
```

### 变量提取

从响应中提取变量：

```psv
# 从响应中提取 user_id
create_user|0|创建用户|POST|{{base_url}}/users|{}|||{"name":"test"}||201|{}|api|user_id=id|||

# 使用提取的变量
get_user|0|获取用户|GET|{{base_url}}/users/{{user_id}}|{}|||{}||200|{}|api|||
```

## 正则表达式断言

### 字段级正则

```psv
id|skip|desc|method|url|expected_status|expected_body|tags
regex_01|0|检查IP格式|GET|{{base_url}}/ip|200|{"origin":"{{regex:^[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}$}}"}|regex
regex_02|0|检查无错误|GET|{{base_url}}/status/200|200|{"message":"{{not_regex:error}}"}|regex
regex_03|0|检查数字类型字段|GET|{{base_url}}/stats|200|{"count":"{{regex:\\d+}}"}|regex
```

### 响应体正则

```psv
id|skip|desc|method|url|expected_status|body_regex|tags
body_01|0|确保响应无错误|GET|{{base_url}}/health|200|!error|health
body_02|0|确保包含成功|GET|{{base_url}}/success|200|success|health
```

### 类型兼容说明

正则表达式断言会自动将实际响应值转换为字符串后进行匹配，因此可以匹配任何 JSON 类型：

| 实际值类型 | 转换后字符串 | 正则示例 |
|-----------|-------------|---------|
| 字符串 `"8331"` | `"8331"` | `\d+` ✓ |
| 数字 `8331` | `"8331"` | `\d+` ✓ |
| 布尔 `true` | `"true"` | `true\|false` ✓ |
| 布尔 `false` | `"false"` | `true\|false` ✓ |
| `null` | `"null"` | `null` ✓ |

### 特殊标记

| 标记 | 描述 |
|------|------|
| `{{regex:...}}` | 字段值必须匹配正则表达式（自动转换为字符串） |
| `{{not_regex:...}}` | 字段值必须不匹配正则表达式（自动转换为字符串） |
| `{{skip}}` | 跳过此字段检查 |
| `{{not_exists}}` | 字段必须不存在于响应中（仅 subset 模式） |

## 匹配模式

### 精确匹配（默认）

响应 JSON 必须完全匹配 `expected_body`：

```psv
strict_01|0|严格匹配|GET|{{base_url}}/ip|200|{"origin":"{{regex:^[0-9]{1,3}\\..*}}"}|strict|
```

### 子集匹配

响应 JSON 必须至少包含 `expected_body` 中的字段：

```psv
subset_01|0|子集匹配|GET|{{base_url}}/get|200|{"args":{}}|api|subset|
```

## 延迟控制

测试用例支持两种延迟方式，用于控制测试执行节奏：

### 执行前延迟（delay_ms）

在测试用例开始执行前等待指定时间：

```psv
# 等待上游服务就绪后再执行
query_order|0|查询订单状态|GET|{{base_url}}/api/orders/{{order_id}}|{}|||{}||200||api|||1000||
```

### 执行后延迟（delay_after_ms）

在测试用例执行完成后等待指定时间，再执行下一个用例：

```psv
# 给下游服务处理时间
create_order|0|创建订单|POST|{{base_url}}/api/orders|{}|||{"amount":100}||201||api|order_id=id|||2000
```

### 同时使用两种延迟

```psv
# 执行前等待500ms，执行后等待3秒
complex_operation|0|复杂操作|POST|{{base_url}}/api/complex|{}|||{"data":"test"}||200||api|||500|3000
```

### 延迟场景建议

| 场景 | 使用字段 | 建议值 |
|------|---------|-------|
| 等待上游服务就绪 | `delay_ms` | 1000-3000ms |
| 避免触发限流 | `delay_ms` 或 `delay_after_ms` | 100-500ms |
| 给下游服务处理时间 | `delay_after_ms` | 2000-5000ms |
| 数据库最终一致性 | `delay_after_ms` | 3000-10000ms |

## 流式断言

对于 SSE 流式响应：

```psv
stream_01|0|SSE流式断言|POST|{{base_url}}/chat/completions|{"Content-Type":"application/json"}|||{"model":"gpt-4","messages":[{"role":"user","content":"hi"}],"stream":true}||200|{"aggregated_content":"{{regex:.*hi.*}}","chunk_count":"{{skip}}"}|stream||1|[{"kind":"contains","pattern":"hi","min_chunks":1}]|subset|||
```

### 流式断言规则

| 字段 | 描述 |
|------|------|
| `kind` | 断言类型：`contains`（包含）、`regex`（正则）或 `json_path`（JSON路径） |
| `pattern` | 断言模式 |
| `max_wait_ms` | 最大等待时间（预留） |
| `min_chunks` | 所需的最小块数 |

## 自定义设备名

通过配置 `app.host_name` 可以自定义告警邮件中显示的设备名：

```yaml
app:
  host_name: "生产服务器-01"   # 留空则使用系统主机名
```

该名称会出现在：
- 告警邮件正文中（`【设备名称】xxx`）
- ASCII 图表中
- 各类报告中
- 邮件主题（如果支持占位符）

## 命令行参数

| 参数 | 说明 |
|------|------|
| `--config` | 指定配置文件路径（默认 `./config/config.yaml`） |
| `--monitor` | 启用监控模式 |
| `--scraper` | 启用数据采集器 |
| `--sysreport` | 生成系统资源报告 |
| `--tags` | 按标签过滤执行任务（逗号分隔） |
| `--interval` | 覆盖默认监控间隔（秒） |

## 报告生成

### API 监控报告

每次运行后，报告会保存到 `reports/` 目录：

- `report_YYYYMMDD_HHMMSS.csv` - 完整监控结果（管道符分隔）
- `report_YYYYMMDD_HHMMSS_error.csv` - 仅包含失败的监控任务（管道符分隔）

报告格式（PSV）：
```
id|desc|method|url|request_headers|request_body|tags|status|duration_s|expect_status|actual_status|actual_body|expect_body|pre_conditions|post_conditions|extracted_vars|start_time|end_time|diff
```

### 定期报告

根据 `monitor` 配置，系统会自动生成：

| 报告类型 | 配置项 | 说明 |
|---------|--------|------|
| 每日报告 | `daily_report` | 每天定时生成前一天的汇总报告 |
| 每周报告 | `weekly_report` | 每周生成汇总报告 |
| 每月报告 | `monthly_report` | 每月生成汇总报告 |
| 每年报告 | `yearly_report` | 每年生成汇总报告 |

报告生成时间由 `monitor.report_time` 控制（默认 `07:00`）。

### 系统监控报告

`sys_monitor` 模块会生成：
- ASCII 图表展示 CPU/内存/磁盘/网络使用情况
- 系统资源告警邮件（含设备名标识）

## 历史执行记录

监控执行完成后，系统会自动：

1. **记录执行时间**：每个监控任务执行时间会记录到 CSV 文件
2. **计算平均值**：自动计算每个监控任务的历史平均执行时间
3. **预估执行时间**：下次运行时根据历史数据预估总执行时间

CSV 文件存储在 `sql/` 目录，包含以下文件：
- `monitor_execution_times.csv` - 每次执行的详细记录
- `monitor_average_times.csv` - 各监控任务的平均执行时间

## 自动清理

系统会定期清理旧的日志和报告文件：

- 默认保留 30 天
- 每天自动清理一次
- 支持配置要清理的文件模式

## 依赖项

- `github.com/go-resty/resty/v2` - HTTP 客户端
- `github.com/spf13/viper` - 配置管理
- `github.com/spf13/cobra` - 命令行框架
- `github.com/tidwall/gjson` - JSON 解析（JSONPath 提取）
- `go.uber.org/zap` - 结构化日志
- `github.com/bmatcuk/doublestar/v4` - 文件模式匹配
- `github.com/shirou/gopsutil/v3` - 系统资源采集（CPU/内存/磁盘/网络）
- `encoding/csv` - 标准库 CSV 读写

## 许可证

MIT