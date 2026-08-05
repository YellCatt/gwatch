package report

import "time"

func GenerateYearlyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), 1, 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(1, 0, 0)
	return GenerateReportFromStorage(PeriodYearly, startDate, endDate)
}

func (r *Report) GenerateYearlyContent() string {
	data := struct {
		Base       baseReportData
		HasMonthly bool
		Monthly    monthlyResourceData
	}{
		Base:       buildBaseData(r),
		HasMonthly: len(r.MonthlyMetrics) > 0,
		Monthly:    buildMonthlyResourceData(r),
	}
	return executeTemplate("yearly", data)
}
