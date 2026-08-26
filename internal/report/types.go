package report

import "time"

// ReportPeriod 报告周期类型，用于区分不同时间粒度的报告。
type ReportPeriod string

// 报告周期常量
const (
	PeriodStartup ReportPeriod = "startup" // 启动报告
	PeriodDaily   ReportPeriod = "daily"   // 每日报告
	PeriodWeekly  ReportPeriod = "weekly"  // 每周报告
	PeriodMonthly ReportPeriod = "monthly" // 每月报告
	PeriodYearly  ReportPeriod = "yearly"  // 年度报告
)

// PeriodNames 报告周期的中文名称映射，用于显示和日志。
var PeriodNames = map[ReportPeriod]string{
	PeriodStartup: "启动",
	PeriodDaily:   "每日",
	PeriodWeekly:  "每周",
	PeriodMonthly: "每月",
	PeriodYearly:  "年度",
}

// PeriodNamesEn 报告周期的英文名称映射，用于文件名和目录名。
var PeriodNamesEn = map[ReportPeriod]string{
	PeriodStartup: "startup",
	PeriodDaily:   "daily",
	PeriodWeekly:  "weekly",
	PeriodMonthly: "monthly",
	PeriodYearly:  "yearly",
}

// StartupTaskInfo 启动报告中的任务信息，记录单个监控任务的配置概要。
type StartupTaskInfo struct {
	ID       string // 任务唯一标识
	Desc     string // 任务描述
	Method   string // HTTP 方法
	URL      string // 监控目标 URL
	Interval int    // 执行间隔（秒）
}

// StartupInfo 启动报告所需的上下文信息，包含任务列表和并发配置。
type StartupInfo struct {
	Tasks      []StartupTaskInfo // 当前配置的所有监控任务
	MaxWorkers int               // 实际使用的最大并发数
}

// Report 完整的报告数据对象，聚合了监控、告警、资源指标等所有子系统的数据。
// 根据 Period 不同，部分字段可能为空（如年报不包含 HourlyMetrics）。
type Report struct {
	Period           ReportPeriod               // 报告周期类型
	StartDate        string                     // 报告起始日期（YYYY-MM-DD）
	EndDate          string                     // 报告结束日期（YYYY-MM-DD）
	TotalTasks       int                        // 报告周期内总请求数
	FailedTasks      int                        // 失败请求数
	SuccessTasks     int                        // 成功请求数
	InterfaceStats   []InterfaceStat            // 各接口的统计明细
	AggregatedErrors []AggregatedError          // 聚合后的告警错误列表
	SystemAlerts     []SystemAlertItem          // 系统资源告警列表
	ScraperAlerts    []ScraperAlertItem         // 采集器告警列表
	HourlyMetrics    []HourlyResourceMetric     // 小时级资源指标（仅日报使用）
	DailyMetrics     []DailyResourceMetric      // 日级资源指标（周报/月报使用）
	MonthlyMetrics   []MonthlyResourceMetric    // 月级资源指标（仅年报使用）
	SystemMetrics    *SystemMetricsSnapshot     // 系统资源快照（含趋势图表）
	GeneratedAt      time.Time                  // 报告生成时间
	startupInfo      *StartupInfo               // 启动信息（仅启动报告使用）
}

// HourlyResourceMetric 采集器目标的小时级资源指标，包含 24 个小时时段的平均值。
type HourlyResourceMetric struct {
	TargetName  string      // 采集器目标名称
	MetricName  string      // 指标原始名称
	MetricAlias string      // 指标显示别名
	Unit        string      // 单位
	HourlyData  []HourlyData // 24 小时数据
}

// DailyResourceMetric 采集器目标的日级资源指标，包含一周/一月内每天的平均值。
type DailyResourceMetric struct {
	TargetName  string     // 采集器目标名称
	MetricName  string     // 指标原始名称
	MetricAlias string     // 指标显示别名
	Unit        string     // 单位
	DailyData   []DailyData // 每日数据
}

// MonthlyResourceMetric 采集器目标的月级资源指标，包含 12 个月的平均值。
type MonthlyResourceMetric struct {
	TargetName  string       // 采集器目标名称
	MetricName  string       // 指标原始名称
	MetricAlias string       // 指标显示别名
	Unit        string       // 单位
	MonthlyData []MonthlyData // 12 个月数据
}

// HourlyData 单个小时的指标数据点。
type HourlyData struct {
	Hour     int     // 小时（0-23）
	AvgValue float64 // 该小时的平均值，-1 表示无数据
}

// DailyData 单个日期的指标数据点。
type DailyData struct {
	Day      int     // 天数索引（从 0 开始）
	DayLabel string  // 日期标签（如 "08-26"）
	AvgValue float64 // 当天的平均值，-1 表示无数据
}

// MonthlyData 单个月份的指标数据点。
type MonthlyData struct {
	Month      int     // 月份（1-12）
	MonthLabel string  // 月份标签（如 "8月"）
	AvgValue   float64 // 该月的平均值，-1 表示无数据
}

