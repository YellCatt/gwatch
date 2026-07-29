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
	"gwatch/internal/timeutil"
)

// ReportPeriod 报告周期类型
type ReportPeriod string

const (
	PeriodDaily   ReportPeriod = "daily"
	PeriodWeekly  ReportPeriod = "weekly"
	PeriodMonthly ReportPeriod = "monthly"
	PeriodYearly  ReportPeriod = "yearly"
)

// Report 运维报告（支持多种周期，数据全部来源于 CSV 持久化存储）
type Report struct {
	Period             ReportPeriod
	StartDate          string
	EndDate            string
	TotalTasks         int
	FailedTasks        int
	SuccessTasks       int
	InterfaceStats     []InterfaceStat
	AggregatedErrors   []AggregatedError
	HourlyMetrics      []HourlyResourceMetric
	DailyMetrics       []DailyResourceMetric
	MonthlyMetrics     []MonthlyResourceMetric
	GeneratedAt        time.Time
}

// HourlyResourceMetric 表示按小时聚合的系统资源指标
type HourlyResourceMetric struct {
	TargetName  string
	MetricName  string
	MetricAlias string
	Unit        string
	HourlyData  []HourlyData
}

// DailyResourceMetric 表示按天聚合的系统资源指标
type DailyResourceMetric struct {
	TargetName string
	MetricName string
	MetricAlias string
	Unit        string
	DailyData  []DailyData
}

// MonthlyResourceMetric 表示按月聚合的系统资源指标
type MonthlyResourceMetric struct {
	TargetName string
	MetricName string
	MetricAlias string
	Unit        string
	MonthlyData []MonthlyData
}

// HourlyData 表示小时级别的数据
type HourlyData struct {
	Hour     int
	AvgValue float64
}

// DailyData 表示天级别的数据
type DailyData struct {
	Day      int
	DayLabel string
	AvgValue float64
}

// MonthlyData 表示月级别的数据
type MonthlyData struct {
	Month     int
	MonthLabel string
	AvgValue  float64
}

// InterfaceStat 按接口聚合的执行统计（来源于 monitor_summary.csv）
type InterfaceStat struct {
	TaskID          string
	TaskDesc        string
	URL             string
	Method          string
	TotalCount      int
	SuccessCount    int
	FailedCount     int
	AvgDurationMS   int64
	MaxDurationMS   int64
	LastFailureTime string
}

// AggregatedError 聚合后的告警信息（按接口分组，来源于 alert_summary.csv）
type AggregatedError struct {
	TaskID          string
	TaskDesc        string
	URL             string
	Method          string
	ExpectedStatus  int
	AlertLevel      string
	AlertCount      int
	FirstOccurrence time.Time
	LastOccurrence  time.Time
	ErrorMsg        string
}

// GenerateReportFromStorage 从 CSV 存储中读取聚合数据生成运维报告
func GenerateReportFromStorage(period ReportPeriod, startDate, endDate time.Time) *Report {
	report := &Report{
		Period:      period,
		StartDate:   startDate.Format("2006-01-02"),
		EndDate:     endDate.Format("2006-01-02"),
		GeneratedAt: timeutil.Now(),
	}

	// 告警汇总：来源于 alert_summary.csv，按接口聚合
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

	// 执行统计：来源于 monitor_summary.csv，按接口聚合
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

	return report
}

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

// sortAggregatedErrors 按告警级别（严重优先）和告警次数降序排序
func sortAggregatedErrors(errs []AggregatedError) {
	sort.SliceStable(errs, func(i, j int) bool {
		ri, rj := alertLevelRank(errs[i].AlertLevel), alertLevelRank(errs[j].AlertLevel)
		if ri != rj {
			return ri > rj
		}
		return errs[i].AlertCount > errs[j].AlertCount
	})
}

// sortInterfaceStats 按失败次数降序、执行总次数降序排序
func sortInterfaceStats(stats []InterfaceStat) {
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].FailedCount != stats[j].FailedCount {
			return stats[i].FailedCount > stats[j].FailedCount
		}
		return stats[i].TotalCount > stats[j].TotalCount
	})
}

// alertLevelRank 返回告警级别权重（数值越大越严重）
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

// alertLevelDisplay 返回告警级别对应的图标和展示文本
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

// GenerateDailyReportFromStorage 从存储生成每日运维报告
func GenerateDailyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endDate := startDate.Add(24 * time.Hour)
	return GenerateReportFromStorage(PeriodDaily, startDate, endDate)
}

// GenerateWeeklyReportFromStorage 从存储生成每周运维报告
func GenerateWeeklyReportFromStorage(date time.Time) *Report {
	weekday := date.Weekday()
	daysToMonday := int(weekday - time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}
	startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -daysToMonday)
	endDate := startDate.AddDate(0, 0, 7)
	return GenerateReportFromStorage(PeriodWeekly, startDate, endDate)
}

