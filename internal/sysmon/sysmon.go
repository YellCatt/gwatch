package sysmon

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
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

func StartSystemMonitor() {
	cfg := config.GlobalConfig.SystemMon
	if !cfg.Enabled {
		logger.Info("System monitor is disabled")
		return
	}

	if err := EnsureStorage(); err != nil {
		logger.Error("Failed to initialize system monitor storage", zap.Error(err))
		return
	}

	runningMu.Lock()
	if running {
		runningMu.Unlock()
		return
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
	go dailyReportLoop()

	printSystemMonitorInfo(interval)

	SendSystemStatusEmail(nil)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-stopSysMon

	runningMu.Lock()
	running = false
	runningMu.Unlock()

	logger.Info("System monitor stopped")
}

func StopSystemMonitor() {
	runningMu.Lock()
	defer runningMu.Unlock()
	if running && stopSysMon != nil {
		close(stopSysMon)
	}
}

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
				logger.Error("Failed to collect system metrics", zap.Error(err))
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

				if err := SendAlertEmail(alerts); err != nil {
					logger.Warn("Failed to send system alert email", zap.Error(err))
				}
			}
		}
	}
}

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

func backfillAggregatedMetrics() {
	now := time.Now()

	backfillDays(now)
	backfillMonths(now)
	backfillYears(now)
}

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

func dailyReportLoop() {
	now := time.Now()
	nextReport := time.Date(now.Year(), now.Month(), now.Day(), 7, 0, 0, 0, now.Location())
	if now.After(nextReport) {
		nextReport = nextReport.Add(24 * time.Hour)
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-stopSysMon:
			return
		case <-ticker.C:
			now := time.Now()
			if now.After(nextReport) {
				if err := generateAndSendSystemReport(); err != nil {
					logger.Warn("Failed to generate system report", zap.Error(err))
				}
				nextReport = nextReport.Add(24 * time.Hour)
			}
		}
	}
}

func addHistory(metric SystemMetric) {
	historyMu.Lock()
	defer historyMu.Unlock()

	history = append(history, metric)
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
}

func GetHistory() []SystemMetric {
	historyMu.RLock()
	defer historyMu.RUnlock()
	result := make([]SystemMetric, len(history))
	copy(result, history)
	return result
}

func generateAndSendSystemReport() error {
	metrics, err := LoadRecentMetrics(24)
	if err != nil {
		return err
	}

	if len(metrics) == 0 {
		logger.Info("No system metrics data for report")
		return nil
	}

	latest := metrics[len(metrics)-1]
	alerts := CheckAlerts(latest)

	if config.GlobalConfig.SystemMon.ChartEnabled {
		reportPath, err := SaveSystemReport(metrics, alerts)
		if err != nil {
			logger.Warn("Failed to save system report", zap.Error(err))
		} else {
			logger.Info("System report saved", zap.String("path", reportPath))
		}
	}

	if config.GlobalConfig.SystemMon.EmailEnabled {
		if err := SendSystemStatusEmail(metrics); err != nil {
			logger.Warn("Failed to send system status email", zap.Error(err))
		}
	}

	return nil
}

func printSystemMonitorInfo(interval time.Duration) {
	fmt.Printf("\n╔══════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║           系统资源监控已启动                            ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 采集间隔:   %-43v ║\n", interval)
	fmt.Printf("║ CPU阈值:    %-43s ║\n", fmt.Sprintf("%.0f%%", config.GlobalConfig.SystemMon.CPUThreshold))
	fmt.Printf("║ 内存阈值:   %-43s ║\n", fmt.Sprintf("%.0f%%", config.GlobalConfig.SystemMon.MemoryThreshold))
	fmt.Printf("║ 磁盘阈值:   %-43s ║\n", fmt.Sprintf("%.0f%%", config.GlobalConfig.SystemMon.DiskUsageThreshold))
	fmt.Printf("║ 网络下行阈值: %-43s ║\n", formatSpeed(config.GlobalConfig.SystemMon.NetworkDownThreshold))
	fmt.Printf("║ 网络上行阈值: %-43s ║\n", formatSpeed(config.GlobalConfig.SystemMon.NetworkUpThreshold))
	fmt.Printf("║ 图表生成:   %-43v ║\n", config.GlobalConfig.SystemMon.ChartEnabled)
	fmt.Printf("║ 邮件告警:   %-43v ║\n", config.GlobalConfig.SystemMon.EmailEnabled)
	fmt.Printf("╚══════════════════════════════════════════════════════════╝\n\n")
}

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
