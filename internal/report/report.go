package report

import "time"

// Generate 根据指定周期与时间区间生成报告（从存储中读取数据）。
func Generate(period ReportPeriod, startDate, endDate time.Time) *Report {
	return GenerateReportFromStorage(period, startDate, endDate)
}

// GenerateDaily 便捷函数：生成指定日期的每日报告。
func GenerateDaily(date time.Time) *Report {
	return GenerateDailyReportFromStorage(date)
}

// GenerateWeekly 便捷函数：生成指定日期所在周的周报告。
func GenerateWeekly(date time.Time) *Report {
	return GenerateWeeklyReportFromStorage(date)
}

// GenerateMonthly 便捷函数：生成指定日期所在月的月报告。
func GenerateMonthly(date time.Time) *Report {
	return GenerateMonthlyReportFromStorage(date)
}

// GenerateYearly 便捷函数：生成指定日期所在年的年报告。
func GenerateYearly(date time.Time) *Report {
	return GenerateYearlyReportFromStorage(date)
}

// GenerateStartup 便捷函数：生成启动报告。
func GenerateStartup(info *StartupInfo) *Report {
	return GenerateStartupReport(info)
}
