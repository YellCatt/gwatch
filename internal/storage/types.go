package storage

import "time"

const (
	AlertLevelCritical = "CRITICAL"
	AlertLevelWarning  = "WARNING"
)

// MonitorResultRecord 表示监控结果记录
type MonitorResultRecord struct {
	TestCaseID     string
	TestCaseDesc   string
	URL            string
	Method         string
	ExpectedStatus int
	ActualStatus   int
	ExpectedBody   string
	ActualBody     string
	ErrorMsg       string
	DurationMS     int64
	Success        bool
	AlertType      string
	Timestamp      time.Time
}

// MonitorSummaryRecord 表示监控每日汇总记录
type MonitorSummaryRecord struct {
	Date            string
	TestCaseID      string
	TestCaseDesc    string
	URL             string
	Method          string
	TotalCount      int64
	SuccessCount    int64
	FailedCount     int64
	TotalDurationMS int64
	MinDurationMS   int64
	MaxDurationMS   int64
	LastSuccessTime string
	LastFailureTime string
}

// AlertSummaryRecord 表示告警汇总记录（按任务ID和日期聚合）
type AlertSummaryRecord struct {
	Date            string
	TestCaseID      string
	TestCaseDesc    string
	URL             string
	Method          string
	ExpectedStatus  int
	AlertLevel      string
	AlertCount      int64
	FirstOccurrence string
	LastOccurrence  string
	ErrorMsg        string
}

// ScraperMetricRecord 表示采集指标记录
type ScraperMetricRecord struct {
	TargetName  string
	TargetURL   string
	MetricName  string
	MetricAlias string
	Value       float64
	Unit        string
	Success     bool
	Timestamp   time.Time
}

// ScraperMetricHourlyAvg 表示按小时聚合的指标平均值
type ScraperMetricHourlyAvg struct {
	TargetName  string
	MetricName  string
	MetricAlias string
	Unit        string
	Hour        int
	AvgValue    float64
}

// ScraperMetricDailyAvg 表示按天聚合的指标平均值
type ScraperMetricDailyAvg struct {
	TargetName  string
	MetricName  string
	MetricAlias string
	Unit        string
	Day         int
	AvgValue    float64
}

// ScraperMetricMonthlyAvg 表示按月聚合的指标平均值
type ScraperMetricMonthlyAvg struct {
	TargetName  string
	MetricName  string
	MetricAlias string
	Unit        string
	Month       int
	AvgValue    float64
}

// SystemAlertRecord 表示系统告警汇总记录
type SystemAlertRecord struct {
	Date            string
	Metric          string
	MetricAlias     string
	Value           float64
	Threshold       float64
	Unit            string
	AlertLevel      string
	AlertCount      int64
	FirstOccurrence string
	LastOccurrence  string
	Message         string
}

// ScraperAlertRecord 表示采集告警汇总记录
type ScraperAlertRecord struct {
	Date            string
	TargetName      string
	TargetURL       string
	MetricName      string
	MetricAlias     string
	Value           float64
	Threshold       float64
	Unit            string
	AlertLevel      string
	AlertCount      int64
	FirstOccurrence string
	LastOccurrence  string
	Message         string
}
