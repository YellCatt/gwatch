package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/storage"
	"gwatch/internal/sysmon"
	"gwatch/internal/timeutil"
)

// GenerateReportFromStorage 根据指定周期与时间区间，从存储中加载告警汇总、监控汇总、
// 资源指标（采集器）和系统指标，构建完整的 Report 对象。
func GenerateReportFromStorage(period ReportPeriod, startDate, endDate time.Time) *Report {
	report := &Report{
		Period:      period,
		StartDate:   startDate.Format("2006-01-02"),
		EndDate:     endDate.Format("2006-01-02"),
		GeneratedAt: timeutil.Now(),
	}

	alertSummaries, err := storage.GetAlertSummaryByPeriod(startDate, endDate)
	if err != nil {
		logger.Warn("从存储获取告警汇总失败", zap.Error(err))
	} else {
		for _, summary := range alertSummaries {
			firstOccurrence, _ := time.Parse("2006-01-02 15:04:05", summary.FirstOccurrence)
			lastOccurrence, _ := time.Parse("2006-01-02 15:04:05", summary.LastOccurrence)

			report.AggregatedErrors = append(report.AggregatedErrors, AggregatedError{
				TaskID:          summary.TestCaseID,
				TaskDesc:        summary.TestCaseDesc,
				URL:             summary.URL,
				Method:          summary.Method,
				ExpectedStatus:  summary.ExpectedStatus,
				AlertLevel:      summary.AlertLevel,
				AlertCount:      int(summary.AlertCount),
				FirstOccurrence: firstOccurrence,
				LastOccurrence:  lastOccurrence,
				ErrorMsg:        summary.ErrorMsg,
			})
		}
		sortAggregatedErrors(report.AggregatedErrors)
	}

	alertCountMap := make(map[string]int)
	for _, e := range report.AggregatedErrors {
		key := e.TaskID + "|" + e.URL
		alertCountMap[key] += e.AlertCount
	}

	monitorSummaries, err := storage.GetMonitorSummaryByPeriod(startDate, endDate)
	if err != nil {
		logger.Warn("从存储获取监控汇总失败", zap.Error(err))
		return report
	}

	for _, summary := range monitorSummaries {
		report.TotalTasks += int(summary.TotalCount)
		report.SuccessTasks += int(summary.SuccessCount)
		report.FailedTasks += int(summary.FailedCount)

		var avgMS int64
		if summary.TotalCount > 0 {
			avgMS = summary.TotalDurationMS / summary.TotalCount
		}

		alertKey := summary.TestCaseID + "|" + summary.URL
		alertCount := alertCountMap[alertKey]

		report.InterfaceStats = append(report.InterfaceStats, InterfaceStat{
			TaskID:          summary.TestCaseID,
			TaskDesc:        summary.TestCaseDesc,
			URL:             summary.URL,
			Method:          summary.Method,
			TotalCount:      int(summary.TotalCount),
			SuccessCount:    int(summary.SuccessCount),
			FailedCount:     int(summary.FailedCount),
			AlertCount:      alertCount,
			TotalDurationMS: summary.TotalDurationMS,
			AvgDurationMS:   avgMS,
			MaxDurationMS:   summary.MaxDurationMS,
			LastFailureTime: summary.LastFailureTime,
		})
	}
	sortInterfaceStats(report.InterfaceStats)

	if config.GlobalConfig.Scraper.Enabled && len(config.GlobalConfig.Scraper.Targets) > 0 {
		loadResourceMetricsByPeriod(report, period, startDate, endDate)
	}

	systemAlerts, err := storage.GetSystemAlertsByPeriod(startDate, endDate)
	if err != nil {
		logger.Warn("从存储获取系统告警失败", zap.Error(err))
	} else {
		for _, a := range systemAlerts {
			report.SystemAlerts = append(report.SystemAlerts, SystemAlertItem{
				Metric:          a.Metric,
				MetricAlias:     a.MetricAlias,
				Value:           a.Value,
				Threshold:       a.Threshold,
				Unit:            a.Unit,
				AlertLevel:      a.AlertLevel,
				AlertCount:      a.AlertCount,
				FirstOccurrence: a.FirstOccurrence,
				LastOccurrence:  a.LastOccurrence,
				Message:         a.Message,
			})
		}
	}

	scraperAlerts, err := storage.GetScraperAlertsByPeriod(startDate, endDate)
	if err != nil {
		logger.Warn("从存储获取采集器告警失败", zap.Error(err))
	} else {
		for _, a := range scraperAlerts {
			report.ScraperAlerts = append(report.ScraperAlerts, ScraperAlertItem{
				TargetName:      a.TargetName,
				TargetURL:       a.TargetURL,
				MetricName:      a.MetricName,
				MetricAlias:     a.MetricAlias,
				Value:           a.Value,
				Threshold:       a.Threshold,
				Unit:            a.Unit,
				AlertLevel:      a.AlertLevel,
				AlertCount:      a.AlertCount,
				FirstOccurrence: a.FirstOccurrence,
				LastOccurrence:  a.LastOccurrence,
				Message:         a.Message,
			})
		}
	}

	if config.GlobalConfig.SystemMon.Enabled {
		report.SystemMetrics = loadSystemMetrics(period, startDate, endDate)
	}

	return report
}

