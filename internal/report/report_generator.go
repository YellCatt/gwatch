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
	"gwatch/internal/email"
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
		logger.Error("Failed to get alert summary from storage", zap.Error(err))
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

	monitorSummaries, err := storage.GetMonitorSummaryByPeriod(startDate, endDate)
	if err != nil {
		logger.Error("Failed to get monitor summary from storage", zap.Error(err))
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

		report.InterfaceStats = append(report.InterfaceStats, InterfaceStat{
			TaskID:          summary.TestCaseID,
			TaskDesc:        summary.TestCaseDesc,
			URL:             summary.URL,
			Method:          summary.Method,
			TotalCount:      int(summary.TotalCount),
			SuccessCount:    int(summary.SuccessCount),
			FailedCount:     int(summary.FailedCount),
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
		logger.Error("Failed to get system alerts from storage", zap.Error(err))
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
		logger.Error("Failed to get scraper alerts from storage", zap.Error(err))
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

	if period == PeriodDaily && config.GlobalConfig.SystemMon.Enabled {
		report.SystemMetrics = loadSystemMetrics()
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
		logger.Error("Failed to get scraper metrics hourly avg", zap.Error(err))
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
		logger.Error("Failed to get scraper metrics daily avg", zap.Error(err))
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
		logger.Error("Failed to get scraper metrics monthly avg", zap.Error(err))
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
		return "🚨", "CRITICAL（严重）"
	case storage.AlertLevelWarning:
		return "⚠️", "WARNING（警告）"
	default:
		return "🔔", level
	}
}

// getDeviceName 获取当前主机名，用于报告中标识设备。
func getDeviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return "Unknown"
	}
	return name
}

// SaveReport 生成报告内容并保存为文本文件，返回保存路径。
func (r *Report) SaveReport() (string, error) {
	reportDir := config.GlobalConfig.App.ReportDir
	if reportDir == "" {
		reportDir = "./reports"
	}

	if err := os.MkdirAll(reportDir, 0755); err != nil {
		logger.Error("Failed to create report directory", zap.Error(err))
		return "", err
	}

	filename := fmt.Sprintf("%s_report_%s_%s.txt", PeriodNamesEn[r.Period], r.StartDate, r.EndDate)
	filePath := filepath.Join(reportDir, filename)

	content := r.GenerateContent()
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		logger.Error("Failed to save report", zap.String("file", filePath), zap.Error(err))
		return "", err
	}

	logger.Info("Report saved", zap.String("file", filePath))
	return filePath, nil
}

// SendReportEmail 发送报告邮件，邮件主题包含周期和告警次数信息。
func (r *Report) SendReportEmail() error {
	if !email.Config.Enabled {
		logger.Info("Email is disabled, skipping report email")
		return nil
	}

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

	logger.Info("Sending report email", zap.String("period", string(r.Period)))
	return email.SendCustomEmail(subject, body)
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

// loadSystemMetrics 采集当前系统指标，生成 SystemMetricsSnapshot 用于日报中的系统状态展示。
func loadSystemMetrics() *SystemMetricsSnapshot {
	metric, err := sysmon.CollectMetrics()
	if err != nil {
		logger.Warn("Failed to collect system metrics for report", zap.Error(err))
		return nil
	}
	return &SystemMetricsSnapshot{
		CPUPercent:     metric.CPUPercent,
		MemoryPercent:  metric.MemoryPercent,
		DiskPercent:    metric.DiskPercent,
		NetDownKBps:    metric.NetDownKBps,
		NetUpKBps:      metric.NetUpKBps,
		MemUsedBytes:   metric.MemoryUsed,
		MemTotalBytes:  metric.MemoryTotal,
		DiskUsedBytes:  metric.DiskUsed,
		DiskTotalBytes: metric.DiskTotal,
	}
}
