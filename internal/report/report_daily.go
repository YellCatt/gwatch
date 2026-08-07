package report

import "time"

// GenerateDailyReportFromStorage 从存储中生成指定日期的每日报告。
func GenerateDailyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endDate := startDate.Add(24 * time.Hour)
	return GenerateReportFromStorage(PeriodDaily, startDate, endDate)
}

// GenerateDailyContent 生成每日报告的文本内容，包含每小时资源数据和系统状态。
func (r *Report) GenerateDailyContent() string {
	data := struct {
		Base            baseReportData
		HasHourly       bool
		Hourly          hourlyResourceData
		HasSystemStatus bool
		SystemStatus    *SystemMetricsSnapshot
	}{
		Base:            buildBaseData(r),
		HasHourly:       len(r.HourlyMetrics) > 0,
		Hourly:          buildHourlyResourceData(r),
		HasSystemStatus: r.SystemMetrics != nil,
		SystemStatus:    r.SystemMetrics,
	}
	return executeTemplate("daily", data)
}
