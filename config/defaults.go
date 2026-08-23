package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// setLogDefaults 设置日志模块的默认配置。
// 当用户未在配置文件中显式指定日志相关参数时，采用以下默认值：
//   - Level: 日志级别，默认 info，可选 debug / warn / error
//   - Encoding: 输出格式，默认 json，便于结构化采集
//   - Output: 日志文件路径，默认 ./logs/gwatch.log
//   - MaxSizeMB: 单个日志文件最大体积（MB），超出后自动轮转
func setLogDefaults() {
	// 若日志级别为空，默认使用 info 级别
	if GlobalConfig.Log.Level == "" {
		GlobalConfig.Log.Level = "info"
	}
	// 若日志编码未指定，默认使用 json 格式输出
	if GlobalConfig.Log.Encoding == "" {
		GlobalConfig.Log.Encoding = "json"
	}
	// 若日志输出路径未指定，默认写入 ./logs/gwatch.log
	if GlobalConfig.Log.Output == "" {
		GlobalConfig.Log.Output = "./logs/gwatch.log"
	}
	// 若单文件最大体积未设置或非法，默认 20 MB
	if GlobalConfig.Log.MaxSizeMB <= 0 {
		GlobalConfig.Log.MaxSizeMB = 20
	}
}

// setCleanerDefaults 设置清理模块的默认配置（保留天数、日志目录、清理间隔等）。
// 清理器用于定期清理过期的日志文件，防止磁盘被占满。
//   - RetentionDays: 日志保留天数，超过此天数的日志将被删除
//   - LogDir: 需要清理的日志目录
//   - IncludePatterns: 参与清理的文件扩展名匹配模式
//   - IntervalHours: 清理任务的执行间隔（小时）
func setCleanerDefaults() {
	// 判断是否存在 cleaner 配置段
	hasCleanerConfig := viper.IsSet("cleaner")

	// 若完全没有 cleaner 配置，直接写入一套默认值并返回
	if !hasCleanerConfig {
		GlobalConfig.Cleaner.Enabled = true
		GlobalConfig.Cleaner.RetentionDays = 30
		GlobalConfig.Cleaner.LogDir = "./logs"
		GlobalConfig.Cleaner.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
		GlobalConfig.Cleaner.IntervalHours = 24
		return
	}

	// 存在 cleaner 配置时，对缺失或非法的字段逐项补默认值
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
// 采集器负责周期性地向目标 URL 发送请求，用于探测可用性和响应时间。
//   - Enabled: 采集器总开关，默认关闭
//   - Targets: 采集目标列表，每个目标可独立配置
//   - Method: HTTP 方法，默认 GET
//   - Timeout: 单次请求超时时间，默认 5s
//   - Interval: 采集间隔（秒），默认 10 秒
//   - Enabled: 单个目标的开关，默认开启
func setScraperDefaults() {
	// 判断是否存在 scraper 配置段
	hasScraperConfig := viper.IsSet("scraper")

	// 若完全没有 scraper 配置，默认禁用采集器
	if !hasScraperConfig {
		GlobalConfig.Scraper.Enabled = false
		return
	}

	// 若 scraper.enabled 未显式设置，默认禁用
	if !viper.IsSet("scraper.enabled") {
		GlobalConfig.Scraper.Enabled = false
	}

	// 遍历每个采集目标，为缺失字段补默认值
	for i := range GlobalConfig.Scraper.Targets {
		target := &GlobalConfig.Scraper.Targets[i]

		// 默认使用 GET 方法
		if target.Method == "" {
			target.Method = "GET"
		}

		// 默认超时 5 秒
		if target.Timeout == "" {
			target.Timeout = "5s"
		}

		// 默认采集间隔 10 秒
		if target.Interval <= 0 {
			target.Interval = 10
		}

		// 单个目标默认启用；仅当配置中显式指定时才覆盖
		if !viper.IsSet(fmt.Sprintf("scraper.targets.%d.enabled", i)) {
			target.Enabled = true
		}
	}
}

// setMonitorDefaults 设置监控模块的默认配置（告警间隔、各类报告启用状态、报告时间等）。
// 监控模块负责汇总采集结果、生成报告、触发告警。
//   - AlertInterval: 同一告警的最小间隔（秒），用于告警去重/节流
//   - DailyReport / WeeklyReport / MonthlyReport / YearlyReport: 各类定期报告开关
//   - ReportTime: 定期报告生成的时间点，格式 HH:MM
//   - AlertOnFailure: 采集失败时是否立即告警
func setMonitorDefaults() {
	// 告警间隔默认 6 小时（单位：秒）
	if GlobalConfig.Monitor.AlertInterval <= 0 {
		GlobalConfig.Monitor.AlertInterval = 6 * 60 * 60
	}

	// 以下四类报告默认全部启用，除非用户在配置中显式关闭
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

	// 只要任一定期报告被启用，就需要设置报告生成时间，默认凌晨 05:00
	if (GlobalConfig.Monitor.DailyReport ||
		GlobalConfig.Monitor.WeeklyReport ||
		GlobalConfig.Monitor.MonthlyReport ||
		GlobalConfig.Monitor.YearlyReport) &&
		GlobalConfig.Monitor.ReportTime == "" {
		GlobalConfig.Monitor.ReportTime = "05:00"
	}

	// 采集失败默认触发告警
	if !viper.IsSet("monitor.alert_on_failure") {
		GlobalConfig.Monitor.AlertOnFailure = true
	}
}

// setSystemMonitorDefaults 设置系统监控模块的默认配置（采集间隔、各指标阈值、告警冷却时间等）。
// 系统监控负责采集 CPU、内存、磁盘、网络等指标，超阈值时触发告警。
//   - Enabled: 系统监控总开关
//   - ChartEnabled: 是否在报告中生成图表
//   - EmailEnabled: 是否通过邮件发送告警
//   - Interval: 指标采集间隔（秒）
//   - CPUThreshold: CPU 使用率告警阈值（百分比）
//   - MemoryThreshold: 内存使用率告警阈值（百分比）
//   - DiskUsageThreshold: 磁盘使用率告警阈值（百分比）
//   - NetworkDownThreshold: 下行流量告警阈值（KB/s）
//   - NetworkUpThreshold: 上行流量告警阈值（KB/s）
//   - AlertCooldown: 同一系统告警的冷却时间（秒）
func setSystemMonitorDefaults() {
	sm := GlobalConfig.SystemMon

	// 判断是否存在 sys_monitor 配置段
	hasSysMonConfig := viper.IsSet("sys_monitor")

	if !hasSysMonConfig {
		// 无配置时，启用系统监控、图表和邮件告警
		sm.Enabled = true
		sm.ChartEnabled = true
		sm.EmailEnabled = true
	} else {
		// 有配置时，仅对缺失的字段补默认值
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

	// 指标采集间隔默认 10 秒
	if sm.Interval <= 0 {
		sm.Interval = 10
	}
	// CPU 使用率告警阈值默认 85%
	if sm.CPUThreshold <= 0 {
		sm.CPUThreshold = 85
	}
	// 内存使用率告警阈值默认 90%
	if sm.MemoryThreshold <= 0 {
		sm.MemoryThreshold = 90
	}
	// 磁盘使用率告警阈值默认 90%
	if sm.DiskUsageThreshold <= 0 {
		sm.DiskUsageThreshold = 90
	}
	// 下行网络流量告警阈值默认 3072 KB/s
	if sm.NetworkDownThreshold <= 0 {
		sm.NetworkDownThreshold = 3072
	}
	// 上行网络流量告警阈值默认 1024 KB/s
	if sm.NetworkUpThreshold <= 0 {
		sm.NetworkUpThreshold = 1024
	}
	// 系统告警冷却时间默认 7200 秒（2 小时），避免告警风暴
	if sm.AlertCooldown <= 0 {
		sm.AlertCooldown = 7200
	}

	GlobalConfig.SystemMon = sm
}

// setEmailDefaults 设置邮件模块的默认告警冷却时间（秒）。
// 不同类型的告警使用不同的冷却时间，避免短时间内重复收到相同告警邮件：
//   - ScraperCooldown: 采集器类告警冷却，默认 21600 秒（6 小时）
//   - APICooldown: API 类告警冷却，默认 21600 秒（6 小时）
//   - SystemCooldown: 系统监控类告警冷却，默认 7200 秒（2 小时）
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
