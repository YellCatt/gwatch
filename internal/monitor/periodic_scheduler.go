package monitor

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
)

// PeriodicScheduler 是一个通用的定时调度器，
// 按配置的每日时间触发回调函数，并确保每天只触发一次。
type PeriodicScheduler struct {
	reportHour   int    // 触发小时（0-23）
	reportMinute int    // 触发分钟（0-59）
	lastSentDate string // 上次触发的日期（格式：2006-01-02），用于防止重复触发

	onTrigger func() // 触发回调函数
}

// PeriodicSchedulerOption 是 PeriodicScheduler 的配置选项函数类型，
// 采用 Functional Options 模式进行配置。
type PeriodicSchedulerOption func(*PeriodicScheduler)

// WithReportTime 配置每日触发时间，格式为 "HH:MM"（例如 "07:00"）。
// 默认为 "07:00"。
func WithReportTime(reportTimeStr string) PeriodicSchedulerOption {
	return func(s *PeriodicScheduler) {
		if reportTimeStr == "" {
			reportTimeStr = "07:00"
		}
		fmt.Sscanf(reportTimeStr, "%d:%d", &s.reportHour, &s.reportMinute)
	}
}

// WithTriggerCallback 配置触发回调函数，当到达指定时间时调用。
func WithTriggerCallback(callback func()) PeriodicSchedulerOption {
	return func(s *PeriodicScheduler) {
		s.onTrigger = callback
	}
}

// NewPeriodicScheduler 创建一个新的 PeriodicScheduler 实例。
// 默认触发时间为每天 07:00，可通过选项函数自定义配置。
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

// Start 启动定时调度循环，阻塞直到程序退出。
// 系统启动后只等到下一次调度时间才触发，不再补发"错过"的报告。
// 通过 lastSentDate 机制确保每天只触发一次回调。
func (s *PeriodicScheduler) Start() {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), s.reportHour, s.reportMinute, 0, 0, now.Location())

	if !now.Before(next) {
		next = next.Add(24 * time.Hour)
	}

	for {
		now := time.Now()
		duration := next.Sub(now)

		// 检测是否错过报告日期（例如系统长时间挂起后恢复）
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

		// 等待到下次触发时间
		if duration > 0 {
			logger.Info("Scheduling reports", zap.Time("next_run", next))
			time.Sleep(duration)
		}

		// 检查今天是否已经触发过，防止重复发送
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

// trigger 调用已注册的回调函数。
// 如果回调函数未配置则不执行任何操作。
func (s *PeriodicScheduler) trigger() {
	if s.onTrigger != nil {
		s.onTrigger()
	}
}

// ShouldTriggerWeekly 判断是否应触发周报告（每周一触发）。
func ShouldTriggerWeekly(now time.Time) bool {
	return now.Weekday() == time.Monday
}

// ShouldTriggerMonthly 判断是否应触发月报告（每月 1 日触发）。
func ShouldTriggerMonthly(now time.Time) bool {
	return now.Day() == 1
}

// ShouldTriggerYearly 判断是否应触发年报告（每年 1 月 1 日触发）。
func ShouldTriggerYearly(now time.Time) bool {
	return now.Month() == time.January && now.Day() == 1
}