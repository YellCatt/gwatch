# 六、系统资源监控（`internal/sysmon/`）

## 6.1 `collector.go`

| 函数 | 作用 |
|------|------|
| `CollectMetrics() (SystemMetric, error)` | 通过 `gopsutil` 采集 CPU、内存、磁盘百分比，调用 `calcNetworkSpeed`/`calcDiskSpeed` 计算 I/O 速度，组装 `SystemMetric` 返回。 |
| `calcNetworkSpeed() netSpeedResult` | 读取两次 `net.IOCounters`（间隔 1 秒），差值得到上/下行速率（KB/s）。 |
| `calcDiskSpeed() diskSpeedResult` | 读取两次 `disk.IOCounters`（间隔 1 秒），差值得到读/写速率（KB/s）。 |
| `GetHostInfo() (string, string)` | 返回主机名和操作系统平台。 |

## 6.2 `sysmon.go`

| 函数 | 作用 |
|------|------|
| `StartSystemMonitor()` | 调用 `setupSystemMonitor`，随后阻塞等待 `stopSysMon`。 |
| `setupSystemMonitor() bool` | 检查启用 → `EnsureStorage` → 初始化历史缓冲区 → `backfillAggregatedMetrics` → 启动 `collectLoop` → 打印配置横幅。 |
| `StopSystemMonitor()` | 关闭 `stopSysMon` 触发 `collectLoop` 退出。 |
| `collectLoop(interval)` | 核心循环：按 Ticker 调用 `CollectMetrics` → 追加历史 → 维护 `hourlyAgg` → 跨小时时落盘+触发日/月/年聚合 → 调用 `CheckAlerts` → `DispatchSystemAlerts`。 |
| `aggregateUpperTiers(prevHour, currentHour)` | 根据跨日/跨月/跨年触发对应聚合。 |
| `aggregateDay/day/month/year(...)` | 从低一级存储加载数据，聚合成高一级平均值并写入。 |
| `backfillAggregatedMetrics()` / `backfillDays` / `backfillMonths` / `backfillYears` | 启动时回填历史缺失的聚合记录。 |
| `flushHourlyAgg()` | 停止时把当前小时的聚合刷盘，避免丢失。 |
| `addHistory(metric)` / `GetHistory()` | 维护上限 `maxHistory=600` 的内存环形缓冲。 |
| `printSystemMonitorInfo(interval)` / `PrintCurrentStatus()` | 打印启动横幅和实时状态。 |
| `GenerateAndSaveReport() (string, error)` | 便捷函数：加载 24 小时数据 → 检查告警 → `SaveSystemReport`。 |

## 6.3 `alert.go`

| 函数 | 作用 |
|------|------|
| `CheckAlerts(metric) []AlertItem` | 对照 CPU/内存/磁盘/网络阈值生成告警列表，支持 WARNING 和 CRITICAL 两级。 |
| `DispatchSystemAlerts(alerts)` | 转换为 `UnifiedAlert` 调用 `email.DispatchAlert`，同时 `storage.UpdateSystemAlertSummary`。 |
| `SendSystemStatusEmail(metrics)` | 启动/停止时发送系统状态邮件，包含阈值、功能状态、报告正文。 |
| `formatSpeed(kbps) string` | 委托给 `util.FormatSpeed`。 |

## 6.4 `storage.go`

| 函数 | 作用 |
|------|------|
| `hourlyPath/dailyPath/monthlyPath/yearlyPath()` | 返回各级 CSV 文件路径。 |
| `InitStorage()` | 初始化 4 个 CSV 文件。 |
| `recordMetric(path, metric, sampleCount)` | 线程安全地追加写入一条系统指标记录。 |
| `RecordHourlyMetric/DailyMetric/MonthlyMetric/YearlyMetric` | 各级写入便捷方法。 |
| `loadMetrics(path, since) ([]SystemMetric, error)` | 按时间过滤读取历史指标。 |
| `LoadRecentMetrics(hours)` / `LoadDailyMetrics` / `LoadMonthlyMetrics` / `LoadYearlyMetrics` | 各级读取便捷方法。 |
| `aggregateMetrics(metrics) (SystemMetric, int)` | 用 `hourlyAggregator` 计算平均。 |
| `aggregateAndRecord(path, metrics)` | 聚合并写入指定 CSV。 |
| `EnsureStorage()` | 保证目录和小时级 CSV 存在。 |

## 6.5 `chart.go`

| 函数 | 作用 |
|------|------|
| `GenerateASCIIChart(data, width, unit, thresholds...) string` | 委托给带时间版本。 |
| `GenerateASCIIChartWithTime(...) string` | 将数据按宽度分桶求均值，绘制 `█/░` 柱状图，超阈值加 `⚠️`，附带时间轴。 |
| `GenerateSystemReport(metrics, alerts) string` | 生成完整的系统报告文本（主机信息、当前状态、CPU/内存/磁盘/网络趋势图、告警摘要）。 |
| `generateTimeLabels` / `formatHourLabel` / `extractField` | 图表辅助函数。 |
| `SaveSystemReport(metrics, alerts) (string, error)` | 保存 `system_monitor_<timestamp>.txt` 到 `reports/system/`。 |

### 作者思考

系统监控采用"小时采样 → 日/月/年聚合"的分层聚合模型，这样既能保留细粒度数据用于诊断，又能通过聚合文件支撑长期趋势分析；启动时的回填机制确保数据不会因为停机而出现断层。