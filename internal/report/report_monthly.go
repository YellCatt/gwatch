package report

import "time"

// GenerateMonthlyReportFromStorage 从存储中生成指定日期所在月的月报告。
func GenerateMonthlyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(0, 1, 0)
	return GenerateReportFromStorage(PeriodMonthly, startDate, endDate)
}

// GenerateMonthlyContent 生成月报告的文本内容，包含每日资源数据和系统状态。
func (r *Report) GenerateMonthlyContent() string {
	data := struct {
		Base            baseReportData
		HasDaily        bool
		Daily           dailyResourceData
		HasSystemStatus bool
		SystemStatus    *SystemMetricsSnapshot
	}{
		Base:            buildBaseData(r),
		HasDaily:        len(r.DailyMetrics) > 0,
		Daily:           buildDailyResourceData(r, "每月报表"),
		HasSystemStatus: r.SystemMetrics != nil,
		SystemStatus:    r.SystemMetrics,
	}
	return executeTemplate("monthly", data)
}
