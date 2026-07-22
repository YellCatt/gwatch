# pipet

一个功能强大的企业级 API 测试工具，使用 Go 语言编写。

## 功能特性

- RESTful API 测试
- PSV（管道分隔值）测试用例管理
- YAML 配置管理
- Zap 结构化日志
- 串行测试执行（确保依赖顺序）
- 测试报告生成
- 基于标签的测试过滤
- 变量提取和替换
- 流式（SSE）断言支持
- 正则表达式断言支持（支持匹配数字、布尔等非字符串类型）
- CSV 历史执行时间存储和平均值计算
- 测试用例延迟控制（执行前/后延迟）
- 邮件测试报告通知

## 环境要求

- Go 1.22+

## 安装

### 从源码编译

```bash
go mod download
go build -ldflags="-s -w" -o pipet.exe
```

### 预编译二进制

通过 GitHub Actions 自动构建，支持以下平台：

| 平台 | 架构 | 下载链接 |
|------|------|----------|
| Linux | amd64 | [pipet_linux_amd64](https://github.com/YellCatt/pipetGo/releases/download/dev-latest/pipet_linux_amd64) |
| Linux | arm64 | [pipet_linux_arm64](https://github.com/YellCatt/pipetGo/releases/download/dev-latest/pipet_linux_arm64) |
| Linux | mipsle | [pipet_linux_mipsle](https://github.com/YellCatt/pipetGo/releases/download/dev-latest/pipet_linux_mipsle) |
| Windows | amd64 | [pipet_windows_amd64.exe](https://github.com/YellCatt/pipetGo/releases/download/dev-latest/pipet_windows_amd64.exe) |

**嵌入式设备支持**：mipsle 版本采用静态编译（`CGO_ENABLED=0`），零系统依赖，可直接运行在极路由2等嵌入式设备上。

## 运行时文件结构

编译后的 `pipet.exe` 运行时需要以下文件结构：

```
pipet.exe           # 可执行文件
config/             # 配置目录
  └── config.yaml   # 配置文件
testcases/          # 测试用例目录（可选）
  └── *.psv         # PSV/CSV 测试用例文件
reports/            # 报告输出目录（自动创建）
sql/                # CSV 数据目录（自动创建）
```


## 配置

编辑 `config/config.yaml` 文件：

```yaml
target:
  base_url: "https://httpbin.org"
  timeout: 30

log:
  level: "info"
  encoding: "json"
  output: "stdout"

test:
  report_dir: "./reports"
  test_case_dir: "./testcases"
  data_dir: "./sql"

email:
  from: "sender@example.com"
  to: "recipient@example.com"
  auth_code: "your_auth_code"
  smtp_server: "smtp.example.com"
  smtp_port: 465
```

### 必需文件

| 文件/目录 | 说明 | 是否必需 |
|-----------|------|----------|
| `pipet.exe` | 主程序可执行文件 | **是** |
| `config/config.yaml` | 配置文件 | **是** |
| `testcases/` | 测试用例目录 | 否（运行时指定路径则不需要） |
| `reports/` | 报告输出目录 | 否（自动创建） |
| `sql/` | CSV 数据目录 | 否（自动创建） |


### 配置说明

- **base_url**: API 目标地址，测试用例中可用 `{{base_url}}` 引用
- **timeout**: 请求超时时间（秒）
- **log.level**: 日志级别（debug, info, warn, error）
- **log.encoding**: 日志格式（json, console）
- **log.output**: 日志输出（stdout 或文件路径）
- **test.report_dir**: 测试报告输出目录
- **test.test_case_dir**: 默认测试用例目录
- **test.data_dir**: CSV 数据存储目录

- **email**: 邮件通知配置（测试开始和结束时发送）

## 使用方法

### 运行默认目录下的所有测试

```bash
./pipet.exe
```

### 运行特定的 PSV 文件

```bash
./pipet.exe tests/test_data.psv tests/test_data2.psv
```

### 运行目录下的所有测试

```bash
./pipet.exe tests
```

### 标签过滤

```bash
# 只运行 smoke 测试
./pipet.exe --tags=smoke

# 运行 smoke 和 api 测试
./pipet.exe --tags=smoke,api
```

## PSV 测试用例格式

```psv
id|skip|desc|method|url|headers|params|form|json|body|expected_status|expected_body|tags|extract|stream_mode|stream_assert|match_mode|body_regex|pre|post
```

### 列说明

| 列名 | 描述 |
|------|------|
| `id` | 测试用例唯一标识 |
| `skip` | 是否跳过测试（0/1 或 true/false） |
| `desc` | 测试用例描述 |
| `method` | HTTP 方法（GET, POST, PUT, DELETE, PATCH, HEAD） |
| `url` | API 端点 URL |
| `headers` | 请求头（JSON 对象） |
| `params` | URL 查询参数 |
| `form` | 表单数据 |
| `json` | JSON 请求体 |
| `body` | 原始请求体 |
| `payload` | 兼容性字段，用于原始请求体 |
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
| `fail_mode` | 失败模式：`stop`（默认，前置条件失败则停止）或 `continue`（前置条件失败仍继续） |
| `keep_vars` | 是否保留提取的变量（0/1，默认 0，执行后自动清理） |
| `delay_ms` | 执行前延迟时间（毫秒），用于控制测试用例执行前的等待时间 |
| `delay_after_ms` | 执行后延迟时间（毫秒），用于控制当前用例执行完后到下一个用例开始的间隔 |

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

## 报告生成

每次运行后，报告会保存到 `reports/` 目录：

- `report_YYYYMMDD_HHMMSS.csv` - 完整测试结果（管道符分隔）
- `report_YYYYMMDD_HHMMSS_error.csv` - 仅包含失败的测试用例（管道符分隔）

报告格式（PSV）：
```
id|desc|method|url|request_headers|request_body|tags|status|duration_s|expect_status|actual_status|actual_body|expect_body|pre_conditions|post_conditions|extracted_vars|start_time|end_time|diff
```

## 历史执行记录

测试执行完成后，系统会自动：

1. **记录执行时间**：每个测试用例执行时间会记录到 CSV 文件
2. **计算平均值**：自动计算每个测试用例的历史平均执行时间
3. **预估执行时间**：下次运行时根据历史数据预估总执行时间

CSV 文件存储在 `sql/` 目录，包含以下文件：
- `test_execution_times.csv` - 每次执行的详细记录
- `test_average_times.csv` - 各测试用例的平均执行时间


## 依赖项

- `github.com/go-resty/resty/v2` - HTTP 客户端
- `github.com/spf13/viper` - 配置管理
- `github.com/tidwall/gjson` - JSON 解析
- `go.uber.org/zap` - 日志
- `github.com/bmatcuk/doublestar/v4` - 文件匹配
- `encoding/csv` - 标准库 CSV 读写


## 许可证

MIT