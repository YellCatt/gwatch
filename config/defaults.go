package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// setLogDefaults 设置日志模块的默认配置。
func setLogDefaults() {
	if GlobalConfig.Log.Level == "" {
		GlobalConfig.Log.Level = "info"
	}
	if GlobalConfig.Log.Encoding == "" {
		GlobalConfig.Log.Encoding = "json"
	}
	if GlobalConfig.Log.Output == "" {
		GlobalConfig.Log.Output = "./logs/gwatch.log"
	}
	if GlobalConfig.Log.MaxSizeMB <= 0 {
		GlobalConfig.Log.MaxSizeMB = 20
	}
}

// setCleanerDefaults 设置清理模块的默认配置（保留天数、日志目录、清理间隔等）。
func setCleanerDefaults() {
	hasCleanerConfig := viper.IsSet("cleaner")

	if !hasCleanerConfig {
		GlobalConfig.Cleaner.Enabled = true
		GlobalConfig.Cleaner.RetentionDays = 30
		GlobalConfig.Cleaner.LogDir = "./logs"
		GlobalConfig.Cleaner.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
		GlobalConfig.Cleaner.IntervalHours = 24
		return
	}

	if GlobalConfig.Cleaner.RetentionDays <= 0 {
		GlobalConfig.Cleaner.RetentionDays = 30
	}
	if GlobalConfig.Cleaner.LogDir == "" {
		GlobalConfig.Cleaner.LogDir = "./logs"
	}
	if len(GlobalConfig.Cleaner.IncludePatterns) == 0 {
		GlobalConfig.Cleaner.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
	}
	if GlobalConfig.Cleaner.IntervalHours <= 0 {
		GlobalConfig.Cleaner.IntervalHours = 24
	}
}

// setScraperDefaults 设置采集器模块的默认配置（目标超时、间隔、启用状态等）。
func setScraperDefaults() {
	hasScraperConfig := viper.IsSet("scraper")

	if !hasScraperConfig {
		GlobalConfig.Scraper.Enabled = false
		return
	}

	if !viper.IsSet("scraper.enabled") {
		GlobalConfig.Scraper.Enabled = false
	}

	for i := range GlobalConfig.Scraper.Targets {
		target := &GlobalConfig.Scraper.Targets[i]

		if target.Method == "" {
			target.Method = "GET"
		}

		if target.Timeout == "" {
			target.Timeout = "5s"
		}

		if target.Interval <= 0 {
			target.Interval = 10
		}

		if !viper.IsSet(fmt.Sprintf("scraper.targets.%d.enabled", i)) {
			target.Enabled = true
		}
	}
}

// setMonitorDefaults 设置监控模块的默认配置（告警间隔、各类报告启用状态、报告时间等）。
func setMonitorDefaults() {
	if GlobalConfig.Monitor.AlertInterval <= 0 {
		GlobalConfig.Monitor.AlertInterval = 6 * 60 * 60
	}

	if !viper.IsSet("monitor.daily_report") {
		GlobalConfig.Monitor.DailyReport = true
	}

	if !viper.IsSet("monitor.weekly_report") {
		GlobalConfig.Monitor.WeeklyReport = true
	}

	if !viper.IsSet("monitor.monthly_report") {
		GlobalConfig.Monitor.MonthlyReport = true
	}

	if !viper.IsSet("monitor.yearly_report") {
		GlobalConfig.Monitor.YearlyReport = true
	}

	if (GlobalConfig.Monitor.DailyReport ||
		GlobalConfig.Monitor.WeeklyReport ||
		GlobalConfig.Monitor.MonthlyReport ||
		GlobalConfig.Monitor.YearlyReport) &&
		GlobalConfig.Monitor.ReportTime == "" {
		GlobalConfig.Monitor.ReportTime = "07:00"
	}

	if !viper.IsSet("monitor.alert_on_failure") {
		GlobalConfig.Monitor.AlertOnFailure = true
	}
}

// setSystemMonitorDefaults 设置系统监控模块的默认配置（采集间隔、各指标阈值、告警冷却时间等）。
func setSystemMonitorDefaults() {
	sm := GlobalConfig.SystemMon

	hasSysMonConfig := viper.IsSet("sys_monitor")

	if !hasSysMonConfig {
		sm.Enabled = true
		sm.ChartEnabled = true
		sm.EmailEnabled = true
	} else {
		if !viper.IsSet("sys_monitor.enabled") {
			sm.Enabled = true
		}
		if !viper.IsSet("sys_monitor.chart_enabled") {
			sm.ChartEnabled = true
		}
		if !viper.IsSet("sys_monitor.email_enabled") {
			sm.EmailEnabled = true
		}
	}

	if sm.Interval <= 0 {
		sm.Interval = 10
	}
	if sm.CPUThreshold <= 0 {
		sm.CPUThreshold = 85
	}
	if sm.MemoryThreshold <= 0 {
		sm.MemoryThreshold = 90
	}
	if sm.DiskUsageThreshold <= 0 {
		sm.DiskUsageThreshold = 90
	}
	if sm.NetworkDownThreshold <= 0 {
		sm.NetworkDownThreshold = 3072
	}
	if sm.NetworkUpThreshold <= 0 {
		sm.NetworkUpThreshold = 1024
	}
	if sm.AlertCooldown <= 0 {
		sm.AlertCooldown = 7200
	}

	GlobalConfig.SystemMon = sm
}

// setEmailDefaults 设置邮件模块的默认告警冷却时间（秒）。
func setEmailDefaults() {
	em := &GlobalConfig.Email

	if em.ScraperCooldown <= 0 {
		em.ScraperCooldown = 21600
	}
	if em.APICooldown <= 0 {
		em.APICooldown = 21600
	}
	if em.SystemCooldown <= 0 {
		em.SystemCooldown = 7200
	}
}
