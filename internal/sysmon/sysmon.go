package sysmon

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
)

var (
	history    []SystemMetric
	historyMu  sync.RWMutex
	maxHistory = 600
	stopSysMon chan struct{}
	running    bool
	runningMu  sync.Mutex
	hourlyAgg  hourlyAggregator
	hourlyMu   sync.Mutex
)

// StartSystemMonitor 启动系统资源监控（若配置启用），周期性采集指标、检测阈值、
// 写入存储、聚合上层指标（日/月/年），并阻塞等待停止信号。
func StartSystemMonitor() {
	if !setupSystemMonitor() {
		return
	}

	<-stopSysMon

	runningMu.Lock()
	running = false
	runningMu.Unlock()

	logger.Info("System monitor stopped")
}

// setupSystemMonitor 初始化系统监控但不等待信号，由外部统一管理生命周期。
// 返回 false 表示系统监控未启用或初始化失败。
func setupSystemMonitor() bool {
	cfg := config.GlobalConfig.SystemMon
	if !cfg.Enabled {
		logger.Info("System monitor is disabled")
		return false
	}

	if err := EnsureStorage(); err != nil {
		logger.Warn("Failed to initialize system monitor storage", zap.Error(err))
		return false
	}

	runningMu.Lock()
	if running {
		runningMu.Unlock()
		return false
	}
	running = true
	runningMu.Unlock()

	stopSysMon = make(chan struct{})

	interval := time.Duration(cfg.Interval) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}

	history = make([]SystemMetric, 0, maxHistory)

	backfillAggregatedMetrics()

	go collectLoop(interval)

	printSystemMonitorInfo(interval)

	return true
}

// StopSystemMonitor 发送停止信号终止系统监控采集循环。
func StopSystemMonitor() {
	runningMu.Lock()
	defer runningMu.Unlock()
	if running && stopSysMon != nil {
		close(stopSysMon)
	}
}

// collectLoop 按指定间隔循环采集系统指标，维护小时聚合器，跨小时时将上一小时聚合结果落盘，
// 并触发日/月/年的上层聚合，同时执行阈值告警。
func collectLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("System monitor collection started", zap.Duration("interval", interval))

	for {
		select {
		case <-stopSysMon:
			flushHourlyAgg()
			return
		case <-ticker.C:
			metric, err := CollectMetrics()
			if err != nil {
				logger.Warn("Failed to collect system metrics", zap.Error(err))
				continue
			}

			addHistory(metric)

			currentHour := metric.Timestamp.Truncate(time.Hour)
			hourlyMu.Lock()
			if !hourlyAgg.hour.IsZero() && !hourlyAgg.hour.Equal(currentHour) {
				avg := hourlyAgg.toSystemMetric()
				sampleCount := hourlyAgg.cpuCount
				prevHour := hourlyAgg.hour
				hourlyAgg.reset(currentHour)
				hourlyAgg.add(metric)
				hourlyMu.Unlock()

				if err := RecordHourlyMetric(avg, sampleCount); err != nil {
					logger.Warn("Failed to record hourly metric", zap.Error(err))
				}
				logger.Info("Hourly metric flushed",
					zap.Time("hour", avg.Timestamp),
					zap.Int("samples", sampleCount))

				aggregateUpperTiers(prevHour, currentHour)
			} else {
				if hourlyAgg.hour.IsZero() {
					hourlyAgg.reset(currentHour)
				}
				hourlyAgg.add(metric)
				hourlyMu.Unlock()
			}

			alerts := CheckAlerts(metric)
			if len(alerts) > 0 {
				for _, a := range alerts {
					logger.Warn("System threshold exceeded",
						zap.String("metric", a.Metric),
						zap.Float64("value", a.Value),
						zap.Float64("threshold", a.Threshold))
				}

				DispatchSystemAlerts(alerts)
			}
		}
	}
}

// aggregateUpperTiers 当小时发生变化时，根据跨日/跨月/跨年的情况触发更高层的聚合。
func aggregateUpperTiers(prevHour, currentHour time.Time) {
	prevDay := prevHour.Truncate(24 * time.Hour)
	currDay := currentHour.Truncate(24 * time.Hour)

	if !prevDay.Equal(currDay) {
		aggregateDay(prevDay)
	}

	prevMonth := time.Date(prevHour.Year(), prevHour.Month(), 1, 0, 0, 0, 0, prevHour.Location())
	currMonth := time.Date(currentHour.Year(), currentHour.Month(), 1, 0, 0, 0, 0, currentHour.Location())

	if !prevMonth.Equal(currMonth) {
		aggregateMonth(prevMonth)
	}

	prevYear := time.Date(prevHour.Year(), 1, 1, 0, 0, 0, 0, prevHour.Location())
	currYear := time.Date(currentHour.Year(), 1, 1, 0, 0, 0, 0, currentHour.Location())

	if !prevYear.Equal(currYear) {
		aggregateYear(prevYear)
	}
}