// loadResourceMetricsByPeriod 根据报告周期加载对应类型的采集器资源指标数据。
func loadResourceMetricsByPeriod(report *Report, period ReportPeriod, startDate, endDate time.Time) {
	switch period {
	case PeriodDaily:
		report.HourlyMetrics = loadHourlyResourceMetrics(startDate, endDate)
	case PeriodWeekly:
		report.DailyMetrics = loadDailyResourceMetrics(startDate, endDate)
	case PeriodMonthly:
		report.DailyMetrics = loadDailyResourceMetrics(startDate, endDate)
	case PeriodYearly:
		report.MonthlyMetrics = loadMonthlyResourceMetrics(startDate, endDate)
	}
}

// loadHourlyResourceMetrics 加载指定时间区间的每小时采集器资源指标平均值。
func loadHourlyResourceMetrics(startDate, endDate time.Time) []HourlyResourceMetric {
	hourlyAvgs, err := storage.GetScraperMetricsHourlyAvg(startDate, endDate)
	if err != nil {
		logger.Warn("获取采集器小时平均指标失败", zap.Error(err))
		return nil
	}

	if len(hourlyAvgs) == 0 {
		return nil
	}

	type key struct {
		targetName string
		metricName string
	}

	metricMap := make(map[key]*HourlyResourceMetric)

	for _, avg := range hourlyAvgs {
		k := key{
			targetName: avg.TargetName,
			metricName: avg.MetricName,
		}

		if metricMap[k] == nil {
			metricMap[k] = &HourlyResourceMetric{
				TargetName:  avg.TargetName,
				MetricName:  avg.MetricName,
				MetricAlias: avg.MetricAlias,
				Unit:        avg.Unit,
				HourlyData:  make([]HourlyData, 24),
			}
			for i := range metricMap[k].HourlyData {
				metricMap[k].HourlyData[i].Hour = i
				metricMap[k].HourlyData[i].AvgValue = -1
			}
		}

		metricMap[k].HourlyData[avg.Hour].AvgValue = avg.AvgValue
	}

	var results []HourlyResourceMetric
	for _, m := range metricMap {
		results = append(results, *m)
	}

	return results
}

// loadDailyResourceMetrics 加载指定时间区间的每日采集器资源指标平均值。
func loadDailyResourceMetrics(startDate, endDate time.Time) []DailyResourceMetric {
	dailyAvgs, err := storage.GetScraperMetricsDailyAvg(startDate, endDate)
	if err != nil {
		logger.Warn("获取采集器日平均指标失败", zap.Error(err))
		return nil
	}

	if len(dailyAvgs) == 0 {
		return nil
	}

	daysInPeriod := int(endDate.Sub(startDate).Hours() / 24)

	type key struct {
		targetName string
		metricName string
	}

	metricMap := make(map[key]*DailyResourceMetric)

	for _, avg := range dailyAvgs {
		k := key{
			targetName: avg.TargetName,
			metricName: avg.MetricName,
		}

		if metricMap[k] == nil {
			metricMap[k] = &DailyResourceMetric{
				TargetName:  avg.TargetName,
				MetricName:  avg.MetricName,
				MetricAlias: avg.MetricAlias,
				Unit:        avg.Unit,
				DailyData:   make([]DailyData, daysInPeriod),
			}
			for i := range metricMap[k].DailyData {
				metricMap[k].DailyData[i].Day = i
				metricMap[k].DailyData[i].DayLabel = startDate.AddDate(0, 0, i).Format("01-02")
				metricMap[k].DailyData[i].AvgValue = -1
			}
		}

		if avg.Day >= 0 && avg.Day < daysInPeriod {
			metricMap[k].DailyData[avg.Day].AvgValue = avg.AvgValue
		}
	}

	var results []DailyResourceMetric
	for _, m := range metricMap {
		results = append(results, *m)
	}

	return results
}

