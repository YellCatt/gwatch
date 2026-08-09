package scheduler

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
)

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

func GetWeekStart(date time.Time) time.Time {
	weekday := date.Weekday()
	daysToMonday := int(weekday - time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -daysToMonday)
}