// InterfaceStat 单个接口的统计明细，包含请求次数、成功/失败数、耗时等。
type InterfaceStat struct {
	TaskID          string // 任务唯一标识
	TaskDesc        string // 任务描述
	URL             string // 监控目标 URL
	Method          string // HTTP 方法
	TotalCount      int    // 总请求数
	SuccessCount    int    // 成功数
	FailedCount     int    // 失败数
	AlertCount      int    // 告警次数
	TotalDurationMS int64  // 累计总耗时（毫秒）
	AvgDurationMS   int64  // 平均响应时间（毫秒）
	MaxDurationMS   int64  // 最大响应时间（毫秒）
	LastFailureTime string // 最近一次失败时间
}

// AggregatedError 按 (任务ID+URL) 聚合的错误记录，用于报告中的告警汇总区块。
type AggregatedError struct {
	TaskID          string    // 任务唯一标识
	TaskDesc        string    // 任务描述
	URL             string    // 监控目标 URL
	Method          string    // HTTP 方法
	ExpectedStatus  int       // 期望的 HTTP 状态码
	AlertLevel      string    // 告警级别（CRITICAL/WARNING）
	AlertCount      int       // 告警累计次数
	FirstOccurrence time.Time // 首次发生时间
	LastOccurrence  time.Time // 最近发生时间
	ErrorMsg        string    // 错误信息摘要
}

// SystemMetricsSnapshot 系统资源快照，包含当前实时指标、周期峰值以及 ASCII 趋势图表。
// 图表字段（CPUChart 等）由 loadSystemMetrics 填充，供模板直接渲染。
type SystemMetricsSnapshot struct {
	CPUPercent       float64               // 当前 CPU 使用率（%）
	CPUMaxPercent    float64               // 报告周期内 CPU 使用率峰值（%）
	MemoryPercent    float64               // 当前内存使用率（%）
	MemoryMaxPercent float64               // 报告周期内内存使用率峰值（%）
	DiskPercent      float64               // 当前磁盘使用率（%）
	NetDownKBps      float64               // 当前网络下行速率（KB/s）
	NetUpKBps        float64               // 当前网络上行速率（KB/s）
	NetDownMaxKBps   float64               // 报告周期内网络下行峰值（KB/s）
	NetUpMaxKBps     float64               // 报告周期内网络上行峰值（KB/s）
	MemUsedBytes     uint64                // 内存已用字节数
	MemTotalBytes    uint64                // 内存总字节数
	DiskUsedBytes    uint64                // 磁盘已用字节数
	DiskTotalBytes   uint64                // 磁盘总字节数
	CPUChart         string                // CPU 平均使用率趋势图
	CPUMaxChart      string                // CPU 最高使用率趋势图
	MemoryChart      string                // 内存平均使用率趋势图
	MemoryMaxChart   string                // 内存最高使用率趋势图
	DiskChart        string                // 磁盘使用率趋势图
	NetDownChart     string                // 网络下行平均速率趋势图
	NetUpChart       string                // 网络上行平均速率趋势图
	NetDownMaxChart  string                // 网络下行最高速率趋势图
	NetUpMaxChart    string                // 网络上行最高速率趋势图
	StartTime        string                // 图表起始时间
	EndTime          string                // 图表结束时间
	Partitions       []PartitionInfo       // 磁盘分区信息列表
}

// PartitionInfo 单个磁盘分区的使用情况。
type PartitionInfo struct {
	MountPoint string  // 挂载点（如 "/", "D:"）
	Fstype     string  // 文件系统类型（如 "ext4", "NTFS"）
	Percent    float64 // 使用率（%）
	UsedBytes  uint64  // 已用空间（字节）
	TotalBytes uint64  // 总空间（字节）
}

// SystemAlertItem 系统资源告警记录，描述一次 CPU/内存/磁盘等系统指标的告警事件。
type SystemAlertItem struct {
	Metric          string // 指标名称（如 "cpu_usage"）
	MetricAlias     string // 指标显示别名
	Value           float64 // 告警时的指标值
	Threshold       float64 // 告警阈值
	Unit            string // 单位
	AlertLevel      string // 告警级别（CRITICAL/WARNING）
	AlertCount      int64  // 告警累计次数
	FirstOccurrence string // 首次告警时间
	LastOccurrence  string // 最近告警时间
	Message         string // 告警消息
}

// ScraperAlertItem 采集器告警记录，描述一次采集器指标（如响应时间、可用性）的告警事件。
type ScraperAlertItem struct {
	TargetName      string // 采集器目标名称
	TargetURL       string // 采集器目标 URL
	MetricName      string // 指标名称
	MetricAlias     string // 指标显示别名
	Value           float64 // 告警时的指标值
	Threshold       float64 // 告警阈值
	Unit            string // 单位
	AlertLevel      string // 告警级别（CRITICAL/WARNING）
	AlertCount      int64  // 告警累计次数
	FirstOccurrence string // 首次告警时间
	LastOccurrence  string // 最近告警时间
	Message         string // 告警消息
}