// loadMonthlyResourceMetrics 加载指定时间区间的每月采集器资源指标平均值。
func loadMonthlyResourceMetrics(startDate, endDate time.Time) []MonthlyResourceMetric {
	monthlyAvgs, err := storage.GetScraperMetricsMonthlyAvg(startDate, endDate)
	if err != nil {
		logger.Warn("获取采集器月平均指标失败", zap.Error(err))
		return nil
	}

	if len(monthlyAvgs) == 0 {
		return nil
	}

	type key struct {
		targetName string
		metricName string
	}

	metricMap := make(map[key]*MonthlyResourceMetric)

	for _, avg := range monthlyAvgs {
		k := key{
			targetName: avg.TargetName,
			metricName: avg.MetricName,
		}

		if metricMap[k] == nil {
			metricMap[k] = &MonthlyResourceMetric{
				TargetName:  avg.TargetName,
				MetricName:  avg.MetricName,
				MetricAlias: avg.MetricAlias,
				Unit:        avg.Unit,
				MonthlyData: make([]MonthlyData, 12),
			}
			for i := range metricMap[k].MonthlyData {
				metricMap[k].MonthlyData[i].Month = i + 1
				metricMap[k].MonthlyData[i].MonthLabel = []string{"1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"}[i]
				metricMap[k].MonthlyData[i].AvgValue = -1
			}
		}

		if avg.Month >= 1 && avg.Month <= 12 {
			metricMap[k].MonthlyData[avg.Month-1].AvgValue = avg.AvgValue
		}
	}

	var results []MonthlyResourceMetric
	for _, m := range metricMap {
		results = append(results, *m)
	}

	return results
}

// sortAggregatedErrors 按告警级别和告警次数降序排列聚合错误列表。
func sortAggregatedErrors(errs []AggregatedError) {
	sort.SliceStable(errs, func(i, j int) bool {
		ri, rj := alertLevelRank(errs[i].AlertLevel), alertLevelRank(errs[j].AlertLevel)
		if ri != rj {
			return ri > rj
		}
		return errs[i].AlertCount > errs[j].AlertCount
	})
}

// sortInterfaceStats 按失败次数和总次数降序排列接口统计列表。
func sortInterfaceStats(stats []InterfaceStat) {
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].FailedCount != stats[j].FailedCount {
			return stats[i].FailedCount > stats[j].FailedCount
		}
		return stats[i].TotalCount > stats[j].TotalCount
	})
}

// alertLevelRank 返回告警级别的权重（CRITICAL=2, WARNING=1, 其他=0）。
func alertLevelRank(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case storage.AlertLevelCritical:
		return 2
	case storage.AlertLevelWarning:
		return 1
	default:
		return 0
	}
}

// alertLevelDisplay 返回告警级别的图标和显示文本。
func alertLevelDisplay(level string) (string, string) {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case storage.AlertLevelCritical:
		return "🚨", "严重"
	case storage.AlertLevelWarning:
		return "⚠️", "警告"
	default:
		return "🔔", level
	}
}

