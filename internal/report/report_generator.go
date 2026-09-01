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

	logger.Info("生成报告：开始加载数据",
		zap.String("周期", string(period)),
		zap.String("起始", report.StartDate),
		zap.String("结束", report.EndDate),
	)

	alertSummaries, err := storage.GetAlertSummaryByPeriod(startDate, endDate)
	if err != nil {
		logger.Warn("从存储获取告警汇总失败", zap.Error(err))
	} else {
		for _, summary := range alertSummaries {
			// 存储中写入的是东八区墙钟串（timeutil.FormatDateTime 基于 Asia/Shanghai），
			// 必须用同一时区解析，否则 time.Parse 按 UTC 解析会导致时间偏移 8 小时。
			firstOccurrence, _ := time.ParseInLocation("2006-01-02 15:04:05", summary.FirstOccurrence, timeutil.Location())
			lastOccurrence, _ := time.ParseInLocation("2006-01-02 15:04:05", summary.LastOccurrence, timeutil.Location())

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
	logger.Info("报告加载：告警汇总", zap.Int("条数", len(report.AggregatedErrors)))

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
	logger.Info("报告加载：监控汇总原始条数", zap.Int("条数", len(monitorSummaries)))

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
	logger.Info("报告加载：接口统计",
		zap.Int("接口数", len(report.InterfaceStats)),
		zap.Int("总请求", report.TotalTasks),
		zap.Int("成功", report.SuccessTasks),
		zap.Int("失败", report.FailedTasks),
	)

	if config.GlobalConfig.Scraper.Enabled && len(config.GlobalConfig.Scraper.Targets) > 0 {
		logger.Info("报告加载：采集器已启用，准备加载资源指标",
			zap.Int("目标数", len(config.GlobalConfig.Scraper.Targets)),
		)
		loadResourceMetricsByPeriod(report, period, startDate, endDate)
	} else {
		logger.Info("报告加载：采集器未启用或无目标，跳过资源指标",
			zap.Bool("Enabled", config.GlobalConfig.Scraper.Enabled),
			zap.Int("Targets", len(config.GlobalConfig.Scraper.Targets)),
		)
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
	logger.Info("报告加载：系统告警", zap.Int("条数", len(report.SystemAlerts)))

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
	logger.Info("报告加载：采集器告警", zap.Int("条数", len(report.ScraperAlerts)))

	if config.GlobalConfig.SystemMon.Enabled {
		logger.Info("报告加载：系统监控已启用，准备加载系统指标")
		report.SystemMetrics = loadSystemMetrics(period, startDate, endDate)
	} else {
		logger.Info("报告加载：系统监控未启用，跳过系统指标")
	}

	logger.Info("报告数据加载完成",
		zap.Int("聚合错误", len(report.AggregatedErrors)),
		zap.Int("接口统计", len(report.InterfaceStats)),
		zap.Int("系统告警", len(report.SystemAlerts)),
		zap.Int("采集器告警", len(report.ScraperAlerts)),
		zap.Bool("有系统指标", report.SystemMetrics != nil),
		zap.Int("小时级指标", len(report.HourlyMetrics)),
		zap.Int("日级指标", len(report.DailyMetrics)),
		zap.Int("月级指标", len(report.MonthlyMetrics)),
	)

	return report
}

// loadResourceMetricsByPeriod 根据报告周期加载对应类型的采集器资源指标数据。
// 不同周期选择不同粒度的指标：
//   - 日报：加载小时级指标（24 个数据点）
//   - 周报：加载日级指标（7 个数据点）
//   - 月报：加载日级指标（当月天数，通常 28-31 个数据点）
//   - 年报：加载月级指标（12 个数据点）
//
// 加载结果写入 report 对应字段，供模板渲染使用。
func loadResourceMetricsByPeriod(report *Report, period ReportPeriod, startDate, endDate time.Time) {
	switch period {
	case PeriodDaily:
		report.HourlyMetrics = loadHourlyResourceMetrics(startDate, endDate)
		logger.Info("报告资源指标：加载小时级", zap.Int("指标组数", len(report.HourlyMetrics)))
	case PeriodWeekly:
		report.DailyMetrics = loadDailyResourceMetrics(startDate, endDate)
		logger.Info("报告资源指标：加载周级（日级粒度）", zap.Int("指标组数", len(report.DailyMetrics)))
	case PeriodMonthly:
		report.DailyMetrics = loadDailyResourceMetrics(startDate, endDate)
		logger.Info("报告资源指标：加载月级（日级粒度）", zap.Int("指标组数", len(report.DailyMetrics)))
	case PeriodYearly:
		report.MonthlyMetrics = loadMonthlyResourceMetrics(startDate, endDate)
		logger.Info("报告资源指标：加载年级（月级粒度）", zap.Int("指标组数", len(report.MonthlyMetrics)))
	}
}

// loadHourlyResourceMetrics 加载指定时间区间的每小时采集器资源指标平均值。
// 以 (targetName, metricName) 为 key 聚合，为每个指标预分配 24 个时段槽位（0-23 时），
// 无数据的槽位保留 -1 作为哨兵值，供图表生成时跳过。
// 返回的切片包含所有 (目标×指标) 组合的小时级数据。
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
// 以 (targetName, metricName) 为 key 聚合，根据时间区间天数预分配对应数量的日期槽位，
// 无数据的槽位保留 -1 作为哨兵值。返回的切片包含所有 (目标×指标) 组合的日级数据。
func loadDailyResourceMetrics(startDate, endDate time.Time) []DailyResourceMetric {
	dailyAvgs, err := storage.GetScraperMetricsDailyAvg(startDate, endDate)
	if err != nil {
		logger.Warn("获取采集器日平均指标失败", zap.Error(err))
		return nil
	}

	if len(dailyAvgs) == 0 {
		logger.Debug("日级资源指标：无数据",
			zap.Time("起始", startDate),
			zap.Time("结束", endDate),
		)
		return nil
	}

	daysInPeriod := int(endDate.Sub(startDate).Hours() / 24)
	logger.Debug("日级资源指标：原始聚合数据",
		zap.Int("聚合条数", len(dailyAvgs)),
		zap.Int("周期天数", daysInPeriod),
	)

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
// 以 (targetName, metricName) 为 key 聚合，为每个指标预分配 12 个月份槽位（1月-12月），
// 无数据的槽位保留 -1 作为哨兵值。返回的切片包含所有 (目标×指标) 组合的月级数据。
func loadMonthlyResourceMetrics(startDate, endDate time.Time) []MonthlyResourceMetric {
	monthlyAvgs, err := storage.GetScraperMetricsMonthlyAvg(startDate, endDate)
	if err != nil {
		logger.Warn("获取采集器月平均指标失败", zap.Error(err))
		return nil
	}

	if len(monthlyAvgs) == 0 {
		logger.Debug("月级资源指标：无数据",
			zap.Time("起始", startDate),
			zap.Time("结束", endDate),
		)
		return nil
	}

	logger.Debug("月级资源指标：原始聚合数据",
		zap.Int("聚合条数", len(monthlyAvgs)),
		zap.Time("起始", startDate),
		zap.Time("结束", endDate),
	)

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
//
// 处理流程：
//  1. 采集当前实时指标作为快照的基础值；
//  2. 根据报告周期选择数据源并按时间槽位对齐（日报 24 小时、周报 7 天、月报当月天数、年报 12 个月），
//     数据不足时回退到更细粒度重新聚合；没有采集记录的槽位标记为无效；
//  3. 从历史数据中提取 CPU、内存、磁盘、网络上下行等指标序列；
//  4. 计算整个周期内的最大值（CPU 峰值、内存峰值、网络峰值等），只统计有效数据点；
//  5. 使用 ASCII 图表生成函数为每项指标生成带时间标签和阈值线的趋势图，
//     平均值序列分桶取平均、峰值序列分桶取最大，无效数据点直接跳过不绘制。
//
// 图表宽度由槽位数决定，上限 20 列，保证不同周期的图表视觉宽度一致。
// 有效数据点不足 20 个时按实际点数绘制，不会复制填充出虚假的趋势。
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

	series := loadMetricsWithFallback(period, startDate, endDate)
	metrics, valid := series.metrics, series.valid
	logger.Info("系统指标加载完成",
		zap.String("周期", string(period)),
		zap.Int("槽位数", len(metrics)),
		zap.Int("有效数据点", series.validCount()),
		zap.String("标签格式", series.format),
	)

	if !series.hasData() {
		// 周期内没有任何历史数据（例如年报统计的上一年度尚未开始采集）时，
		// 仍然要绘制图表：改用等长的全无效槽位序列，图表内部会输出"无有效数据"占位，
		// 保证报告结构完整，而不是让整块图表消失。
		logger.Info("周期内暂无系统指标历史数据，图表以无数据占位绘制",
			zap.String("周期", string(period)),
			zap.String("起始", startDate.Format("2006-01-02 15:04")),
			zap.String("结束", endDate.Format("2006-01-02 15:04")),
		)
		series = emptyMetricSeries(period, startDate)
		metrics, valid = series.metrics, series.valid
	}

	chartWidth := len(metrics)
	if period == PeriodDaily {
		chartWidth = 24
	} else if chartWidth > 20 {
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

	firstIdx, lastIdx := -1, -1
	for i, m := range metrics {
		labels[i] = m.Timestamp.Format(series.format)
		if !valid[i] {
			continue
		}
		if firstIdx < 0 {
			firstIdx = i
		}
		lastIdx = i

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
	// 平均/瞬时值序列分桶取平均，峰值序列分桶取最大，两者语义不同不能混用。
	snapshot.CPUChart = sysmon.GenerateASCIIChartWithTimeEx(cpuData, valid, sysmon.AggAvg, chartWidth, "%", labels, cfg.CPUThreshold)
	snapshot.CPUMaxChart = sysmon.GenerateASCIIChartWithTimeEx(cpuMaxData, valid, sysmon.AggMax, chartWidth, "%", labels, cfg.CPUThreshold)
	snapshot.MemoryChart = sysmon.GenerateASCIIChartWithTimeEx(memData, valid, sysmon.AggAvg, chartWidth, "%", labels, cfg.MemoryThreshold)
	snapshot.MemoryMaxChart = sysmon.GenerateASCIIChartWithTimeEx(memMaxData, valid, sysmon.AggMax, chartWidth, "%", labels, cfg.MemoryThreshold)
	snapshot.DiskChart = sysmon.GenerateASCIIChartWithTimeEx(diskData, valid, sysmon.AggAvg, chartWidth, "%", labels, cfg.DiskUsageThreshold)
	snapshot.NetDownChart = sysmon.GenerateASCIIChartWithTimeEx(netDownData, valid, sysmon.AggAvg, chartWidth, "KB/s", labels, cfg.NetworkDownThreshold)
	snapshot.NetUpChart = sysmon.GenerateASCIIChartWithTimeEx(netUpData, valid, sysmon.AggAvg, chartWidth, "KB/s", labels, cfg.NetworkUpThreshold)
	snapshot.NetDownMaxChart = sysmon.GenerateASCIIChartWithTimeEx(netDownMaxData, valid, sysmon.AggMax, chartWidth, "KB/s", labels, cfg.NetworkDownThreshold)
	snapshot.NetUpMaxChart = sysmon.GenerateASCIIChartWithTimeEx(netUpMaxData, valid, sysmon.AggMax, chartWidth, "KB/s", labels, cfg.NetworkUpThreshold)

	if firstIdx >= 0 {
		snapshot.StartTime = metrics[firstIdx].Timestamp.Format("2006-01-02 15:04")
		snapshot.EndTime = metrics[lastIdx].Timestamp.Format("2006-01-02 15:04")
	} else {
		snapshot.StartTime = startDate.Format("2006-01-02 15:04")
		snapshot.EndTime = endDate.Format("2006-01-02 15:04")
	}

	return snapshot
}

// metricSeries 报告趋势图使用的系统指标序列。
//
// metrics 按报告周期的时间槽位对齐（日报 24 小时、周报 7 天、月报当月天数、年报 12 个月），
// valid 标记对应槽位是否真实存在采集数据。缺失的槽位保留零值并将 valid 置为 false，
// 图表生成时会跳过这些点，绝不使用其它时间点的数据复制填充 —— 否则会画出不存在的"平台趋势"。
type metricSeries struct {
	metrics []sysmon.SystemMetric // 按时间槽位对齐后的指标序列
	valid   []bool                // 与 metrics 等长的有效性标记
	format  string                // 时间标签格式
}

// hasData 返回序列中是否至少存在一个有效数据点。
func (s metricSeries) hasData() bool {
	return s.validCount() > 0
}

// validCount 返回有效数据点的数量。
func (s metricSeries) validCount() int {
	n := 0
	for _, ok := range s.valid {
		if ok {
			n++
		}
	}
	return n
}

// emptyMetricSeries 构造一个与报告周期时间槽位数量一致、但没有任何有效数据点的指标序列。
//
// 用于周期内完全没有采集数据的场景（如年报统计的上一年度尚未开始采集）：
// 图表函数会基于这些全无效的槽位输出"无有效数据"占位，使图表区块照常绘制，
// 而不是因为无数据就整块跳过，导致报告结构缺失。
func emptyMetricSeries(period ReportPeriod, startDate time.Time) metricSeries {
	switch period {
	case PeriodYearly:
		return emptyAlignedSeries(12, startDate, monthAt, "2006-01")
	case PeriodWeekly:
		return emptyAlignedSeries(7, startDate, dayAt, "01-02")
	case PeriodMonthly:
		return emptyAlignedSeries(daysInMonthOf(startDate), startDate, dayAt, "01-02")
	default:
		dayStart := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
		return emptyAlignedSeries(24, dayStart, hourAt, "01-02 15:04")
	}
}

// emptyAlignedSeries 生成 count 个时间槽位全部无效的空指标序列，槽位时间戳按 dateAt 计算。
func emptyAlignedSeries(count int, startDate time.Time,
	dateAt func(time.Time, int) time.Time, format string) metricSeries {

	series := metricSeries{
		metrics: make([]sysmon.SystemMetric, count),
		valid:   make([]bool, count),
		format:  format,
	}
	for i := 0; i < count; i++ {
		series.metrics[i].Timestamp = dateAt(startDate, i)
	}
	return series
}

// hourAt 返回 startDate 起第 i 个小时的时间。
func hourAt(start time.Time, i int) time.Time {
	return start.Add(time.Duration(i) * time.Hour)
}

// alignMetrics 将指标按日期归位到固定长度的时间槽位序列中。
//
// count 为目标槽位数量，dateAt(start, i) 返回第 i 个槽位对应的日期。
// 落在序列范围之外的记录会被忽略；无数据的槽位保持零值且 valid 为 false。
func alignMetrics(metrics []sysmon.SystemMetric, count int, startDate time.Time,
	dateAt func(time.Time, int) time.Time, format string) metricSeries {

	series := metricSeries{
		metrics: make([]sysmon.SystemMetric, count),
		valid:   make([]bool, count),
		format:  format,
	}

	index := make(map[time.Time]int, count)
	for i := 0; i < count; i++ {
		day := dateAt(startDate, i)
		series.metrics[i].Timestamp = day
		index[day] = i
	}

	for _, m := range metrics {
		y, mo, d := m.Timestamp.Date()
		day := time.Date(y, mo, d, 0, 0, 0, 0, startDate.Location())
		if i, ok := index[day]; ok {
			series.metrics[i] = m
			series.valid[i] = true
		}
	}
	return series
}

// alignHourlyMetrics 将指标按小时归位到 24 个小时槽位，缺失小时保持零值并标记为无效。
func alignHourlyMetrics(metrics []sysmon.SystemMetric, startDate time.Time, format string) metricSeries {
	const count = 24

	dayStart := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())

	series := metricSeries{
		metrics: make([]sysmon.SystemMetric, count),
		valid:   make([]bool, count),
		format:  format,
	}
	for i := 0; i < count; i++ {
		series.metrics[i].Timestamp = dayStart.Add(time.Duration(i) * time.Hour)
	}
	for _, m := range metrics {
		h := m.Timestamp.Hour()
		if h >= 0 && h < count {
			series.metrics[h] = m
			series.valid[h] = true
		}
	}
	return series
}

// dayAt 返回 startDate 所在日起第 i 天的零点时间。
func dayAt(start time.Time, i int) time.Time {
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location()).AddDate(0, 0, i)
}

// monthAt 返回 startDate 所在月起第 i 个月的 1 号零点时间。
func monthAt(start time.Time, i int) time.Time {
	return time.Date(start.Year(), start.Month()+time.Month(i), 1, 0, 0, 0, 0, start.Location())
}

// daysInMonthOf 返回 startDate 所在月的天数。
func daysInMonthOf(start time.Time) int {
	year, month, _ := start.Date()
	return time.Date(year, month+1, 0, 0, 0, 0, 0, start.Location()).Day()
}

// mergeMetricsByDay 合并两组按天聚合的指标，同一天的数据以 preferred 为准。
// 用于日级 CSV 不完整时，用小时级聚合结果补齐缺失的日期。
func mergeMetricsByDay(preferred, fallback []sysmon.SystemMetric) []sysmon.SystemMetric {
	if len(preferred) == 0 {
		return fallback
	}
	if len(fallback) == 0 {
		return preferred
	}

	byDay := make(map[time.Time]sysmon.SystemMetric, len(preferred)+len(fallback))
	loc := preferred[0].Timestamp.Location()
	dayOf := func(m sysmon.SystemMetric) time.Time {
		y, mo, d := m.Timestamp.Date()
		return time.Date(y, mo, d, 0, 0, 0, 0, loc)
	}
	for _, m := range fallback {
		byDay[dayOf(m)] = m
	}
	for _, m := range preferred {
		byDay[dayOf(m)] = m
	}

	merged := make([]sysmon.SystemMetric, 0, len(byDay))
	for _, m := range byDay {
		merged = append(merged, m)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Timestamp.Before(merged[j].Timestamp)
	})
	return merged
}

// mergeMetricsByMonth 合并两组按月聚合的指标，同一月份的数据以 preferred 为准。
// 用于月级 CSV 不完整时，用日级/小时级聚合结果补齐缺失的月份。
func mergeMetricsByMonth(preferred, fallback []sysmon.SystemMetric) []sysmon.SystemMetric {
	if len(preferred) == 0 {
		return fallback
	}
	if len(fallback) == 0 {
		return preferred
	}

	monthOf := func(m sysmon.SystemMetric) time.Time {
		return time.Date(m.Timestamp.Year(), m.Timestamp.Month(), 1, 0, 0, 0, 0, m.Timestamp.Location())
	}

	byMonth := make(map[time.Time]sysmon.SystemMetric, len(preferred)+len(fallback))
	for _, m := range fallback {
		byMonth[monthOf(m)] = m
	}
	for _, m := range preferred {
		byMonth[monthOf(m)] = m
	}

	merged := make([]sysmon.SystemMetric, 0, len(byMonth))
	for _, m := range byMonth {
		merged = append(merged, m)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Timestamp.Before(merged[j].Timestamp)
	})
	return merged
}

// countDistinctMonths 统计指标序列覆盖的不同月份数量，用于判断月级数据是否完整。
func countDistinctMonths(metrics []sysmon.SystemMetric) int {
	seen := make(map[int]struct{}, len(metrics))
	for _, m := range metrics {
		seen[m.Timestamp.Year()*12+int(m.Timestamp.Month())] = struct{}{}
	}
	return len(seen)
}

// monthsBetween 返回 [startDate, endDate) 区间覆盖的月份数量，至少为 1。
func monthsBetween(startDate, endDate time.Time) int {
	n := (endDate.Year()-startDate.Year())*12 + int(endDate.Month()) - int(startDate.Month())
	if n < 1 {
		return 1
	}
	return n
}

// loadMetricsWithFallback 根据报告周期加载系统指标序列。
//
// 各周期使用的图表粒度：日报 24 小时、周报 7 天、月报 当月天数、年报 12 个月。
// 数据不足时会回退到更细粒度的数据源重新聚合，但绝不用已有数据复制填充缺失的时间槽位。
func loadMetricsWithFallback(period ReportPeriod, startDate, endDate time.Time) metricSeries {
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
			return metricSeries{format: "01-02 15:04"}
		}
		return alignHourlyMetrics(metrics, startDate, "01-02 15:04")
	}
}

