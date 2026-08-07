package monitor

import (
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/report"
	"gwatch/internal/timeutil"
)

// scheduleReports 创建并启动一个周期调度器，用于按配置时间触发定期报告（日/周/月/年）。
func scheduleReports() {
	scheduler := NewPeriodicScheduler(
		WithReportTime(config.GlobalConfig.Monitor.ReportTime),
		WithTriggerCallback(generateAllReports),
	)
	scheduler.Start()
}

// generateAllReports 根据当前时间触发所有已启用的报告（日/周/月/年）。
// 周、月、年报告只在对应周期的第一天才会生成。
func generateAllReports() {
	now := timeutil.Now()

	if config.GlobalConfig.Monitor.DailyReport {
		yesterday := now.Add(-24 * time.Hour)
		logger.Info("Generating daily report for", zap.String("date", yesterday.Format("2006-01-02")))
		generateAndSendReport(report.PeriodDaily, yesterday)
	}

	if config.GlobalConfig.Monitor.WeeklyReport {
		if ShouldTriggerWeekly(now) {
			logger.Info("Generating weekly report for week starting", zap.String("date", getWeekStart(now).Format("2006-01-02")))
			generateAndSendReport(report.PeriodWeekly, now)
		}
	}

	if config.GlobalConfig.Monitor.MonthlyReport {
		if ShouldTriggerMonthly(now) {
			logger.Info("Generating monthly report for", zap.String("month", now.Format("2006-01")))
			generateAndSendReport(report.PeriodMonthly, now)
		}
	}

	if config.GlobalConfig.Monitor.YearlyReport {
		if ShouldTriggerYearly(now) {
			logger.Info("Generating yearly report for", zap.String("year", now.Format("2006")))
			generateAndSendReport(report.PeriodYearly, now)
		}
	}
}

// getWeekStart 获取给定日期所在周的周一（作为一周的起始日期）。
func getWeekStart(date time.Time) time.Time {
	weekday := date.Weekday()
	daysToMonday := int(weekday - time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -daysToMonday)
}
