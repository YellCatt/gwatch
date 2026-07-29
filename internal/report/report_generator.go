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

func sortAggregatedErrors(errs []AggregatedError) {
	sort.SliceStable(errs, func(i, j int) bool {
		ri, rj := alertLevelRank(errs[i].AlertLevel), alertLevelRank(errs[j].AlertLevel)
		if ri != rj {
			return ri > rj
		}
		return errs[i].AlertCount > errs[j].AlertCount
	})
}

func sortInterfaceStats(stats []InterfaceStat) {
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].FailedCount != stats[j].FailedCount {
			return stats[i].FailedCount > stats[j].FailedCount
		}
		return stats[i].TotalCount > stats[j].TotalCount
	})
}

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

func GenerateDailyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endDate := startDate.Add(24 * time.Hour)
	return GenerateReportFromStorage(PeriodDaily, startDate, endDate)
}

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

func GenerateMonthlyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(0, 1, 0)
	return GenerateReportFromStorage(PeriodMonthly, startDate, endDate)
}

func GenerateYearlyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), 1, 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(1, 0, 0)
	return GenerateReportFromStorage(PeriodYearly, startDate, endDate)
}

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

func getDeviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return "Unknown"
	}
	return name
}