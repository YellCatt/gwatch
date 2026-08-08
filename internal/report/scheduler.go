package report

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

type EmailSender func(subject, body string) error

type PeriodicScheduler struct {
	reportHour   int
	reportMinute int
	lastSentDate string
	onTrigger    func()
}

type PeriodicSchedulerOption func(*PeriodicScheduler)

func WithReportTime(reportTimeStr string) PeriodicSchedulerOption {
	return func(s *PeriodicScheduler) {
		if reportTimeStr == "" {
			reportTimeStr = "07:00"
		}
		fmt.Sscanf(reportTimeStr, "%d:%d", &s.reportHour, &s.reportMinute)
	}
}

func WithTriggerCallback(callback func()) PeriodicSchedulerOption {
	return func(s *PeriodicScheduler) {
		s.onTrigger = callback
	}
}

func NewPeriodicScheduler(opts ...PeriodicSchedulerOption) *PeriodicScheduler {
	s := &PeriodicScheduler{
		reportHour:   7,
		reportMinute: 0,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *PeriodicScheduler) Start() {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), s.reportHour, s.reportMinute, 0, 0, now.Location())

	if !now.Before(next) {
		next = next.Add(24 * time.Hour)
	}

	for {
		now := time.Now()
		duration := next.Sub(now)

		if duration <= 0 {
			if now.Year() != next.Year() ||
				now.Month() != next.Month() ||
				now.Day() != next.Day() {
				logger.Warn("We've missed the report day, advancing",
					zap.Time("missed_report", next))
				next = next.Add(24 * time.Hour)
				continue
			}
		}

		if duration > 0 {
			logger.Info("Scheduling reports", zap.Time("next_run", next))
			time.Sleep(duration)
		}

		today := time.Now().Format("2006-01-02")
		if today == s.lastSentDate {
			logger.Info("Reports already sent today, skipping", zap.String("date", today))
			next = next.Add(24 * time.Hour)
			continue
		}
		s.lastSentDate = today

		s.trigger()

		next = next.Add(24 * time.Hour)
	}
}

func (s *PeriodicScheduler) trigger() {
	if s.onTrigger != nil {
		s.onTrigger()
	}
}

func ShouldTriggerWeekly(now time.Time) bool {
	return now.Weekday() == time.Monday
}

func ShouldTriggerMonthly(now time.Time) bool {
	return now.Day() == 1
}

func ShouldTriggerYearly(now time.Time) bool {
	return now.Month() == time.January && now.Day() == 1
}

func getWeekStart(date time.Time) time.Time {
	weekday := date.Weekday()
	daysToMonday := int(weekday - time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -daysToMonday)
}

type ReportScheduler struct {
	scheduler *PeriodicScheduler
	sender    EmailSender
}

func NewReportScheduler(sender EmailSender) *ReportScheduler {
	return &ReportScheduler{sender: sender}
}

func (rs *ReportScheduler) Start() {
	rs.scheduler = NewPeriodicScheduler(
		WithReportTime(config.GlobalConfig.Monitor.ReportTime),
		WithTriggerCallback(rs.generateAllReports),
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
		if ShouldTriggerWeekly(now) {
			logger.Info("Generating weekly report for week starting", zap.String("date", getWeekStart(now).Format("2006-01-02")))
			generateAndSendReport(PeriodWeekly, now, rs.sender)
		}
	}

	if config.GlobalConfig.Monitor.MonthlyReport {
		if ShouldTriggerMonthly(now) {
			logger.Info("Generating monthly report for", zap.String("month", now.Format("2006-01")))
			generateAndSendReport(PeriodMonthly, now, rs.sender)
		}
	}

	if config.GlobalConfig.Monitor.YearlyReport {
		if ShouldTriggerYearly(now) {
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
		weekday := date.Weekday()
		daysToMonday := int(weekday - time.Monday)
		if daysToMonday < 0 {
			daysToMonday += 7
		}
		startDate = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -daysToMonday)
		endDate = startDate.AddDate(0, 0, 7)
	case PeriodMonthly:
		startDate = time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
		endDate = startDate.AddDate(0, 1, 0)
	case PeriodYearly:
		startDate = time.Date(date.Year(), 1, 1, 0, 0, 0, 0, date.Location())
		endDate = startDate.AddDate(1, 0, 0)
	default:
		logger.Error("Unknown report period", zap.String("period", string(period)))
		return
	}

	r := GenerateReportFromStorage(period, startDate, endDate)

	_, err := r.SaveReport()
	if err != nil {
		logger.Error("Failed to save report", zap.Error(err))
		return
	}

	subject, body := r.PrepareReportEmail()
	if sender != nil {
		err = sender(subject, body)
		if err != nil {
			logger.Error("Failed to send report email", zap.Error(err))
		}
	}
}
