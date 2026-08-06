package report

import "time"

type ReportPeriod string

const (
	PeriodStartup ReportPeriod = "startup"
	PeriodDaily   ReportPeriod = "daily"
	PeriodWeekly  ReportPeriod = "weekly"
	PeriodMonthly ReportPeriod = "monthly"
	PeriodYearly  ReportPeriod = "yearly"
)

var PeriodNames = map[ReportPeriod]string{
	PeriodStartup: "启动",
	PeriodDaily:   "每日",
	PeriodWeekly:  "每周",
	PeriodMonthly: "每月",
	PeriodYearly:  "年度",
}

var PeriodNamesEn = map[ReportPeriod]string{
	PeriodStartup: "startup",
	PeriodDaily:   "daily",
	PeriodWeekly:  "weekly",
	PeriodMonthly: "monthly",
	PeriodYearly:  "yearly",
}

type StartupTaskInfo struct {
	ID       string
	Desc     string
	Method   string
	URL      string
	Interval int
}

type StartupInfo struct {
	Tasks      []StartupTaskInfo
	MaxWorkers int
}

type Report struct {
	Period           ReportPeriod
	StartDate        string
	EndDate          string
	TotalTasks       int
	FailedTasks      int
	SuccessTasks     int
	InterfaceStats   []InterfaceStat
	AggregatedErrors []AggregatedError
	HourlyMetrics    []HourlyResourceMetric
	DailyMetrics     []DailyResourceMetric
	MonthlyMetrics   []MonthlyResourceMetric
	SystemMetrics    *SystemMetricsSnapshot
	GeneratedAt      time.Time
	startupInfo      *StartupInfo
}

type HourlyResourceMetric struct {
	TargetName  string
	MetricName  string
	MetricAlias string
	Unit        string
	HourlyData  []HourlyData
}

type DailyResourceMetric struct {
	TargetName  string
	MetricName  string
	MetricAlias string
	Unit        string
	DailyData   []DailyData
}

type MonthlyResourceMetric struct {
	TargetName  string
	MetricName  string
	MetricAlias string
	Unit        string
	MonthlyData []MonthlyData
}

type HourlyData struct {
	Hour     int
	AvgValue float64
}

type DailyData struct {
	Day      int
	DayLabel string
	AvgValue float64
}

type MonthlyData struct {
	Month      int
	MonthLabel string
	AvgValue   float64
}

type InterfaceStat struct {
	TaskID          string
	TaskDesc        string
	URL             string
	Method          string
	TotalCount      int
	SuccessCount    int
	FailedCount     int
	AvgDurationMS   int64
	MaxDurationMS   int64
	LastFailureTime string
}

type AggregatedError struct {
	TaskID          string
	TaskDesc        string
	URL             string
	Method          string
	ExpectedStatus  int
	AlertLevel      string
	AlertCount      int
	FirstOccurrence time.Time
	LastOccurrence  time.Time
	ErrorMsg        string
}

type SystemMetricsSnapshot struct {
	CPUPercent    float64
	MemoryPercent float64
	DiskPercent   float64
	NetDownKBps   float64
	NetUpKBps     float64
	MemUsedBytes  uint64
	MemTotalBytes uint64
	DiskUsedBytes uint64
	DiskTotalBytes uint64
}