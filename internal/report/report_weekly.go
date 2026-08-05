package report

import "time"

func GenerateWeeklyReportFromStorage(date time.Time) *Report {
	weekday := date.Weekday()
	daysToMonday := int(weekday - time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}
	startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -daysToMonday)
	endDate := startDate.AddDate(0, 0, 7)
	return GenerateReportFromStorage(PeriodWeekly, startDate, endDate)
}

func (r *Report) GenerateWeeklyContent() string {
	data := struct {
		Base     baseReportData
		HasDaily bool
		Daily    dailyResourceData
	}{
		Base:     buildBaseData(r),
		HasDaily: len(r.DailyMetrics) > 0,
		Daily:    buildDailyResourceData(r, "每周报表"),
	}
	return executeTemplate("weekly", data)
}