// GenerateMonthlyReportFromStorage 从存储生成每月运维报告
func GenerateMonthlyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(0, 1, 0)
	return GenerateReportFromStorage(PeriodMonthly, startDate, endDate)
}

// GenerateYearlyReportFromStorage 从存储生成年度运维报告
func GenerateYearlyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), 1, 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(1, 0, 0)
	return GenerateReportFromStorage(PeriodYearly, startDate, endDate)
}

// GenerateReportContent 生成报告内容（文本格式，数据全部来源于 CSV 聚合统计）
func (r *Report) GenerateReportContent() string {
	var content strings.Builder

	periodNames := map[ReportPeriod]string{
		PeriodDaily:   "每日",
		PeriodWeekly:  "每周",
		PeriodMonthly: "每月",
		PeriodYearly:  "年度",
	}

	// 汇总告警数据
	totalAlerts := 0
	criticalAlerts := 0
	warningAlerts := 0
	for _, e := range r.AggregatedErrors {
		totalAlerts += e.AlertCount
		if alertLevelRank(e.AlertLevel) >= 2 {
			criticalAlerts += e.AlertCount
		} else {
			warningAlerts += e.AlertCount
		}
	}

	successRate := 0.0
	if r.TotalTasks > 0 {
		successRate = float64(r.SuccessTasks) / float64(r.TotalTasks) * 100
	}

	content.WriteString("══════════════════════════════════════════════════════════════╗\n")
	content.WriteString(fmt.Sprintf("║              gwatch %s运维报告                              ║\n", periodNames[r.Period]))
	content.WriteString("╚══════════════════════════════════════════════════════════════╝\n")
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("【报告周期】%s ~ %s\n", r.StartDate, r.EndDate))
	content.WriteString(fmt.Sprintf("【生成时间】%s\n", timeutil.FormatDateTime(r.GeneratedAt)))
	content.WriteString(fmt.Sprintf("【设备名称】%s\n", getDeviceName()))
	content.WriteString("【数据来源】CSV 持久化存储（按接口聚合统计）\n")
	content.WriteString("\n")

	// 执行总览
	content.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	content.WriteString("【执行总览】\n")
	content.WriteString(fmt.Sprintf("  监控接口数: %d\n", len(r.InterfaceStats)))
	content.WriteString(fmt.Sprintf("  执行总次数: %d\n", r.TotalTasks))
	content.WriteString(fmt.Sprintf("  ✅ 成功: %d\n", r.SuccessTasks))
	content.WriteString(fmt.Sprintf("  ❌ 失败: %d\n", r.FailedTasks))
	content.WriteString(fmt.Sprintf("  成功率: %.2f%%\n", successRate))
	content.WriteString(fmt.Sprintf("  告警接口数: %d\n", len(r.AggregatedErrors)))
	content.WriteString(fmt.Sprintf("  告警总次数: %d（🚨 严重 %d / ⚠️ 警告 %d）\n", totalAlerts, criticalAlerts, warningAlerts))

	// 告警汇总（核心内容：哪个接口、什么级别、告警了多少次）
	if len(r.AggregatedErrors) > 0 {
		content.WriteString("\n")
		content.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		content.WriteString("【告警汇总】（按接口聚合，按告警级别排序）\n")
		content.WriteString("\n")

		for i, aggErr := range r.AggregatedErrors {
			levelIcon, levelText := alertLevelDisplay(aggErr.AlertLevel)
			content.WriteString("┌──────────────────────────────────────────────────────────────┐\n")
			content.WriteString(fmt.Sprintf("│ 告警 #%d  %s %s\n", i+1, levelIcon, levelText))
			content.WriteString("├──────────────────────────────────────────────────────────────┤\n")
			content.WriteString(fmt.Sprintf("│ 接口:     [%s] %s\n", aggErr.Method, aggErr.URL))
			content.WriteString(fmt.Sprintf("│ 任务:     [%s] %s\n", aggErr.TaskID, aggErr.TaskDesc))
			content.WriteString(fmt.Sprintf("│ 期望状态: %d\n", aggErr.ExpectedStatus))
			content.WriteString(fmt.Sprintf("│ 告警次数: %d 次\n", aggErr.AlertCount))
			content.WriteString(fmt.Sprintf("│ 首次告警: %s\n", timeutil.FormatDateTime(aggErr.FirstOccurrence)))
			content.WriteString(fmt.Sprintf("│ 最后告警: %s\n", timeutil.FormatDateTime(aggErr.LastOccurrence)))
			if aggErr.ErrorMsg != "" {
				content.WriteString(fmt.Sprintf("│ 错误信息: %s\n", aggErr.ErrorMsg))
			}
			content.WriteString("└──────────────────────────────────────────────────────────────┘\n")
			content.WriteString("\n")
		}
	} else {
		content.WriteString("\n")
		content.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		content.WriteString("【告警汇总】本周期内无告警，所有接口运行正常 ✅\n")
	}

	// 接口执行明细（按接口聚合）
	if len(r.InterfaceStats) > 0 {
		content.WriteString("\n")
		content.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		content.WriteString("【接口执行明细】（按接口聚合，按失败次数排序）\n")
		content.WriteString("\n")

		for i, stat := range r.InterfaceStats {
			rate := 0.0
			if stat.TotalCount > 0 {
				rate = float64(stat.SuccessCount) / float64(stat.TotalCount) * 100
			}
			statusIcon := "✅"
			if stat.FailedCount > 0 {
				statusIcon = "❌"
			}
			content.WriteString("┌──────────────────────────────────────────────────────────────┐\n")
			content.WriteString(fmt.Sprintf("│ #%d %s [%s] %s\n", i+1, statusIcon, stat.Method, stat.URL))
			content.WriteString("├──────────────────────────────────────────────────────────────┤\n")
			content.WriteString(fmt.Sprintf("│ 任务:     [%s] %s\n", stat.TaskID, stat.TaskDesc))
			content.WriteString(fmt.Sprintf("│ 执行:     %d 次（成功 %d / 失败 %d，成功率 %.2f%%）\n", stat.TotalCount, stat.SuccessCount, stat.FailedCount, rate))
			content.WriteString(fmt.Sprintf("│ 耗时:     平均 %dms / 最大 %dms\n", stat.AvgDurationMS, stat.MaxDurationMS))
			if stat.LastFailureTime != "" {
				content.WriteString(fmt.Sprintf("│ 最后失败: %s\n", stat.LastFailureTime))
			}
			content.WriteString("└──────────────────────────────────────────────────────────────┘\n")
			content.WriteString("\n")
		}
	}

	// 系统资源监控
	r.generateResourceMetricsContent(&content)

	content.WriteString("\n")
	content.WriteString("══════════════════════════════════════════════════════════════\n")
	content.WriteString("来自 gwatch 接口监控系统\n")

	return content.String()
}