// SaveReport 生成报告内容并保存为文本文件，返回保存路径。
func (r *Report) SaveReport() (string, error) {
	reportDir := config.GlobalConfig.App.ReportDir
	if reportDir == "" {
		reportDir = "./reports"
	}

	subDir := filepath.Join(reportDir, PeriodNamesEn[r.Period])
	if err := os.MkdirAll(subDir, 0755); err != nil {
		logger.Error("创建报告目录失败", zap.Error(err))
		return "", err
	}

	filename := fmt.Sprintf("%s_report_%s_%s.txt", PeriodNamesEn[r.Period], r.StartDate, r.EndDate)
	filePath := filepath.Join(subDir, filename)

	content := r.GenerateContent()
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		logger.Error("保存报告失败", zap.String("文件", filePath), zap.Error(err))
		return "", err
	}

	logger.Info("报告已保存", zap.String("文件", filePath))
	return filePath, nil
}

// PrepareReportEmail 准备报告邮件的主题和正文，由调用方负责发送。
func (r *Report) PrepareReportEmail() (string, string) {
	totalAlerts := 0
	for _, e := range r.AggregatedErrors {
		totalAlerts += e.AlertCount
	}
	for _, a := range r.SystemAlerts {
		totalAlerts += int(a.AlertCount)
	}
	for _, a := range r.ScraperAlerts {
		totalAlerts += int(a.AlertCount)
	}

	subject := fmt.Sprintf("[gwatch] %s运维报告 - %s ~ %s", PeriodNames[r.Period], r.StartDate, r.EndDate)
	if totalAlerts > 0 {
		subject = fmt.Sprintf("[gwatch] %s运维报告 - %s ~ %s（告警 %d 次）", PeriodNames[r.Period], r.StartDate, r.EndDate, totalAlerts)
	}
	body := r.GenerateContent()

	logger.Info("正在准备报告邮件", zap.String("周期", string(r.Period)))
	return subject, body
}

// GenerateContent 根据报告周期分发到对应的内容生成方法。
func (r *Report) GenerateContent() string {
	switch r.Period {
	case PeriodStartup:
		return r.GenerateStartupContent()
	case PeriodDaily:
		return r.GenerateDailyContent()
	case PeriodWeekly:
		return r.GenerateWeeklyContent()
	case PeriodMonthly:
		return r.GenerateMonthlyContent()
	case PeriodYearly:
		return r.GenerateYearlyContent()
	default:
		return executeTemplate("base", buildBaseData(r))
	}
}

