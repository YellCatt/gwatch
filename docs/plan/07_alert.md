# 七、统一告警调度（`internal/email/dispatcher.go`）

## 7.1 核心数据结构

| 结构体 | 作用 |
|--------|------|
| `UnifiedAlert` | 统一告警载体：Source、SourceName、TargetName、MetricName、MetricAlias、Value、Unit、Threshold、AlertLevel、Message、Timestamp。 |
| `AlertSource` 常量 | `SourceAPI`、`SourceScraper`、`SourceSystem` 三种来源。 |

## 7.2 调度函数

| 函数 | 作用 |
|------|------|
| `DispatchAlert(alert)` | 入口：首次调用启动 `alertDispatcherLoop` 协程，将告警推入带缓冲的 `alertChan`（容量 200）。 |
| `alertDispatcherLoop()` | 30 秒 Ticker 触发 `flushAndSend`；`alertChan` 关闭时最后一次 flush 后退出。 |
| `flushAndSend()` | 取出全部 `collectedAlerts` → `filterByCooldown` 冷却过滤 → `sendUnifiedAlertEmail`。 |
| `filterByCooldown(alerts) []UnifiedAlert` | 以 `source:target:metric` 为 key，在冷却期内（API/Scraper 6h、System 2h）的告警被抑制。 |
| `getCooldownForSource(source) time.Duration` | 从配置读取冷却时间，未配置使用默认值。 |
| `sendUnifiedAlertEmail(alerts) error` | 统计 CRITICAL/WARNING 数量 → `groupBySource` 分组 → `RenderUnifiedAlertBody` 渲染 → `BuildUnifiedAlertSubject` 生成主题 → `SendEmail`。 |
| `groupBySource(alerts) []alertGroup` | 按预设顺序（API→Scraper→System）分组，保证邮件阅读顺序稳定。 |
| `CloseDispatcher()` | 关闭 `alertChan` 触发调度协程退出。 |

### 作者思考

用"通道 + 批量 + 冷却"三板斧抑制告警风暴，而不是在每个业务模块里各自判断要不要发邮件，保证告警策略集中在一处可配置。