func (r *Report) generateResourceMetricsContent(content *strings.Builder) {
	switch r.Period {
	case PeriodDaily:
		r.generateHourlyMetricsContent(content)
	case PeriodWeekly:
		r.generateDailyMetricsContent(content, "周")
	case PeriodMonthly:
		r.generateDailyMetricsContent(content, "月")
	case PeriodYearly:
		r.generateMonthlyMetricsContent(content)
	}
}

func (r *Report) generateHourlyMetricsContent(content *strings.Builder) {
	if len(r.HourlyMetrics) == 0 {
		return
	}

	targetMap := make(map[string][]*HourlyResourceMetric)
	for i := range r.HourlyMetrics {
		targetMap[r.HourlyMetrics[i].TargetName] = append(targetMap[r.HourlyMetrics[i].TargetName], &r.HourlyMetrics[i])
	}

	for targetName, metrics := range targetMap {
		content.WriteString(fmt.Sprintf("\n"))
		content.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"))
		content.WriteString(fmt.Sprintf("【系统资源监控 - %s】\n", targetName))

		for _, metric := range metrics {
			name := metric.MetricName
			if metric.MetricAlias != "" {
				name = metric.MetricAlias
			}

			content.WriteString(fmt.Sprintf("\n"))
			content.WriteString(fmt.Sprintf("============ %s 24小时平均负载 ============\n", name))

			for _, hourly := range metric.HourlyData {
				if hourly.AvgValue < 0 {
					content.WriteString(fmt.Sprintf("%02d│\n", hourly.Hour))
				} else {
					bar := generateBar(hourly.AvgValue)
					content.WriteString(fmt.Sprintf("%02d│%s %d%%\n", hourly.Hour, bar, int(hourly.AvgValue)))
				}
			}

			content.WriteString(fmt.Sprintf("============================================\n"))
		}
	}
}

