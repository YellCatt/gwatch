# 三、测试执行引擎（`internal/testcase/`）

## 3.1 `executor.go`

| 函数 | 作用 |
|------|------|
| `executePreConditions(preIDs []string) (TestResult, error)` | 顺序执行前置用例，任一失败则中断并返回错误；输出彩色控制台日志。 |
| `executePostConditions(postIDs []string)` | 顺序执行后置用例，失败只告警不中断主流程。 |
| `finishTestCase(tc, result, startTime) TestResult` | 统一收尾：计算耗时、打印 PASS/FAIL、调用 `storage.RecordExecutionTime` 异步落盘、执行后置条件、按规则清理提取的变量、等待 `DelayAfterMs`。 |
| `ExecuteTestCase(tc TestCase) TestResult` | 核心执行函数。处理顺序：Skip → 前置条件 → 变量替换（URL/Headers/Body/JSON）→ 组装请求体（JSON/Form/Body/Payload 按优先级）→ HTTP 请求 → 状态码校验 → 正则/JSON 断言 → 变量提取 → `finishTestCase`。 |
| `executeStreamAssert(tc, resp, startTime) TestResult` | 针对流式响应：用 `bufio.Scanner` 按行读取 `data:` 前缀，解析 `choices[0].delta.content` 聚合文本，再调用 `assert.StreamAssert` 或 `assert.JSONMatch`。 |
| `hasFileField(form map[string]string) bool` | 判断 Form 中是否有以 `@` 或 `file://` 开头的文件字段。 |

## 3.2 `runner.go` —— 一次性测试入口

| 函数 | 作用 |
|------|------|
| `RunTests(paths []string, tags []string)` | 完整的一次性测试流程：初始化 CSV 存储 → 解析 PSV → `SetAllTestCases` → 标签过滤 → 计算预估耗时 → 执行全局前置 → 循环执行 `ExecuteTestCase`（每执行一个增量保存 PSV 报告）→ 打印汇总 → 计算平均耗时 → 全局后置 → 清理变量 → 失败则 `os.Exit(1)`。 |
| `PrintTaskSummary(...)` | 打印漂亮的任务统计横幅（解析数、链式/独立任务数、过滤后数、预估耗时）。 |
| `CalculateEstimatedDuration(testCases) time.Duration` | 从 storage 读取历史平均耗时累加；无历史记录时用全量平均值估算。 |
| `FormatDuration(d) string` | 按 ms/s/m/h 自动选择最合适的时长格式。 |

## 3.3 `assert/assert.go` —— 断言引擎

| 函数 | 作用 |
|------|------|
| `CompactBody(body) string` | 优先用 `json.Compact` 压缩为单行；否则手工去除换行。 |
| `BodyRegexMatch(body, pattern) (bool, string)` | 支持 `!` 前缀取反的正则匹配。 |
| `JSONMatch(expected, actual, matchMode) (bool, string)` | 选择 `jsonExactMatch` 或 `jsonSubsetMatch`。 |
| `jsonExactMatch` | 键数和值完全相等。 |
| `jsonSubsetMatch` | 期望 JSON 是实际 JSON 的子集，支持 `{{not_exists}}`。 |
| `compareValues(expected, actual) (bool, string)` | 支持 `{{skip}}`、`{{regex:...}}`、`{{not_regex:...}}`、`{{not_exists}}` 四种特殊指令。 |
| `StreamAssert / checkStreamAssert` | 支持 contains / regex / json_path 三种流式断言，要求 `chunkCount >= MinChunks`。 |
| `ExtractVariables(responseBody, extractExpr) (map[string]string, error)` | 解析 `key=jsonpath` 表达式，用 `gjson.Get` 取值。 |
| `fixRegexEscapes(pattern) string` | 修复 JSON 解析过程中丢失的反斜杠（`\d` 变成 `d`）。 |
| `validateRegexPattern(pattern) (string, error)` | 为以 `*+?{` 开头的 pattern 自动加 `.` 前缀，避免语法错误。 |
| `BuildAggregatedResult(content, chunkCount) string` | 把流式聚合结果包装成 JSON 字符串返回。 |

### 作者思考

断言的可扩展性是通过"字符串指令"实现的，例如 `{{regex:\d+}}`、`{{skip}}`、`{{not_exists}}`，用户只需在 PSV 文件里写字符串即可启用高级断言，不需要引入新的语法或 schema。

## 3.4 `vars/vars.go` —— 全局变量

| 函数 | 作用 |
|------|------|
| `Set(key, value)` / `Get(key)` / `GetAll()` / `Delete(key)` | 线程安全的变量增删查。 |
| `Replace(text) string` | 用正则 `\{\{([a-zA-Z][a-zA-Z0-9_]*)\}\}` 匹配并替换变量引用；长值自动脱敏。 |
| `InitFromConfig(config map[string]string)` | 从配置文件注入默认变量。 |
| `MarkAsGlobalPre(key)` / `CleanupGlobalPreVariables()` | 标记并清理全局前置脚本提取的变量，保证跨用例链路可复用。 |

## 3.5 `httpclient/client.go`

| 函数 | 作用 |
|------|------|
| `InitClient()` | 使用 `resty.New()` 配置 BaseURL、超时、重试次数、重试等待时间、可选的 TLS 跳过验证。 |