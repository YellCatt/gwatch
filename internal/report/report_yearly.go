package report

import "time"

// GenerateYearlyReportFromStorage 从存储中生成指定日期所在年的年报告。
func GenerateYearlyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), 1, 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(1, 0, 0)
	return GenerateReportFromStorage(PeriodYearly, startDate, endDate)
}

// GenerateYearlyContent 生成年报告的文本内容，包含每月资源数据和系统状态。
func (r *Report) GenerateYearlyContent() string {
	data := struct {
		Base            baseReportData
		HasMonthly      bool
		Monthly         monthlyResourceData
		HasSystemStatus bool
		SystemStatus    *SystemMetricsSnapshot
	}{
		Base:            buildBaseData(r),
		HasMonthly:      len(r.MonthlyMetrics) > 0,
		Monthly:         buildMonthlyResourceData(r),
		HasSystemStatus: r.SystemMetrics != nil,
		SystemStatus:    r.SystemMetrics,
	}
	return executeTemplate("yearly", data)
}