// aggregateDay 按日聚合系统指标：读取当天所有小时记录并计算平均值写入日存储。
func aggregateDay(day time.Time) {
	dayStart := day
	dayEnd := day.Add(24 * time.Hour)
	metrics, err := loadMetrics(hourlyPath(), dayStart)
	if err != nil {
		logger.Warn("Failed to load hourly metrics for daily aggregation", zap.Error(err))
		return
	}

	var dayMetrics []SystemMetric
	for _, m := range metrics {
		if m.Timestamp.Before(dayEnd) {
			dayMetrics = append(dayMetrics, m)
		}
	}

	if len(dayMetrics) == 0 {
		return
	}

	if err := aggregateAndRecord(dailyPath(), dayMetrics); err != nil {
		logger.Warn("Failed to record daily metric", zap.Error(err))
	} else {
		logger.Info("Daily metric aggregated",
			zap.Time("day", day),
			zap.Int("hourly_records", len(dayMetrics)))
	}
}

// aggregateMonth 按月聚合系统指标：读取当月所有日记录并计算平均值写入月存储。
func aggregateMonth(month time.Time) {
	monthStart := month
	monthEnd := month.AddDate(0, 1, 0)
	metrics, err := loadMetrics(dailyPath(), monthStart)
	if err != nil {
		logger.Warn("Failed to load daily metrics for monthly aggregation", zap.Error(err))
		return
	}

	var monthMetrics []SystemMetric
	for _, m := range metrics {
		if m.Timestamp.Before(monthEnd) {
			monthMetrics = append(monthMetrics, m)
		}
	}

	if len(monthMetrics) == 0 {
		return
	}

	if err := aggregateAndRecord(monthlyPath(), monthMetrics); err != nil {
		logger.Warn("Failed to record monthly metric", zap.Error(err))
	} else {
		logger.Info("Monthly metric aggregated",
			zap.Time("month", month),
			zap.Int("daily_records", len(monthMetrics)))
	}
}

// aggregateYear 按年聚合系统指标：读取当年所有月记录并计算平均值写入年存储。
func aggregateYear(year time.Time) {
	yearStart := year
	yearEnd := year.AddDate(1, 0, 0)
	metrics, err := loadMetrics(monthlyPath(), yearStart)
	if err != nil {
		logger.Warn("Failed to load monthly metrics for yearly aggregation", zap.Error(err))
		return
	}

	var yearMetrics []SystemMetric
	for _, m := range metrics {
		if m.Timestamp.Before(yearEnd) {
			yearMetrics = append(yearMetrics, m)
		}
	}

	if len(yearMetrics) == 0 {
		return
	}

	if err := aggregateAndRecord(yearlyPath(), yearMetrics); err != nil {
		logger.Warn("Failed to record yearly metric", zap.Error(err))
	} else {
		logger.Info("Yearly metric aggregated",
			zap.Time("year", year),
			zap.Int("monthly_records", len(yearMetrics)))
	}
}

// backfillAggregatedMetrics 系统启动时回填历史缺失的日/月/年聚合记录。
func backfillAggregatedMetrics() {
	now := timeutil.Now()

	backfillDays(now)
	backfillMonths(now)
	backfillYears(now)
}

// backfillDays 回填缺失的日聚合记录，从最后一条日记录的下一天一直补到今天之前。
func backfillDays(now time.Time) {
	dailyMetrics, err := loadMetrics(dailyPath(), time.Time{})
	if err != nil || len(dailyMetrics) == 0 {
		return
	}

	lastDaily := dailyMetrics[len(dailyMetrics)-1].Timestamp.Truncate(24 * time.Hour)
	today := now.Truncate(24 * time.Hour)

	for d := lastDaily.Add(24 * time.Hour); d.Before(today); d = d.Add(24 * time.Hour) {
		aggregateDay(d)
	}
}

// backfillMonths 回填缺失的月聚合记录。
func backfillMonths(now time.Time) {
	monthlyMetrics, err := loadMetrics(monthlyPath(), time.Time{})
	if err != nil || len(monthlyMetrics) == 0 {
		return
	}

	lastMonthly := monthlyMetrics[len(monthlyMetrics)-1].Timestamp
	lastMonth := time.Date(lastMonthly.Year(), lastMonthly.Month(), 1, 0, 0, 0, 0, lastMonthly.Location())
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	for m := lastMonth.AddDate(0, 1, 0); m.Before(thisMonth); m = m.AddDate(0, 1, 0) {
		aggregateMonth(m)
	}
}

