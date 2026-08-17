# 九、数据持久化层（`internal/storage/`）

## 9.1 `init.go` —— 通用初始化与工具

| 函数 | 作用 |
|------|------|
| `InitDB(dir) error` | `sync.Once` 包装，调用 `initCSVInternal`。 |
| `initCSVInternal(dir) error` | 创建目录并初始化 8 个 CSV 文件（执行/平均/监控/监控汇总/告警汇总/采集指标/系统告警/采集告警），最后生成 `csv_index.csv` 总表。 |
| `csvFileMetas() []csvFileMeta` | 返回所有 CSV 的元数据（文件名、描述、写入方式、表头）。 |
| `writeIndexCSV() error` | 写入带 UTF-8 BOM 的索引文件。 |
| 各 `*CSVPath()` | 返回对应 CSV 的绝对路径。 |
| `ensureCSV(path, header) error` | 文件不存在或为空时创建并写入表头。 |
| `readRecords(path) ([]string, [][]string, error)` | 读取全部记录，返回表头和数据行。 |
| `appendRecord(path, record) error` | 追加一行。 |
| `writeRecords(path, header, records) error` | 覆盖重写整个文件（用于汇总更新）。 |
| `alertLevelRank(level) int` | `CRITICAL=2, WARNING=1, 其他=0`，用于比较告警级别。 |
| `parseAlertLevel(level) string` | 空值默认 `CRITICAL`。 |
| `parseSuccess` / `parseInt64` / `parseFloat64` | CSV 字段解析辅助。 |

## 9.2 `record_monitor.go`

| 函数 | 作用 |
|------|------|
| `RecordMonitorResult(record) error` | 追加写入监控明细。 |
| `GetMonitorResultsByDate(date) ([]MonitorResultRecord, error)` | 按日期读取明细。 |
| `UpdateMonitorSummary(record) error` | 增量更新 `monitor_summary.csv`（按 `date+test_case_id` 聚合 Total/Success/Failed/Min/Max 等）。 |
| `GetMonitorSummaryByDate(date)` / `GetMonitorSummaryByPeriod(start, end)` | 读取汇总，支持跨天聚合。 |

## 9.3 `record_alert.go`

| 函数 | 作用 |
|------|------|
| `UpdateAlertSummary(record) error` | 增量更新告警汇总；`upgradeAlertSummaryRecords` 做历史字段升级。 |
| `GetAlertSummaryByDate(date)` / `GetAlertSummaryByPeriod(start, end)` | 读取告警汇总，跨天聚合。 |

## 9.4 `record_scraper.go` / `record_sysalert.go` / `record_execution.go`

| 函数 | 作用 |
|------|------|
| `RecordExecutionTime` | 追加每次测试/监控的执行耗时，用于计算平均值与预估执行时间。 |
| `CalculateAndStoreAverages` / `GetAllAverageDurations` | 基于执行记录重新计算每个 URL 的平均耗时。 |
| `RecordScraperMetric` / `UpdateScraperAlertSummary` | 采集器指标与告警落盘。 |
| `UpdateSystemAlertSummary` | 系统告警落盘。 |

### 作者思考

"CSV 作为数据库"是一个权衡后的选择——对单机/中小规模场景，零依赖、可直读、易迁移；作者通过"追加明细 + 覆盖汇总"的双写模式（append + rewrite），实现了增量聚合，避免每次全量重算。