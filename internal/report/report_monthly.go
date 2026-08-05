package report

import "time"

func GenerateMonthlyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(0, 1, 0)
	return GenerateReportFromStorage(PeriodMonthly, startDate, endDate)
}

func (r *Report) GenerateMonthlyContent() string {
	data := struct {
		Base     baseReportData
		HasDaily bool
		Daily    dailyResourceData
	}{
		Base:     buildBaseData(r),
		HasDaily: len(r.DailyMetrics) > 0,
		Daily:    buildDailyResourceData(r, "每月报表"),
	}
	return executeTemplate("monthly", data)
}
