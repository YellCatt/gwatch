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