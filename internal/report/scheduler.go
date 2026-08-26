package report

import (
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/scheduler"
	"gwatch/internal/timeutil"
)

// EmailSender 邮件发送器函数类型，接收主题和正文字符串，返回错误。
type EmailSender func(subject, body string) error

// ReportScheduler 报告调度器，基于周期调度器在指定时间触发各类报告的生成与发送。
type ReportScheduler struct {
	scheduler *scheduler.PeriodicScheduler
	sender    EmailSender
}

// NewReportScheduler 创建报告调度器实例，传入邮件发送器用于发送报告邮件。
func NewReportScheduler(sender EmailSender) *ReportScheduler {
	return &ReportScheduler{sender: sender}
}

// Start 启动报告调度器，根据全局配置中的报告时间触发 generateAllReports。
func (rs *ReportScheduler) Start() {
	rs.scheduler = scheduler.NewPeriodicScheduler(
		scheduler.WithReportTime(config.GlobalConfig.Monitor.ReportTime),
		scheduler.WithTriggerCallback(rs.generateAllReports),
	)
	rs.scheduler.Start()
}

// generateAllReports 根据全局配置依次生成并发送日、周、月、年报告。
// 每种报告都针对上一个完整周期：日报=昨天，周报=上周，月报=上月，年报=去年。
// 当 DailyAllReports 为 true 时，忽略周/月/年的日期限制，每天都生成所有报告（测试用）。
func (rs *ReportScheduler) generateAllReports() {
	now := timeutil.Now()
	dailyAll := config.GlobalConfig.Monitor.DailyAllReports

	if config.GlobalConfig.Monitor.DailyReport {
		yesterday := now.Add(-24 * time.Hour)
		logger.Info("正在生成日报", zap.String("日期", yesterday.Format("2006-01-02")))
		generateAndSendReport(PeriodDaily, yesterday, rs.sender)
	}

	if config.GlobalConfig.Monitor.WeeklyReport {
		if dailyAll || scheduler.ShouldTriggerWeekly(now) {
			lastWeek := now.Add(-7 * 24 * time.Hour)
			logger.Info("正在生成周报", zap.String("起始日期", scheduler.GetWeekStart(lastWeek).Format("2006-01-02")))
			generateAndSendReport(PeriodWeekly, lastWeek, rs.sender)
		}
	}

	if config.GlobalConfig.Monitor.MonthlyReport {
		if dailyAll || scheduler.ShouldTriggerMonthly(now) {
			lastMonth := now.AddDate(0, -1, 0)
			logger.Info("正在生成月报", zap.String("月份", lastMonth.Format("2006-01")))
			generateAndSendReport(PeriodMonthly, lastMonth, rs.sender)
		}
	}

	if config.GlobalConfig.Monitor.YearlyReport {
		if dailyAll || scheduler.ShouldTriggerYearly(now) {
			lastYear := now.AddDate(-1, 0, 0)
			logger.Info("正在生成年报", zap.String("年份", lastYear.Format("2006")))
			generateAndSendReport(PeriodYearly, lastYear, rs.sender)
		}
	}
}

// generateAndSendReport 根据报告周期确定时间区间，生成报告、保存文件并尝试发送邮件。
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
