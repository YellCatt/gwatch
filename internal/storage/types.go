// Package storage 负责将运行时产生的监控数据、采集结果、告警等信息
// 持久化到本地 CSV 文件，便于后续生成各类报告或离线分析。
package storage

import "time"

const (
	// AlertLevelCritical 严重告警级别
	AlertLevelCritical = "CRITICAL"
	// AlertLevelWarning 警告级别
	AlertLevelWarning = "WARNING"
)

// MonitorResultRecord 单次接口监控的原始执行结果记录。
type MonitorResultRecord struct {
	TestCaseID     string    // 用例 ID
	TestCaseDesc   string    // 用例描述
	URL            string    // 实际请求 URL
	Method         string    // HTTP 方法
	ExpectedStatus int       // 期望状态码
	ActualStatus   int       // 实际状态码
	ExpectedBody   string    // 期望响应体（或匹配规则）
	ActualBody     string    // 实际响应体摘要
	ErrorMsg       string    // 错误信息
	DurationMS     int64     // 本次执行耗时（毫秒）
	Success        bool      // 是否成功
	AlertType      string    // 触发的告警类型（failure/slow，未触发为空）
	Timestamp      time.Time // 记录时间
}

// MonitorSummaryRecord 按日期 + 用例维度聚合的每日监控统计。
type MonitorSummaryRecord struct {
	Date            string // 统计日期（YYYY-MM-DD）
	TestCaseID      string // 用例 ID
	TestCaseDesc    string // 用例描述
	URL             string // URL 摘要
	Method          string // HTTP 方法
	TotalCount      int64  // 当日总执行次数
	SuccessCount    int64  // 成功次数
	FailedCount     int64  // 失败次数
	TotalDurationMS int64  // 累计耗时（毫秒）
	MinDurationMS   int64  // 最小耗时
	MaxDurationMS   int64  // 最大耗时
	LastSuccessTime string // 最近一次成功时间
	LastFailureTime string // 最近一次失败时间
}

// AlertSummaryRecord 按日期 + 用例维度聚合的告警统计。
type AlertSummaryRecord struct {
	Date            string // 统计日期
	TestCaseID      string // 用例 ID
	TestCaseDesc    string // 用例描述
	URL             string // URL 摘要
	Method          string // HTTP 方法
	ExpectedStatus  int    // 期望状态码
	AlertLevel      string // 告警级别
	AlertCount      int64  // 告警次数
	FirstOccurrence string // 首次告警时间
	LastOccurrence  string // 最近告警时间
	ErrorMsg        string // 最近一次告警的错误信息
}

// ScraperMetricRecord 单次采集的指标原始记录。
type ScraperMetricRecord struct {
	TargetName  string    // 目标名
	TargetURL   string    // 目标 URL
	MetricName  string    // 指标内部名
	MetricAlias string    // 指标别名
	Value       float64   // 指标值
	Unit        string    // 单位
	Success     bool      // 本次采集是否成功
	Timestamp   time.Time // 采集时间
}

// ScraperMetricHourlyAvg 按小时聚合的指标平均值（用于图表绘制）。
type ScraperMetricHourlyAvg struct {
	TargetName  string  // 目标名
	MetricName  string  // 指标名
	MetricAlias string  // 指标别名
	Unit        string  // 单位
	Hour        int     // 小时（0-23）
	AvgValue    float64 // 小时内平均值
}

// ScraperMetricDailyAvg 按天聚合的指标平均值。
type ScraperMetricDailyAvg struct {
	TargetName  string  // 目标名
	MetricName  string  // 指标名
	MetricAlias string  // 指标别名
	Unit        string  // 单位
	Day         int     // 当日时间戳（Unix 日）
	AvgValue    float64 // 日平均值
}

// ScraperMetricMonthlyAvg 按月聚合的指标平均值。
type ScraperMetricMonthlyAvg struct {
	TargetName  string  // 目标名
	MetricName  string  // 指标名
	MetricAlias string  // 指标别名
	Unit        string  // 单位
	Month       int     // 月份（YYYYMM）
	AvgValue    float64 // 月平均值
}

// SystemAlertRecord 系统资源告警的汇总记录。
type SystemAlertRecord struct {
	Date            string  // 统计日期
	Metric          string  // 指标内部名（cpu/memory/disk/network_down 等）
	MetricAlias     string  // 指标别名
	Value           float64 // 当前告警值
	Threshold       float64 // 阈值
	Unit            string  // 单位
	AlertLevel      string  // 告警级别
	AlertCount      int64   // 当日告警次数
	FirstOccurrence string  // 首次告警时间
	LastOccurrence  string  // 最近告警时间
	Message         string  // 告警描述
}

// ScraperAlertRecord 远程采集指标告警的汇总记录。
type ScraperAlertRecord struct {
	Date            string  // 统计日期
	TargetName      string  // 目标名
	TargetURL       string  // 目标 URL
	MetricName      string  // 指标名
	MetricAlias     string  // 指标别名
	Value           float64 // 当前值
	Threshold       float64 // 阈值
	Unit            string  // 单位
	AlertLevel      string  // 告警级别
	AlertCount      int64   // 告警次数
	FirstOccurrence string  // 首次告警时间
	LastOccurrence  string  // 最近告警时间
	Message         string  // 告警描述
}