// loadYearlyMetricsWithAggregation 加载年度报告的系统指标数据。
//
// 年报「不完整」（年份还没走完）也要照常绘制图表：只要有任意月份存在数据就绘制，
// 不会因为缺少后续月份而放弃整张图。
//
// 数据源优先级：月级 CSV → 日级 CSV 按月聚合 → 小时级 CSV 按月聚合。
// 月级 CSV 覆盖的月份不完整时，会用细粒度数据聚合出的月份补齐（同月以月级为准），
// 避免只有一两个月数据就把整年图表画成单点。
// 结果对齐到 12 个月槽位，缺失月份标记为无效。
func loadYearlyMetricsWithAggregation(startDate, endDate time.Time) metricSeries {
	const format = "2006-01"

	monthCount := monthsBetween(startDate, endDate)

	monthly, err := sysmon.LoadMonthlyMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载月级指标失败", zap.Error(err))
	}
	monthlyMonths := countDistinctMonths(monthly)
	if monthlyMonths >= monthCount {
		logger.Info(fmt.Sprintf("使用月级数据源生成年报图表 (月份: %d/%d)", monthlyMonths, monthCount))
		return alignMetrics(monthly, 12, startDate, monthAt, format)
	}

	logger.Info("月级数据不完整，回退到日级/小时级数据按月聚合补齐",
		zap.Int("月级月份数", monthlyMonths),
		zap.Int("应有月份数", monthCount),
	)

	daily, err := sysmon.LoadDailyMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载日级指标失败", zap.Error(err))
	}
	hourly, err := sysmon.LoadMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载小时级指标失败", zap.Error(err))
	}

	aggregated := aggregateMetricsByMonth(mergeMetricsByDay(daily, sysmon.AggregateMetricsByDay(hourly)))
	merged := mergeMetricsByMonth(monthly, aggregated)
	if len(merged) == 0 {
		logger.Info("所有数据源均无数据，无法生成年报图表")
		return metricSeries{format: format}
	}

	logger.Info(fmt.Sprintf("使用合并数据源生成年报图表 (月份: %d/%d)", countDistinctMonths(merged), monthCount))
	return alignMetrics(merged, 12, startDate, monthAt, format)
}

