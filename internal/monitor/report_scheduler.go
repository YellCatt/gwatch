package monitor

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/report"
	"gwatch/internal/timeutil"
)

func scheduleReports() {
	for {
		now := timeutil.Now()
		reportTimeStr := config.GlobalConfig.Monitor.ReportTime
		if reportTimeStr == "" {
			reportTimeStr = "07:00"
		}

		var reportHour, reportMinute int
		fmt.Sscanf(reportTimeStr, "%d:%d", &reportHour, &reportMinute)

		nextReportTime := time.Date(now.Year(), now.Month(), now.Day(), reportHour, reportMinute, 0, 0, now.Location())
		if now.After(nextReportTime) {
			nextReportTime = nextReportTime.Add(24 * time.Hour)
		}

		duration := nextReportTime.Sub(now)
		logger.Info("Scheduling reports", zap.Time("next_run", nextReportTime))

		time.Sleep(duration)

		if config.GlobalConfig.Monitor.DailyReport {
			yesterday := timeutil.Now().Add(-24 * time.Hour)
			logger.Info("Generating daily report for", zap.String("date", yesterday.Format("2006-01-02")))
			generateAndSendReport(report.PeriodDaily, yesterday)
		}

		if config.GlobalConfig.Monitor.WeeklyReport {
			today := timeutil.Now()
			if today.Weekday() == time.Monday {
				logger.Info("Generating weekly report for week starting", zap.String("date", getWeekStart(today).Format("2006-01-02")))
				generateAndSendReport(report.PeriodWeekly, today)
			}
		}

		if config.GlobalConfig.Monitor.MonthlyReport {
			today := timeutil.Now()
			if today.Day() == 1 {
				logger.Info("Generating monthly report for", zap.String("month", today.Format("2006-01")))
				generateAndSendReport(report.PeriodMonthly, today)
			}
		}

		if config.GlobalConfig.Monitor.YearlyReport {
			today := timeutil.Now()
			if today.Month() == time.January && today.Day() == 1 {
				logger.Info("Generating yearly report for", zap.String("year", today.Format("2006")))
				generateAndSendReport(report.PeriodYearly, today)
			}
		}
	}
}

func getWeekStart(date time.Time) time.Time {
	weekday := date.Weekday()
	daysToMonday := int(weekday - time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -daysToMonday)
}