// backfillYears 回填缺失的年聚合记录。
func backfillYears(now time.Time) {
	yearlyMetrics, err := loadMetrics(yearlyPath(), time.Time{})
	if err != nil || len(yearlyMetrics) == 0 {
		return
	}

	lastYearly := yearlyMetrics[len(yearlyMetrics)-1].Timestamp
	lastYear := time.Date(lastYearly.Year(), 1, 1, 0, 0, 0, 0, lastYearly.Location())
	thisYear := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())

	for y := lastYear.AddDate(1, 0, 0); y.Before(thisYear); y = y.AddDate(1, 0, 0) {
		aggregateYear(y)
	}
}

// flushHourlyAgg 停止时将当前小时的聚合数据落盘，避免丢失采样数据。
func flushHourlyAgg() {
	hourlyMu.Lock()
	defer hourlyMu.Unlock()

	if hourlyAgg.hour.IsZero() || hourlyAgg.cpuCount == 0 {
		return
	}

	avg := hourlyAgg.toSystemMetric()
	sampleCount := hourlyAgg.cpuCount
	if err := RecordHourlyMetric(avg, sampleCount); err != nil {
		logger.Warn("Failed to flush hourly metric on shutdown", zap.Error(err))
	} else {
		logger.Info("Hourly metric flushed on shutdown",
			zap.Time("hour", avg.Timestamp),
			zap.Int("samples", sampleCount))
	}
	hourlyAgg.reset(time.Time{})
}

// addHistory 将新采集的指标追加到历史记录中，超过 maxHistory 时截断最旧数据。
func addHistory(metric SystemMetric) {
	historyMu.Lock()
	defer historyMu.Unlock()

	history = append(history, metric)
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
}

// GetHistory 获取历史指标记录的线程安全副本。
func GetHistory() []SystemMetric {
	historyMu.RLock()
	defer historyMu.RUnlock()
	result := make([]SystemMetric, len(history))
	copy(result, history)
	return result
}

// printSystemMonitorInfo 打印系统监控启动时的配置摘要信息。
func printSystemMonitorInfo(interval time.Duration) {
	cfg := config.GlobalConfig.SystemMon
	fmt.Printf("\n╔══════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║           系统资源监控已启动                            ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 采集间隔:   %-43v ║\n", interval)
	fmt.Printf("║ CPU阈值:    %-43s ║\n", fmt.Sprintf("%.0f%%", cfg.CPUThreshold))
	fmt.Printf("║ 内存阈值:   %-43s ║\n", fmt.Sprintf("%.0f%%", cfg.MemoryThreshold))
	fmt.Printf("║ 磁盘阈值:   %-43s ║\n", fmt.Sprintf("%.0f%%", cfg.DiskUsageThreshold))
	fmt.Printf("║ 网络下行(严重): %-41s ║\n", formatSpeed(cfg.NetworkDownThreshold))
	fmt.Printf("║ 网络下行(警告): %-41s ║\n", formatSpeed(cfg.NetworkDownWarnThreshold))
	fmt.Printf("║ 网络上行(严重): %-41s ║\n", formatSpeed(cfg.NetworkUpThreshold))
	fmt.Printf("║ 网络上行(警告): %-41s ║\n", formatSpeed(cfg.NetworkUpWarnThreshold))
	fmt.Printf("║ 图表生成:   %-43v ║\n", cfg.ChartEnabled)
	fmt.Printf("║ 邮件告警:   %-43v ║\n", cfg.EmailEnabled)
	fmt.Printf("╚══════════════════════════════════════════════════════════╝\n\n")
}

// PrintCurrentStatus 立即采集一次系统指标并打印当前状态快照。
func PrintCurrentStatus() {
	metric, err := CollectMetrics()
	if err != nil {
		fmt.Printf("采集失败: %v\n", err)
		return
	}

	fmt.Printf("\n╔══ 当前系统状态 ══╗\n")
	fmt.Printf("║ CPU:    %6.2f%%  ║\n", metric.CPUPercent)
	fmt.Printf("║ MEM:    %6.2f%%  ║", metric.MemoryPercent)
	fmt.Printf("║ DISK:   %6.2f%%  ║", metric.DiskPercent)
	fmt.Printf("║ NET↓:   %6.2f KB/s  ║", metric.NetDownKBps)
	fmt.Printf("║ NET↑:   %6.2f KB/s  ║", metric.NetUpKBps)
	fmt.Printf("╚══════════════════╝\n")
}

// GenerateAndSaveReport 基于最近 24 小时指标生成系统报告并保存到本地，返回报告文件路径。
func GenerateAndSaveReport() (string, error) {
	metrics, err := LoadRecentMetrics(24)
	if err != nil {
		return "", err
	}

	if len(metrics) == 0 {
		return "", fmt.Errorf("no data")
	}

	latest := metrics[len(metrics)-1]
	alerts := CheckAlerts(latest)
	return SaveSystemReport(metrics, alerts)
}