package report

import "time"

func Generate(period ReportPeriod, startDate, endDate time.Time) *Report {
	return GenerateReportFromStorage(period, startDate, endDate)
}

func GenerateDaily(date time.Time) *Report {
	return GenerateDailyReportFromStorage(date)
}

func GenerateWeekly(date time.Time) *Report {
	return GenerateWeeklyReportFromStorage(date)
}

func GenerateMonthly(date time.Time) *Report {
	return GenerateMonthlyReportFromStorage(date)
}

func GenerateYearly(date time.Time) *Report {
	return GenerateYearlyReportFromStorage(date)
}

func GenerateStartup() *Report {
	return GenerateStartupReport()
}