func (r *Report) generateDailyMetricsContent(content *strings.Builder, periodLabel string) {
	if len(r.DailyMetrics) == 0 {
		return
	}

	targetMap := make(map[string][]*DailyResourceMetric)
	for i := range r.DailyMetrics {
		targetMap[r.DailyMetrics[i].TargetName] = append(targetMap[r.DailyMetrics[i].TargetName], &r.DailyMetrics[i])
	}

	for targetName, metrics := range targetMap {
		content.WriteString(fmt.Sprintf("\n"))
		content.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"))
		content.WriteString(fmt.Sprintf("【系统资源监控 - %s】\n", targetName))

		for _, metric := range metrics {
			name := metric.MetricName
			if metric.MetricAlias != "" {
				name = metric.MetricAlias
			}

			content.WriteString(fmt.Sprintf("\n"))
			content.WriteString(fmt.Sprintf("============ %s %s平均负载 ============\n", name, periodLabel))

			for _, daily := range metric.DailyData {
				if daily.AvgValue < 0 {
					content.WriteString(fmt.Sprintf("%s│\n", daily.DayLabel))
				} else {
					bar := generateBar(daily.AvgValue)
					content.WriteString(fmt.Sprintf("%s│%s %d%%\n", daily.DayLabel, bar, int(daily.AvgValue)))
				}
			}

			content.WriteString(fmt.Sprintf("============================================\n"))
		}
	}
}

func (r *Report) generateMonthlyMetricsContent(content *strings.Builder) {
	if len(r.MonthlyMetrics) == 0 {
		return
	}

	targetMap := make(map[string][]*MonthlyResourceMetric)
	for i := range r.MonthlyMetrics {
		targetMap[r.MonthlyMetrics[i].TargetName] = append(targetMap[r.MonthlyMetrics[i].TargetName], &r.MonthlyMetrics[i])
	}

	for targetName, metrics := range targetMap {
		content.WriteString(fmt.Sprintf("\n"))
		content.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"))
		content.WriteString(fmt.Sprintf("【系统资源监控 - %s】\n", targetName))

		for _, metric := range metrics {
			name := metric.MetricName
			if metric.MetricAlias != "" {
				name = metric.MetricAlias
			}

			content.WriteString(fmt.Sprintf("\n"))
			content.WriteString(fmt.Sprintf("============ %s 年度平均负载 ============\n", name))

			for _, monthly := range metric.MonthlyData {
				if monthly.AvgValue < 0 {
					content.WriteString(fmt.Sprintf("%s│\n", monthly.MonthLabel))
				} else {
					bar := generateBar(monthly.AvgValue)
					content.WriteString(fmt.Sprintf("%s│%s %d%%\n", monthly.MonthLabel, bar, int(monthly.AvgValue)))
				}
			}

			content.WriteString(fmt.Sprintf("============================================\n"))
		}
	}
}

func generateBar(value float64) string {
	maxLen := 10
	ratio := value / 100
	barLen := int(ratio * float64(maxLen))
	if barLen < 0 {
		barLen = 0
	}
	if barLen > maxLen {
		barLen = maxLen
	}
	return strings.Repeat("█", barLen)
}

// SaveReport 保存报告到文件
func (r *Report) SaveReport() (string, error) {
	reportDir := config.GlobalConfig.App.ReportDir
	if reportDir == "" {
		reportDir = "./reports"
	}

	if err := os.MkdirAll(reportDir, 0755); err != nil {
		logger.Error("Failed to create report directory", zap.Error(err))
		return "", err
	}

	periodNames := map[ReportPeriod]string{
		PeriodDaily:   "daily",
		PeriodWeekly:  "weekly",
		PeriodMonthly: "monthly",
		PeriodYearly:  "yearly",
	}

	filename := fmt.Sprintf("%s_report_%s_%s.txt", periodNames[r.Period], r.StartDate, r.EndDate)
	filePath := filepath.Join(reportDir, filename)

	content := r.GenerateReportContent()
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		logger.Error("Failed to save report", zap.String("file", filePath), zap.Error(err))
		return "", err
	}

	logger.Info("Report saved", zap.String("file", filePath))
	return filePath, nil
}

// SendReportEmail 发送报告邮件
func (r *Report) SendReportEmail() error {
	if !email.Config.Enabled {
		logger.Info("Email is disabled, skipping report email")
		return nil
	}

	periodNames := map[ReportPeriod]string{
		PeriodDaily:   "每日",
		PeriodWeekly:  "每周",
		PeriodMonthly: "每月",
		PeriodYearly:  "年度",
	}

	totalAlerts := 0
	for _, e := range r.AggregatedErrors {
		totalAlerts += e.AlertCount
	}

	subject := fmt.Sprintf("[gwatch] %s运维报告 - %s ~ %s", periodNames[r.Period], r.StartDate, r.EndDate)
	if totalAlerts > 0 {
		subject = fmt.Sprintf("[gwatch] %s运维报告 - %s ~ %s（告警 %d 次）", periodNames[r.Period], r.StartDate, r.EndDate, totalAlerts)
	}
	body := r.GenerateReportContent()

	logger.Info("Sending report email", zap.String("period", string(r.Period)))
	return email.SendCustomEmail(subject, body)
}

// getDeviceName 获取设备名称
func getDeviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return "Unknown"
	}
	return name
}