// loadSystemMetrics 采集当前系统指标，加载报告周期内的历史数据生成趋势图表，返回 SystemMetricsSnapshot。
// 根据报告周期选择不同粒度的数据源：日报使用小时级，周报使用日级，月报使用周级，年报使用月级。
// 当首选数据源数据为空时，自动回退到更细粒度的数据源，确保图表始终可用。
func loadSystemMetrics(period ReportPeriod, startDate, endDate time.Time) *SystemMetricsSnapshot {
	current, err := sysmon.CollectMetrics()
	if err != nil {
		logger.Error("报告采集系统指标失败", zap.Error(err))
		return nil
	}

	snapshot := &SystemMetricsSnapshot{
		CPUPercent:       current.CPUPercent,
		CPUMaxPercent:    current.CPUPercent,
		MemoryPercent:    current.MemoryPercent,
		MemoryMaxPercent: current.MemoryPercent,
		DiskPercent:      current.DiskPercent,
		NetDownKBps:      current.NetDownKBps,
		NetUpKBps:        current.NetUpKBps,
		NetDownMaxKBps:   current.NetDownKBps,
		NetUpMaxKBps:     current.NetUpKBps,
		MemUsedBytes:     current.MemoryUsed,
		MemTotalBytes:    current.MemoryTotal,
		DiskUsedBytes:    current.DiskUsed,
		DiskTotalBytes:   current.DiskTotal,
	}

	for _, p := range current.Partitions {
		snapshot.Partitions = append(snapshot.Partitions, PartitionInfo{
			MountPoint: p.MountPoint,
			Fstype:     p.Fstype,
			Percent:    p.Percent,
			UsedBytes:  p.Used,
			TotalBytes: p.Total,
		})
	}

	sysmon.FlushHourlyAgg()

	metrics, labelFormat := loadMetricsWithFallback(period, startDate, endDate)

	if len(metrics) == 0 {
		logger.Info("暂无系统指标数据，跳过图表生成")
		return snapshot
	}

	chartWidth := len(metrics)
	if chartWidth > 20 {
		chartWidth = 20
	}

	labels := make([]string, len(metrics))
	cpuData := make([]float64, len(metrics))
	cpuMaxData := make([]float64, len(metrics))
	memData := make([]float64, len(metrics))
	memMaxData := make([]float64, len(metrics))
	diskData := make([]float64, len(metrics))
	netDownData := make([]float64, len(metrics))
	netUpData := make([]float64, len(metrics))
	netDownMaxData := make([]float64, len(metrics))
	netUpMaxData := make([]float64, len(metrics))

	for i, m := range metrics {
		labels[i] = m.Timestamp.Format(labelFormat)
		cpuData[i] = m.CPUPercent
		cpuMaxData[i] = m.CPUMaxPercent
		memData[i] = m.MemoryPercent
		memMaxData[i] = m.MemoryMaxPercent
		diskData[i] = m.DiskPercent
		netDownData[i] = m.NetDownKBps
		netUpData[i] = m.NetUpKBps
		netDownMaxData[i] = m.NetDownMaxKBps
		netUpMaxData[i] = m.NetUpMaxKBps

		if m.CPUMaxPercent > snapshot.CPUMaxPercent {
			snapshot.CPUMaxPercent = m.CPUMaxPercent
		}
		if m.MemoryMaxPercent > snapshot.MemoryMaxPercent {
			snapshot.MemoryMaxPercent = m.MemoryMaxPercent
		}
		if m.NetDownMaxKBps > snapshot.NetDownMaxKBps {
			snapshot.NetDownMaxKBps = m.NetDownMaxKBps
		}
		if m.NetUpMaxKBps > snapshot.NetUpMaxKBps {
			snapshot.NetUpMaxKBps = m.NetUpMaxKBps
		}
	}

	cfg := config.GlobalConfig.SystemMon
	snapshot.CPUChart = sysmon.GenerateASCIIChartWithTime(cpuData, chartWidth, "%", labels, cfg.CPUThreshold)
	snapshot.CPUMaxChart = sysmon.GenerateASCIIChartWithTime(cpuMaxData, chartWidth, "%", labels, cfg.CPUThreshold)
	snapshot.MemoryChart = sysmon.GenerateASCIIChartWithTime(memData, chartWidth, "%", labels, cfg.MemoryThreshold)
	snapshot.MemoryMaxChart = sysmon.GenerateASCIIChartWithTime(memMaxData, chartWidth, "%", labels, cfg.MemoryThreshold)
	snapshot.DiskChart = sysmon.GenerateASCIIChartWithTime(diskData, chartWidth, "%", labels, cfg.DiskUsageThreshold)
	snapshot.NetDownChart = sysmon.GenerateASCIIChartWithTime(netDownData, chartWidth, "KB/s", labels, cfg.NetworkDownThreshold)
	snapshot.NetUpChart = sysmon.GenerateASCIIChartWithTime(netUpData, chartWidth, "KB/s", labels, cfg.NetworkUpThreshold)
	snapshot.NetDownMaxChart = sysmon.GenerateASCIIChartWithTime(netDownMaxData, chartWidth, "KB/s", labels, cfg.NetworkDownThreshold)
	snapshot.NetUpMaxChart = sysmon.GenerateASCIIChartWithTime(netUpMaxData, chartWidth, "KB/s", labels, cfg.NetworkUpThreshold)

	snapshot.StartTime = metrics[0].Timestamp.Format("2006-01-02 15:04")
	snapshot.EndTime = metrics[len(metrics)-1].Timestamp.Format("2006-01-02 15:04")

	return snapshot
}

// loadMetricsWithFallback 根据报告周期加载系统指标数据。
// 对于年度报告，若月级数据不足则从日级/小时级数据按月聚合，确保始终展示月度维度的图表。
// 对于周/月报告，回退到更细粒度的数据源（粒度降级在短周期场景下可接受）。
func loadMetricsWithFallback(period ReportPeriod, startDate, endDate time.Time) ([]sysmon.SystemMetric, string) {
	switch period {
	case PeriodYearly:
		return loadYearlyMetricsWithAggregation(startDate, endDate)
	case PeriodWeekly:
		return loadWeeklyMetricsWithFallback(startDate, endDate)
	case PeriodMonthly:
		return loadMonthlyMetricsWithFallback(startDate, endDate)
	default:
		metrics, err := sysmon.LoadMetricsByRange(startDate, endDate)
		if err != nil {
			logger.Warn("加载小时级指标失败", zap.Error(err))
			return nil, "01-02 15:04"
		}
		return metrics, "01-02 15:04"
	}
}

