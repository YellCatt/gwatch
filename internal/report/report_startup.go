package report

import "gwatch/internal/timeutil"

// GenerateStartupReport 创建启动报告对象，包含启动时间和启动信息。
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

// GenerateStartupContent 生成启动报告的文本内容。
func (r *Report) GenerateStartupContent() string {
	return executeTemplate("startup", buildStartupData(r, r.startupInfo))
}
