// Package scheduler 提供周期性调度器，用于在固定时间点（默认每日 07:00）
// 触发报告生成等后台任务。同时提供周/月/年周期判断的工具函数。
package scheduler

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

// PeriodicScheduler 每日周期调度器。
// 到点后会调用 onTrigger 回调执行任务，并通过 lastSentDate 防止当天重复触发。
type PeriodicScheduler struct {
	reportHour   int    // 触发小时（24 小时制）
	reportMinute int    // 触发分钟
	lastSentDate string // 上次触发日期（YYYY-MM-DD），用于去重
	onTrigger    func() // 触发回调
}

// PeriodicSchedulerOption 调度器配置项函数。
type PeriodicSchedulerOption func(*PeriodicScheduler)

// WithReportTime 设置每日报告触发时间（格式 HH:mm）。
func WithReportTime(reportTimeStr string) PeriodicSchedulerOption {
	return func(s *PeriodicScheduler) {
		if reportTimeStr == "" {
			reportTimeStr = "07:00"
		}
		fmt.Sscanf(reportTimeStr, "%d:%d", &s.reportHour, &s.reportMinute)
	}
}

// WithTriggerCallback 设置触发回调函数。
func WithTriggerCallback(callback func()) PeriodicSchedulerOption {
	return func(s *PeriodicScheduler) {
		s.onTrigger = callback
	}
}

// NewPeriodicScheduler 创建一个默认每天 07:00 触发的周期调度器。
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

// Start 阻塞运行调度循环，到点触发回调。
func (s *PeriodicScheduler) Start() {
	now := timeutil.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), s.reportHour, s.reportMinute, 0, 0, now.Location())

	if !now.Before(next) {
		next = next.Add(24 * time.Hour)
	}

	for {
		now := timeutil.Now()
		duration := next.Sub(now)

		if duration <= 0 {
			if now.Year() != next.Year() ||
				now.Month() != next.Month() ||
				now.Day() != next.Day() {
				logger.Warn("已错过报告发送时间，顺延至下一天",
					zap.Time("missed_report", next))
				next = next.Add(24 * time.Hour)
				continue
			}
		}

		if duration > 0 {
			logger.Info("报告调度中", zap.Time("next_run", next))
			time.Sleep(duration)
		}

		today := timeutil.Now().Format("2006-01-02")
		if today == s.lastSentDate {
			logger.Info("今日报告已发送，跳过", zap.String("date", today))
			next = next.Add(24 * time.Hour)
			continue
		}
		s.lastSentDate = today

		s.trigger()

		next = next.Add(24 * time.Hour)
	}
}

// trigger 执行触发回调，防止回调为空时 panic。
func (s *PeriodicScheduler) trigger() {
	if s.onTrigger != nil {
		s.onTrigger()
	}
}

// ShouldTriggerWeekly 判断是否需要触发周报（周一）。
func ShouldTriggerWeekly(now time.Time) bool {
	return now.Weekday() == time.Monday
}

// ShouldTriggerMonthly 判断是否需要触发月报（每月 1 号）。
func ShouldTriggerMonthly(now time.Time) bool {
	return now.Day() == 1
}

// ShouldTriggerYearly 判断是否需要触发年报（每年 1 月 1 日）。
func ShouldTriggerYearly(now time.Time) bool {
	return now.Month() == time.January && now.Day() == 1
}

// GetWeekStart 获取指定日期所在周的周一 00:00:00。
func GetWeekStart(date time.Time) time.Time {
	weekday := date.Weekday()
	daysToMonday := int(weekday - time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -daysToMonday)
}
