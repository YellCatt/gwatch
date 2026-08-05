package report

import "gwatch/internal/timeutil"

func GenerateStartupReport() *Report {
	now := timeutil.Now()
	return &Report{
		Period:      PeriodStartup,
		StartDate:   now.Format("2006-01-02"),
		EndDate:     now.Format("2006-01-02"),
		GeneratedAt: now,
	}
}

func (r *Report) GenerateStartupContent() string {
	return executeTemplate("startup", buildStartupData(r))
}