// loadYearlyMetricsWithAggregation 加载年度报告的系统指标数据。
// 优先使用月级 CSV 数据；若数据不足，则从日级/小时级数据按月聚合，确保月度维度的图表始终可用。
// 最终将数据填充至 12 个月（缺失月份复用已存在的数据），保证图表宽度与日/周/月报一致。
func loadYearlyMetricsWithAggregation(startDate, endDate time.Time) ([]sysmon.SystemMetric, string) {
	metrics, err := sysmon.LoadMonthlyMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载月级指标失败", zap.Error(err))
	}
	if len(metrics) >= 2 {
		logger.Info(fmt.Sprintf("使用月级数据源生成年报图表 (数据点: %d)", len(metrics)))
		return padYearlyMetrics(metrics), "2006-01"
	}

	logger.Info(fmt.Sprintf("月级数据不足 (仅 %d 条)，回退到日级数据按月聚合", len(metrics)))
	dailyMetrics, err := sysmon.LoadDailyMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载日级指标失败", zap.Error(err))
	}
	if len(dailyMetrics) > 0 {
		aggregated := aggregateMetricsByMonth(dailyMetrics)
		if len(aggregated) >= 1 {
			logger.Info(fmt.Sprintf("日级数据按月聚合成功 (聚合后: %d 个月)", len(aggregated)))
			return padYearlyMetrics(aggregated), "2006-01"
		}
	}

	logger.Info("日级数据也不足，回退到小时级数据按月聚合")
	hourlyMetrics, err := sysmon.LoadMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载小时级指标失败", zap.Error(err))
	}
	if len(hourlyMetrics) > 0 {
		aggregated := aggregateMetricsByMonth(hourlyMetrics)
		if len(aggregated) >= 1 {
			logger.Info(fmt.Sprintf("小时级数据按月聚合成功 (聚合后: %d 个月)", len(aggregated)))
			return padYearlyMetrics(aggregated), "2006-01"
		}
	}

	logger.Info("所有数据源均无数据，无法生成年报图表")
	return nil, "2006-01"
}

// padYearlyMetrics 将年度系统指标补齐到 12 个月。
// 缺失的月份使用已存在的数据点填充，保证图表宽度与其它报告一致。
func padYearlyMetrics(metrics []sysmon.SystemMetric) []sysmon.SystemMetric {
	if len(metrics) >= 12 {
		return metrics
	}
	if len(metrics) == 0 {
		return metrics
	}

	year := metrics[0].Timestamp.Year()
	monthMap := make(map[time.Month]sysmon.SystemMetric, len(metrics))
	for _, m := range metrics {
		monthMap[m.Timestamp.Month()] = m
	}

	base := metrics[0]
	padded := make([]sysmon.SystemMetric, 0, 12)
	for month := time.Month(1); month <= 12; month++ {
		if m, ok := monthMap[month]; ok {
			padded = append(padded, m)
		} else {
			filler := base
			filler.Timestamp = time.Date(year, month, 1, 0, 0, 0, 0, base.Timestamp.Location())
			padded = append(padded, filler)
		}
	}
	return padded
}

// padWeeklyMetrics 将周报系统指标补齐到 7 天，保证图表宽度稳定。
// 缺失的日期使用已存在的数据点填充。
func padWeeklyMetrics(metrics []sysmon.SystemMetric, startDate time.Time) []sysmon.SystemMetric {
	const target = 7
	if len(metrics) >= target || len(metrics) == 0 {
		return metrics
	}

	base := metrics[0]
	slotMap := make(map[int]sysmon.SystemMetric, len(metrics))
	for _, m := range metrics {
		slot := int(m.Timestamp.Sub(startDate).Hours() / 24)
		if slot >= 0 && slot < target {
			slotMap[slot] = m
		}
	}

	padded := make([]sysmon.SystemMetric, 0, target)
	for i := 0; i < target; i++ {
		if m, ok := slotMap[i]; ok {
			padded = append(padded, m)
		} else {
			filler := base
			filler.Timestamp = startDate.AddDate(0, 0, i)
			padded = append(padded, filler)
		}
	}
	return padded
}

