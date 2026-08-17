# PSV 测试用例详解

## 概述

PSV（Pipe-Separated Values）是 gwatch 定义接口测试用例的文件格式，使用管道符 `|` 作为字段分隔符。支持 `.psv` 和 `.csv` 两种扩展名。

## 文件格式

```
id|desc|method|url|expected_status|headers|body|expected_body|tags|extract|...
```

首行为列名（header），后续行为测试用例数据。

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 用例唯一标识 |
| `desc` | string | 是 | 用例描述 |
| `method` | string | 是 | HTTP 方法：GET/POST/PUT/DELETE/PATCH/HEAD |
| `url` | string | 是 | 请求 URL（支持 `{{变量}}` 替换） |
| `endpoint` | string | 否 | API 端点（兼容字段） |
| `expected_status` | int | 否 | 期望 HTTP 状态码 |
| `expected_code` | int | 否 | 期望状态码（兼容字段） |
| `expected_body` | string | 否 | 期望响应体（JSON 或正则） |
| `headers` | JSON | 否 | 请求头，JSON 格式 |
| `params` | JSON | 否 | URL 查询参数，JSON 格式 |
| `form` | JSON | 否 | 表单数据，JSON 格式 |
| `json` | string | 否 | JSON 请求体 |
| `body` | string | 否 | 原始请求体 |
| `payload` | string | 否 | 兼容字段，等同于 body |
| `tags` | string | 否 | 标签列表，逗号分隔，用于过滤 |
| `extract` | string | 否 | 变量提取规则，格式：`变量名=JSONPath` |
| `pre` | string | 否 | 前置条件，逗号分隔的用例 ID |
| `post` | string | 否 | 后置条件，逗号分隔的用例 ID |
| `fail_mode` | string | 否 | 失败模式：`stop`（默认）/ `continue` |
| `match_mode` | string | 否 | JSON 匹配模式：`exact`（精确）/ `subset`（子集） |
| `body_regex` | string | 否 | 响应体正则断言，支持 `!` 前缀取反 |
| `skip` | bool | 否 | 是否跳过该用例 |
| `skip_reason` | string | 否 | 跳过原因 |
| `delay_ms` | int | 否 | 执行前延迟（毫秒） |
| `delay_after_ms` | int | 否 | 执行后延迟（毫秒） |
| `keep_vars` | bool | 否 | 是否保留提取的变量（默认 false） |

### 监控专用字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `monitor_enabled` | bool | 是否启用持续监控 |
| `monitor_interval` | int | 监控周期（秒），默认 60 |
| `response_threshold` | int | 慢响应阈值（毫秒） |
| `alert_on_failure` | bool | 失败时是否发送告警邮件 |
| `alert_on_slow` | bool | 慢响应时是否发送告警邮件 |
| `stream_mode` | bool | 是否为流式（SSE）模式 |
| `stream_assert` | JSON | 流式断言规则列表 |

## 变量提取与替换

### 提取

在 `extract` 字段中定义提取规则，用 `=` 连接变量名和 JSONPath：

```
user_id=$.data.user.id,token=$.data.token
```

### 替换

在 URL、Headers、Body 等字段中使用 `{{变量名}}` 引用已提取的变量：

```
url: https://api.example.com/user/{{user_id}}
headers: {"Authorization": "Bearer {{token}}"}
```

## 断言类型

| 类型 | 说明 |
|------|------|
| 状态码断言 | `expected_status` 字段 |
| JSON 精确匹配 | `expected_body` + `match_mode: exact` |
| JSON 子集匹配 | `expected_body` + `match_mode: subset`（只比对 expected 中的键值） |
| 正则匹配 | `body_regex` 字段，支持 `!` 前缀取反 |
| 流式断言 | `stream_assert` 数组，支持 contains/regex/json_path 三种类型 |

## 前置/后置条件

- **前置条件（pre）**：执行当前用例前先执行指定的依赖用例，任一失败则当前用例失败
- **后置条件（post）**：执行当前用例后执行指定用例，失败仅记录警告不中断
- **全局前置/后置**：在 `config.yaml` 中配置 `app.global_pre` / `app.global_post`，作用于所有用例

## 标签过滤

通过 `--tags` 参数可按标签过滤执行：

```bash
# 只运行包含 smoke 或 regression 标签的用例
gwatch -t cases/ --tags smoke,regression
```

## PSV 示例

```
id|desc|method|url|expected_status|headers|extract|monitor_enabled|monitor_interval|response_threshold|alert_on_failure|alert_on_slow
login|用户登录|POST|https://api.example.com/login|200|{"Content-Type":"application/json"}|token=$.data.access_token|true|60|3000|true|true
get_user|获取用户信息|GET|https://api.example.com/user/{{token}}|200|{"Authorization":"Bearer {{token}}"}|user_id=$.data.id|true|60|2000|true|true
```

## 执行模式

### 一次性测试模式

```bash
gwatch -t cases/
```

按 PSV 文件顺序串行执行所有用例，适用于 CI/CD 集成。

### 持续监控模式

```bash
gwatch cases/
```

默认模式，为每个 `monitor_enabled: true` 的用例启动定时调度器，持续执行并触发告警。