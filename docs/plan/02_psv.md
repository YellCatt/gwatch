# 二、PSV 测试用例解析（`internal/psv/psv.go`）

## 2.1 数据结构

| 结构体/字段 | 作用 |
|-------------|------|
| `TestCase` | 测试用例的完整内存表示，包含 ID、URL、Method、Headers、Params、Form、JSON、Body、ExpectedStatus、ExpectedBody、Tags、Extract、StreamMode、StreamAssert、Pre/Post 前置/后置、FailMode、DelayMs、MonitorEnabled、MonitorInterval、ResponseThreshold、AlertOnFailure、AlertOnSlow 等。 |
| `StreamAssert` | 流式断言配置：`Kind`（contains/regex/json_path）、`Pattern`、`MaxWaitMs`、`MinChunks`。 |

## 2.2 解析函数

| 函数 | 作用 |
|------|------|
| `ParseFile(filePath string) ([]TestCase, error)` | 打开单个 `.psv`/`.csv` 文件并解析为 `TestCase` 列表。 |
| `ParseFiles(paths []string) ([]TestCase, error)` | 对多个路径调用 `expandPath` + `ParseFile`，聚合成一个列表。 |
| `expandPath(path string) ([]string, error)` | 判断是文件还是目录；目录使用 `doublestar.Glob` 递归查找 `**/*.{psv,csv}`。 |
| `parseReader(reader io.Reader, filePath string) ([]TestCase, error)` | 逐行扫描：跳过空行和 `#` 注释；第 1 行作为表头；后续行调用 `parseLine`/`parseTestCase`。 |
| `parseLine(line string) []string` | 使用 `encoding/csv` 按 `\|` 分隔，并启用 `LazyQuotes` 处理 JSON 内嵌引号；解析失败时回退为 `strings.Split`。 |
| `parseTestCase(header, fields) (TestCase, error)` | 遍历表头-字段对，用 switch 映射各字段；监控字段从 `config.GlobalConfig.Monitor` 取默认值。 |
| `parseKeyValueMap(str) map[string]string` | 支持 JSON 对象、URL 查询字符串两种格式解析 Headers/Params/Form。 |
| `parseTags(str) []string` | 按逗号切分并 TrimSpace。 |
| `parseDelimited(str, delimiter) []string` | 按指定分隔符切分（用于 `Pre`/`Post`）。 |
| `parseInt(s) int` | 用正则 `\d+` 提取整数，兼容字符串中夹带文本的场景。 |
| `generateID(tc) string` | 根据 method + URL 生成稳定 ID（`get_/api_user` 这种格式）。 |

### 作者思考

用"表头驱动"代替"列号驱动"，这样 PSV 文件列顺序可以任意调整；同时兼容 JSON 对象和 URL 查询字符串的 Headers 写法，降低用户心智负担。