// padMonthlyMetrics 将月报系统指标补齐到当月天数，保证图表宽度稳定。
// 缺失的日期使用已存在的数据点填充。
func padMonthlyMetrics(metrics []sysmon.SystemMetric, startDate time.Time) []sysmon.SystemMetric {
	year, month, _ := startDate.Date()
	daysInMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, startDate.Location()).Day()

	if len(metrics) >= daysInMonth || len(metrics) == 0 {
		return metrics
	}

	base := metrics[0]
	dayMap := make(map[int]sysmon.SystemMetric, len(metrics))
	for _, m := range metrics {
		dayMap[m.Timestamp.Day()] = m
	}

	padded := make([]sysmon.SystemMetric, 0, daysInMonth)
	for day := 1; day <= daysInMonth; day++ {
		if m, ok := dayMap[day]; ok {
			padded = append(padded, m)
		} else {
			filler := base
			filler.Timestamp = time.Date(year, month, day, 0, 0, 0, 0, startDate.Location())
			padded = append(padded, filler)
		}
	}
	return padded
}

// loadWeeklyMetricsWithFallback 加载周报的系统指标数据，使用日级数据源，回退到小时级。
// 返回前将数据补齐到 7 天，保证图表宽度稳定。
func loadWeeklyMetricsWithFallback(startDate, endDate time.Time) ([]sysmon.SystemMetric, string) {
	metrics, err := sysmon.LoadDailyMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载日级指标失败", zap.Error(err))
	}
	if len(metrics) > 0 {
		logger.Info(fmt.Sprintf("使用日级数据源生成周报图表 (数据点: %d)", len(metrics)))
		return padWeeklyMetrics(metrics, startDate), "01-02"
	}

	metrics, err = sysmon.LoadMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载小时级指标失败", zap.Error(err))
	}
	if len(metrics) > 0 {
		logger.Info(fmt.Sprintf("使用小时级数据源生成周报图表 (数据点: %d)", len(metrics)))
		return padWeeklyMetrics(metrics, startDate), "01-02 15:04"
	}

	logger.Info("所有数据源均无数据，无法生成周报图表")
	return nil, "01-02"
}

// loadMonthlyMetricsWithFallback 加载月报的系统指标数据，使用周级数据源，回退到日级，再回退到小时级。
// 返回前将数据补齐到当月天数，保证图表宽度稳定。
func loadMonthlyMetricsWithFallback(startDate, endDate time.Time) ([]sysmon.SystemMetric, string) {
	metrics, err := sysmon.LoadWeeklyMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载周级指标失败", zap.Error(err))
	}
	if len(metrics) > 0 {
		logger.Info(fmt.Sprintf("使用周级数据源生成月报图表 (数据点: %d)", len(metrics)))
		return padMonthlyMetrics(metrics, startDate), "01-02"
	}

	metrics, err = sysmon.LoadDailyMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载日级指标失败", zap.Error(err))
	}
	if len(metrics) > 0 {
		logger.Info(fmt.Sprintf("使用日级数据源生成月报图表 (数据点: %d)", len(metrics)))
		return padMonthlyMetrics(metrics, startDate), "01-02"
	}

	metrics, err = sysmon.LoadMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载小时级指标失败", zap.Error(err))
	}
	if len(metrics) > 0 {
		logger.Info(fmt.Sprintf("使用小时级数据源生成月报图表 (数据点: %d)", len(metrics)))
		return padMonthlyMetrics(metrics, startDate), "01-02 15:04"
	}

	logger.Info("所有数据源均无数据，无法生成月报图表")
	return nil, "01-02"
}

// partitionAggregate 分区聚合数据
type partitionAggregate struct {
	Name       string
	MountPoint string
	Fstype     string
	percentSum float64
	usedSum    uint64
	totalSum   uint64
	count      int
}

