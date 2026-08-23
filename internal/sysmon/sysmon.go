package sysmon

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
	"gwatch/internal/util"
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

	logger.Info("系统监控已停止")
}

// setupSystemMonitor 初始化系统监控但不等待信号，由外部统一管理生命周期。
// 返回 false 表示系统监控未启用或初始化失败。
func setupSystemMonitor() bool {
	cfg := config.GlobalConfig.SystemMon
	if !cfg.Enabled {
		logger.Info("系统监控未启用")
		return false
	}

	if err := EnsureStorage(); err != nil {
		logger.Warn("初始化系统监控存储失败", zap.Error(err))
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

	logger.Info("系统监控采集已启动", zap.Duration("间隔", interval))

	for {
		select {
		case <-stopSysMon:
			flushHourlyAgg()
			return
		case <-ticker.C:
			metric, err := CollectMetrics()
			if err != nil {
				logger.Warn("采集系统指标失败", zap.Error(err))
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
					logger.Warn("记录小时指标失败", zap.Error(err))
				}
				logger.Info("小时指标已落盘",
					zap.Time("小时", avg.Timestamp),
					zap.Int("采样数", sampleCount))

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
					logger.Warn("系统阈值已超过",
						zap.String("指标", a.Metric),
						zap.Float64("当前值", a.Value),
						zap.Float64("阈值", a.Threshold))
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
		logger.Warn("加载小时指标用于日聚合失败", zap.Error(err))
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
		logger.Warn("记录日指标失败", zap.Error(err))
	} else {
		logger.Info("日指标已聚合",
			zap.Time("日期", day),
			zap.Int("小时记录数", len(dayMetrics)))
	}
}

// aggregateMonth 按月聚合系统指标：读取当月所有日记录并计算平均值写入月存储。
func aggregateMonth(month time.Time) {
	monthStart := month
	monthEnd := month.AddDate(0, 1, 0)
	metrics, err := loadMetrics(dailyPath(), monthStart)
	if err != nil {
		logger.Warn("加载日指标用于月聚合失败", zap.Error(err))
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
		logger.Warn("记录月指标失败", zap.Error(err))
	} else {
		logger.Info("月指标已聚合",
			zap.Time("月份", month),
			zap.Int("日记录数", len(monthMetrics)))
	}
}

// aggregateYear 按年聚合系统指标：读取当年所有月记录并计算平均值写入年存储。
func aggregateYear(year time.Time) {
	yearStart := year
	yearEnd := year.AddDate(1, 0, 0)
	metrics, err := loadMetrics(monthlyPath(), yearStart)
	if err != nil {
		logger.Warn("加载月指标用于年聚合失败", zap.Error(err))
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
		logger.Warn("记录年指标失败", zap.Error(err))
	} else {
		logger.Info("年指标已聚合",
			zap.Time("年份", year),
			zap.Int("月记录数", len(yearMetrics)))
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

// FlushHourlyAgg 将当前小时的聚合数据落盘，避免丢失采样数据。
// 在生成报告前调用，确保当前小时数据已完整写入存储。
func FlushHourlyAgg() {
	flushHourlyAgg()
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
		logger.Warn("关机时刷新小时指标失败", zap.Error(err))
	} else {
		logger.Info("关机时小时指标已落盘",
			zap.Time("小时", avg.Timestamp),
			zap.Int("采样数", sampleCount))
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
	fmt.Printf("║ CPU:    %6.2f%% (最高 %6.2f%%) ║\n", metric.CPUPercent, metric.CPUMaxPercent)
	fmt.Printf("║ MEM:    %6.2f%% (最高 %6.2f%%) ║\n", metric.MemoryPercent, metric.MemoryMaxPercent)
	fmt.Printf("║ DISK:   %6.2f%%  ║\n", metric.DiskPercent)
	fmt.Printf("║ NET↓:   %s (最高 %s) ║\n", util.FormatSpeed(metric.NetDownKBps), util.FormatSpeed(metric.NetDownMaxKBps))
	fmt.Printf("║ NET↑:   %s (最高 %s) ║\n", util.FormatSpeed(metric.NetUpKBps), util.FormatSpeed(metric.NetUpMaxKBps))
	if len(metric.Partitions) > 0 {
		fmt.Printf("║ 分区信息: ║\n")
		for _, p := range metric.Partitions {
			fmt.Printf("║   %s: %.2f%% (%s / %s) ║\n",
				p.MountPoint, p.Percent,
				util.FormatBytes(p.Used), util.FormatBytes(p.Total))
		}
	}
	topProcs := CollectAllProcesses()
	if len(topProcs) > 0 {
		cpuTop := SortProcesses(topProcs, SortByCPU)
		memTop := SortProcesses(topProcs, SortByMem)

		if len(cpuTop) > 5 {
			cpuTop = cpuTop[:5]
		}
		if len(memTop) > 5 {
			memTop = memTop[:5]
		}

		fmt.Printf("║ 进程占用 Top 5: ║\n")
		fmt.Printf("║   [CPU] ║\n")
		for _, p := range cpuTop {
			fmt.Printf("║     %-20s CPU:%5.2f%% MEM:%5.2f%% ║\n",
				p.Name, p.CPUPercent, p.MemPercent)
		}
		fmt.Printf("║   [MEM] ║\n")
		for _, p := range memTop {
			fmt.Printf("║     %-20s MEM:%5.2f%% CPU:%5.2f%% MEM:%s ║\n",
				p.Name, p.MemPercent, p.CPUPercent, util.FormatBytes(p.MemUsed))
		}
	}
	fmt.Printf("╚══════════════════╝\n")
}

// GenerateAndSaveReport 基于最近 24 小时指标生成系统报告并保存到本地，返回报告文件路径。
func GenerateAndSaveReport() (string, error) {
	metrics, err := LoadRecentMetrics(24)
	if err != nil {
		return "", err
	}

	if len(metrics) == 0 {
		return "", fmt.Errorf("暂无数据")
	}

	latest := metrics[len(metrics)-1]
	alerts := CheckAlerts(latest)
	return SaveSystemReport(metrics, alerts)
}
