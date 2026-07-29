package report

import "time"

var (
	PeriodDaily   = ReportPeriod("daily")
	PeriodWeekly  = ReportPeriod("weekly")
	PeriodMonthly = ReportPeriod("monthly")
	PeriodYearly  = ReportPeriod("yearly")
)

func Generate(period ReportPeriod, startDate, endDate time.Time) *Report {
	return GenerateReportFromStorage(period, startDate, endDate)
}