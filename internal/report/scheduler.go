package report

import (
	"path/filepath"
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
		scheduler.WithStateFile(reportSentStatePath()),
	)
	rs.scheduler.Start()
}

// reportSentStatePath 返回报告发送状态文件的路径。
// 记录"上次触发日期"，使进程重启后仍能避免当天重复发送报告。
// 目录取自 app.data_dir，未配置时回退到 ./sql。
func reportSentStatePath() string {
	dir := config.GlobalConfig.App.DataDir
	if dir == "" {
		dir = "./sql"
	}
	return filepath.Join(dir, "last_report_sent.txt")
}

// generateAllReports 根据全局配置依次生成并发送日、周、月、年报告。
// 每种报告都针对上一个完整周期：日报=昨天，周报=上周，月报=上月；
// 年报是累计型报告，统计当年 1 月 ~ 上月末（1 月时统计上一年度），具体区间见 YearlyRange。
// 当 DailyAllReports 为 true 时，忽略周/月/年的日期限制，每天都生成所有报告（测试用）。
func (rs *ReportScheduler) generateAllReports() {
	now := timeutil.Now()
	dailyAll := config.GlobalConfig.Monitor.DailyAllReports

	logger.Info("报告调度触发",
		zap.Time("当前时间", now),
		zap.Bool("全量日报模式", dailyAll),
		zap.Bool("日报开启", config.GlobalConfig.Monitor.DailyReport),
		zap.Bool("周报开启", config.GlobalConfig.Monitor.WeeklyReport),
		zap.Bool("月报开启", config.GlobalConfig.Monitor.MonthlyReport),
		zap.Bool("年报开启", config.GlobalConfig.Monitor.YearlyReport),
	)

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
		} else {
			logger.Debug("跳过周报：尚未到触发时间",
				zap.Time("当前", now),
				zap.Bool("ShouldTriggerWeekly", scheduler.ShouldTriggerWeekly(now)),
			)
		}
	}

	if config.GlobalConfig.Monitor.MonthlyReport {
		if dailyAll || scheduler.ShouldTriggerMonthly(now) {
			lastMonth := now.AddDate(0, -1, 0)
			logger.Info("正在生成月报", zap.String("月份", lastMonth.Format("2006-01")))
			generateAndSendReport(PeriodMonthly, lastMonth, rs.sender)
		} else {
			logger.Debug("跳过月报：尚未到触发时间",
				zap.Time("当前", now),
				zap.Bool("ShouldTriggerMonthly", scheduler.ShouldTriggerMonthly(now)),
			)
		}
	}

	if config.GlobalConfig.Monitor.YearlyReport {
		if dailyAll || scheduler.ShouldTriggerYearly(now) {
			start, end := YearlyRange(now)
			logger.Info("正在生成年报",
				zap.String("统计区间", start.Format("2006-01-02")+" ~ "+end.Format("2006-01-02")))
			generateAndSendReport(PeriodYearly, now, rs.sender)
		} else {
			logger.Debug("跳过年报：尚未到触发时间",
				zap.Time("当前", now),
				zap.Bool("ShouldTriggerYearly", scheduler.ShouldTriggerYearly(now)),
			)
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
		startDate, endDate = YearlyRange(date)
	default:
		logger.Warn("未知的报告周期", zap.String("周期", string(period)))
		return
	}

	logger.Info("开始生成报告",
		zap.String("周期", string(period)),
		zap.Time("起始", startDate),
		zap.Time("结束", endDate),
		zap.Duration("时长范围", endDate.Sub(startDate)),
	)

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
		} else {
			logger.Info("报告邮件已发送",
				zap.String("周期", string(period)),
				zap.Int("正文长度", len(body)),
			)
		}
	} else {
		logger.Warn("发送报告邮件失败：邮件发送器未配置",
			zap.String("周期", reportName),
			zap.String("开始日期", startDate.Format("2006-01-02")),
			zap.String("结束日期", endDate.Format("2006-01-02")))
	}
}