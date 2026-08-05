package report

import "gwatch/internal/timeutil"

func GenerateStartupReport(info *StartupInfo) *Report {
	now := timeutil.Now()
	return &Report{
		Period:      PeriodStartup,
		StartDate:   now.Format("2006-01-02"),
		EndDate:     now.Format("2006-01-02"),
		GeneratedAt: now,
		startupInfo: info,
	}
}

func (r *Report) GenerateStartupContent() string {
	return executeTemplate("startup", buildStartupData(r, r.startupInfo))
}