// aggregateMetricsByMonth 将系统指标按月聚合，返回每月一条记录。
// 用于年度报告在月级 CSV 数据不足时，从日级或小时级数据动态聚合出月度数据。
func aggregateMetricsByMonth(metrics []sysmon.SystemMetric) []sysmon.SystemMetric {
	if len(metrics) == 0 {
		return nil
	}

	type monthKey struct {
		year  int
		month time.Month
	}

	type aggState struct {
		cpuSum, memSum, diskSum, netDownSum, netUpSum float64
		cpuMax, memMax, netDownMax, netUpMax          float64
		diskReadSum, diskWriteSum                     float64
		count                                         int
		timestamp                                     time.Time
		partitions                                    map[string]*partitionAggregate
	}

	aggMap := make(map[monthKey]*aggState)

	for _, m := range metrics {
		key := monthKey{year: m.Timestamp.Year(), month: m.Timestamp.Month()}
		if aggMap[key] == nil {
			aggMap[key] = &aggState{
				timestamp:  time.Date(key.year, key.month, 1, 0, 0, 0, 0, m.Timestamp.Location()),
				partitions: make(map[string]*partitionAggregate),
			}
		}
		agg := aggMap[key]
		agg.cpuSum += m.CPUPercent
		agg.memSum += m.MemoryPercent
		agg.diskSum += m.DiskPercent
		agg.netDownSum += m.NetDownKBps
		agg.netUpSum += m.NetUpKBps
		agg.diskReadSum += m.DiskReadKBps
		agg.diskWriteSum += m.DiskWriteKBps
		agg.count++

		if m.CPUMaxPercent > agg.cpuMax {
			agg.cpuMax = m.CPUMaxPercent
		}
		if m.MemoryMaxPercent > agg.memMax {
			agg.memMax = m.MemoryMaxPercent
		}
		if m.NetDownMaxKBps > agg.netDownMax {
			agg.netDownMax = m.NetDownMaxKBps
		}
		if m.NetUpMaxKBps > agg.netUpMax {
			agg.netUpMax = m.NetUpMaxKBps
		}

		for _, p := range m.Partitions {
			pk := p.Name + "|" + p.MountPoint
			if agg.partitions[pk] == nil {
				agg.partitions[pk] = &partitionAggregate{
					Name: p.Name, MountPoint: p.MountPoint, Fstype: p.Fstype,
				}
			}
			pa := agg.partitions[pk]
			pa.percentSum += p.Percent
			pa.usedSum += p.Used
			pa.totalSum += p.Total
			pa.count++
		}
	}

	results := make([]sysmon.SystemMetric, 0, len(aggMap))
	for _, agg := range aggMap {
		result := sysmon.SystemMetric{
			CPUPercent:       avgFloat64(agg.cpuSum, agg.count),
			CPUMaxPercent:    agg.cpuMax,
			MemoryPercent:    avgFloat64(agg.memSum, agg.count),
			MemoryMaxPercent: agg.memMax,
			DiskPercent:      avgFloat64(agg.diskSum, agg.count),
			NetDownKBps:      avgFloat64(agg.netDownSum, agg.count),
			NetUpKBps:        avgFloat64(agg.netUpSum, agg.count),
			NetDownMaxKBps:   agg.netDownMax,
			NetUpMaxKBps:     agg.netUpMax,
			DiskReadKBps:     avgFloat64(agg.diskReadSum, agg.count),
			DiskWriteKBps:    avgFloat64(agg.diskWriteSum, agg.count),
			Timestamp:        agg.timestamp,
		}

		for _, pa := range agg.partitions {
			if pa.count > 0 {
				result.Partitions = append(result.Partitions, sysmon.DiskPartition{
					Name:       pa.Name,
					MountPoint: pa.MountPoint,
					Fstype:     pa.Fstype,
					Percent:    avgFloat64(pa.percentSum, pa.count),
					Used:       pa.usedSum / uint64(pa.count),
					Total:      pa.totalSum / uint64(pa.count),
				})
			}
		}

		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	return results
}

// avgFloat64 安全计算浮点数平均值，count 为 0 时返回 0。
func avgFloat64(sum float64, count int) float64 {
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}