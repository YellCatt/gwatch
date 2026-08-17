package report

import (
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/scheduler"
	"gwatch/internal/timeutil"
)

type EmailSender func(subject, body string) error

type ReportScheduler struct {
	scheduler *scheduler.PeriodicScheduler
	sender    EmailSender
}

func NewReportScheduler(sender EmailSender) *ReportScheduler {
	return &ReportScheduler{sender: sender}
}

func (rs *ReportScheduler) Start() {
	rs.scheduler = scheduler.NewPeriodicScheduler(
		scheduler.WithReportTime(config.GlobalConfig.Monitor.ReportTime),
		scheduler.WithTriggerCallback(rs.generateAllReports),
	)
	rs.scheduler.Start()
}

func (rs *ReportScheduler) generateAllReports() {
	now := timeutil.Now()

	if config.GlobalConfig.Monitor.DailyReport {
		yesterday := now.Add(-24 * time.Hour)
		logger.Info("Generating daily report for", zap.String("date", yesterday.Format("2006-01-02")))
		generateAndSendReport(PeriodDaily, yesterday, rs.sender)
	}

	if config.GlobalConfig.Monitor.WeeklyReport {
		if scheduler.ShouldTriggerWeekly(now) {
			logger.Info("Generating weekly report for week starting", zap.String("date", scheduler.GetWeekStart(now).Format("2006-01-02")))
			generateAndSendReport(PeriodWeekly, now, rs.sender)
		}
	}

	if config.GlobalConfig.Monitor.MonthlyReport {
		if scheduler.ShouldTriggerMonthly(now) {
			logger.Info("Generating monthly report for", zap.String("month", now.Format("2006-01")))
			generateAndSendReport(PeriodMonthly, now, rs.sender)
		}
	}

	if config.GlobalConfig.Monitor.YearlyReport {
		if scheduler.ShouldTriggerYearly(now) {
			logger.Info("Generating yearly report for", zap.String("year", now.Format("2006")))
			generateAndSendReport(PeriodYearly, now, rs.sender)
		}
	}
}

func generateAndSendReport(period ReportPeriod, date time.Time, sender EmailSender) {
	var startDate, endDate time.Time
	switch period {
	case PeriodDaily:
		startDate = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		endDate = startDate.Add(24 * time.Hour)
	case PeriodWeekly:
		startDate = scheduler.GetWeekStart(date)
		endDate = startDate.AddDate(0, 0, 7)
	case PeriodMonthly:
		startDate = time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
		endDate = startDate.AddDate(0, 1, 0)
	case PeriodYearly:
		startDate = time.Date(date.Year(), 1, 1, 0, 0, 0, 0, date.Location())
		endDate = startDate.AddDate(1, 0, 0)
	default:
		logger.Warn("Unknown report period", zap.String("period", string(period)))
		return
	}

	r := GenerateReportFromStorage(period, startDate, endDate)

	reportName := PeriodNames[period]
	_, err := r.SaveReport()
	if err != nil {
		logger.Error("Failed to save report",
			zap.String("period", string(period)),
			zap.String("start_date", startDate.Format("2006-01-02")),
			zap.String("end_date", endDate.Format("2006-01-02")),
			zap.Error(err))
	}

	subject, body := r.PrepareReportEmail()
	if sender != nil {
		err = sender(subject, body)
		if err != nil {
			logger.Error("Failed to send report email",
				zap.String("period", string(period)),
				zap.Error(err))
		}
	} else {
		logger.Error("Failed to send report email: sender is not configured",
			zap.String("period", reportName),
			zap.String("start_date", startDate.Format("2006-01-02")),
			zap.String("end_date", endDate.Format("2006-01-02")))
	}
}