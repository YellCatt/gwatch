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
		logger.Info("正在生成日报", zap.String("日期", yesterday.Format("2006-01-02")))
		generateAndSendReport(PeriodDaily, yesterday, rs.sender)
	}

	if config.GlobalConfig.Monitor.WeeklyReport {
		if scheduler.ShouldTriggerWeekly(now) {
			logger.Info("正在生成周报", zap.String("起始日期", scheduler.GetWeekStart(now).Format("2006-01-02")))
			generateAndSendReport(PeriodWeekly, now, rs.sender)
		}
	}

	if config.GlobalConfig.Monitor.MonthlyReport {
		if scheduler.ShouldTriggerMonthly(now) {
			logger.Info("正在生成月报", zap.String("月份", now.Format("2006-01")))
			generateAndSendReport(PeriodMonthly, now, rs.sender)
		}
	}

	if config.GlobalConfig.Monitor.YearlyReport {
		if scheduler.ShouldTriggerYearly(now) {
			logger.Info("正在生成年报", zap.String("年份", now.Format("2006")))
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
		logger.Warn("未知的报告周期", zap.String("周期", string(period)))
		return
	}

	r := GenerateReportFromStorage(period, startDate, endDate)

	reportName := PeriodNames[period]
	_, err := r.SaveReport()
	if err != nil {
		logger.Error("保存报告失败",
			zap.String("周期", string(period)),
			zap.String("开始日期", startDate.Format("2006-01-02")),
			zap.String("结束日期", endDate.Format("2006-01-02")),
			zap.Error(err))
	}

	subject, body := r.PrepareReportEmail()
	if sender != nil {
		err = sender(subject, body)
		if err != nil {
			logger.Warn("发送报告邮件失败",
				zap.String("周期", string(period)),
				zap.Error(err))
		}
	} else {
		logger.Warn("发送报告邮件失败：邮件发送器未配置",
			zap.String("周期", reportName),
			zap.String("开始日期", startDate.Format("2006-01-02")),
			zap.String("结束日期", endDate.Format("2006-01-02")))
	}
}