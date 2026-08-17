# 五、远程资源采集器（`internal/scraper/loop.go`）

| 函数 | 作用 |
|------|------|
| `StartLoop()` | 入口：检查配置启用和 targets 非空 → 初始化 stopChan → 进入死循环，对每个 target 组装 `TargetConfig` + `MetricConfig` 列表 → 调用 `Scrape`。 |
| `StopLoop()` | 关闭 stopChan 终止循环。 |
| `ProbeURL(targetURL)` | 调试辅助：探测一个 URL，打印 HTTP 状态码、响应体、JSON 树，并给出采集器配置建议（metrics 路径）。 |
| `printMetricSuggestions(data, path)` | 递归遍历 JSON 树，为每个叶子节点输出 name/path/alias/unit 配置建议。 |
| `buildAlertMessage(name, metric, isSpeed) string` | 按单位类型（速度/普通）生成易读的告警文本。 |
| `getOpMessageDesc(op) string` | 将 `gt/lt/ge/le/eq` 转为人类可读的"超过阈值""低于阈值"等描述。 |

### 作者思考

采集器和 PSV 测试的差异在于"数据源"——PSV 面向"断言式测试"，采集器面向"指标式采样"。两者在 HTTP 请求层面可以共享 resty，但在解析层面必须分开（一个比对响应，一个抽取数值）。