// loadWeeklyMetricsWithFallback 加载周报的系统指标数据。
// 以「天」为图表粒度：优先使用日级数据，日级不完整时用小时级数据按天聚合补齐。
// 结果对齐到 7 天槽位，缺失日期标记为无效。
func loadWeeklyMetricsWithFallback(startDate, endDate time.Time) metricSeries {
	const format = "01-02"

	daily, err := sysmon.LoadDailyMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载日级指标失败", zap.Error(err))
	}
	hourly, err := sysmon.LoadMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载小时级指标失败", zap.Error(err))
	}

	merged := mergeMetricsByDay(daily, sysmon.AggregateMetricsByDay(hourly))
	if len(merged) == 0 {
		logger.Info("所有数据源均无数据，无法生成周报图表")
		return metricSeries{format: format}
	}

	logger.Info(fmt.Sprintf("使用日级数据源生成周报图表 (有效天数: %d/7)", len(merged)))
	return alignMetrics(merged, 7, startDate, dayAt, format)
}

// loadMonthlyMetricsWithFallback 加载月报的系统指标数据。
//
// 月报必须以「天」为图表粒度，因此不再使用周级数据源（一个月只有 4~6 个周，
// 无法表达每日趋势，还会导致缺失日期被复制填充成平台状折线）。
// 优先使用日级 CSV，日级不完整时用小时级数据按自然日聚合补齐；
// 两者都缺失时才回退到周级数据，此时图表仅显示有数据的周。
// 结果对齐到当月天数个槽位，缺失日期标记为无效。
func loadMonthlyMetricsWithFallback(startDate, endDate time.Time) metricSeries {
	const format = "01-02"
	days := daysInMonthOf(startDate)

	daily, err := sysmon.LoadDailyMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载日级指标失败", zap.Error(err))
	}
	hourly, err := sysmon.LoadMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载小时级指标失败", zap.Error(err))
	}

	if merged := mergeMetricsByDay(daily, sysmon.AggregateMetricsByDay(hourly)); len(merged) > 0 {
		logger.Info(fmt.Sprintf("使用日级数据源生成月报图表 (有效天数: %d/%d)", len(merged), days))
		return alignMetrics(merged, days, startDate, dayAt, format)
	}

	weekly, err := sysmon.LoadWeeklyMetricsByRange(startDate, endDate)
	if err != nil {
		logger.Warn("加载周级指标失败", zap.Error(err))
	}
	if len(weekly) > 0 {
		logger.Warn("日级与小时级数据均缺失，回退到周级数据源生成月报图表", zap.Int("数据点", len(weekly)))
		return alignMetrics(weekly, days, startDate, dayAt, format)
	}

	logger.Info("所有数据源均无数据，无法生成月报图表")
	return metricSeries{format: format}
}

// partitionAggregate 磁盘分区的聚合中间数据结构。
// 用于在按月聚合过程中累计同一分区的统计值，最终计算平均值。
type partitionAggregate struct {
	Name       string  // 分区名称
	MountPoint string  // 挂载点
	Fstype     string  // 文件系统类型
	percentSum float64 // 使用率累计和，用于计算平均值
	usedSum    uint64  // 已用空间累计和（字节），用于计算平均值
	totalSum   uint64  // 总空间累计和（字节），用于计算平均值
	count      int     // 采样次数
}

// aggregateMetricsByMonth 将系统指标按月聚合，返回每月一条记录。
// 用于年度报告在月级 CSV 数据不足时，从日级或小时级数据动态聚合出月度数据。
// 聚合规则：
//   - CPU/内存/磁盘/网络速率等取算术平均值
//   - CPU 最大值/内存最大值/网络峰值取该月内的最大值
//   - 磁盘分区信息按同名分区聚合后取平均值
//
// 结果按时间戳升序排列，缺失月份不会填充（由上层 padYearlyMetrics 补齐）。
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