package config

import (
	"fmt"

	"github.com/spf13/viper"
)

func setCleanerDefaults() {
	hasCleanerConfig := viper.IsSet("cleaner")

	if !hasCleanerConfig {
		GlobalConfig.Cleaner.Enabled = true
		GlobalConfig.Cleaner.RetentionDays = 30
		GlobalConfig.Cleaner.LogDir = "./logs"
		GlobalConfig.Cleaner.ReportDir = "./reports"
		GlobalConfig.Cleaner.DataDir = "./sql"
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
	if GlobalConfig.Cleaner.ReportDir == "" {
		GlobalConfig.Cleaner.ReportDir = "./reports"
	}
	if GlobalConfig.Cleaner.DataDir == "" {
		GlobalConfig.Cleaner.DataDir = "./sql"
	}
	if len(GlobalConfig.Cleaner.IncludePatterns) == 0 {
		GlobalConfig.Cleaner.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
	}
	if GlobalConfig.Cleaner.IntervalHours <= 0 {
		GlobalConfig.Cleaner.IntervalHours = 24
	}
}

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

func setSystemMonitorDefaults() {
	sm := GlobalConfig.SystemMon

	hasSysMonConfig := viper.IsSet("sys_monitor")

	if !hasSysMonConfig {
		sm.Enabled = true
		sm.ChartEnabled = true
		sm.EmailEnabled = false
	} else {
		if !viper.IsSet("sys_monitor.enabled") {
			sm.Enabled = true
		}
		if !viper.IsSet("sys_monitor.chart_enabled") {
			sm.ChartEnabled = true
		}
		if !viper.IsSet("sys_monitor.email_enabled") {
			sm.EmailEnabled = false
		}
	}

	if sm.Interval <= 0 {
		sm.Interval = 10
	}
	if sm.RetentionHours <= 0 {
		sm.RetentionHours = 168
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
		sm.NetworkDownThreshold = 1.0
	}
	if sm.NetworkUpThreshold <= 0 {
		sm.NetworkUpThreshold = 1.0
	}
	if sm.AlertCooldown <= 0 {
		sm.AlertCooldown = 300
	}

	GlobalConfig.SystemMon = sm
}