package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/psv"
	"gwatch/internal/storage"
	"gwatch/internal/testcase"
	"gwatch/internal/timeutil"
)

// MonitorResult 表示监控结果（与 monitor 包中的结构相同，避免导入）
type MonitorResult struct {
	TestCase  psv.TestCase
	Result    testcase.TestResult
	Timestamp time.Time
}

// ReportPeriod 报告周期类型
type ReportPeriod string

const (
	PeriodDaily   ReportPeriod = "daily"
	PeriodWeekly  ReportPeriod = "weekly"
	PeriodMonthly ReportPeriod = "monthly"
	PeriodYearly  ReportPeriod = "yearly"
)

// Report 运维报告（支持多种周期）
type Report struct {
	Period             ReportPeriod
	StartDate          string
	EndDate            string
	TotalTasks         int
	FailedTasks        int
	SuccessTasks       int
	ErrorDetails       []ErrorDetail
	AggregatedErrors   []AggregatedError
	ResourceMetrics    []ResourceMetric
	GeneratedAt        time.Time
}

// ResourceMetric 表示系统资源指标（按小时聚合）
type ResourceMetric struct {
	TargetName  string
	MetricName  string
	MetricAlias string
	Unit        string
	HourlyData  []HourlyData
}

// HourlyData 表示小时级别的数据
type HourlyData struct {
	Hour     int
	AvgValue float64
}

// ErrorDetail 错误详情
type ErrorDetail struct {
	TaskID         string
	TaskDesc       string
	URL            string
	Method         string
	ExpectedStatus int
	ActualStatus   int
	ExpectedBody   string
	ActualBody     string
	ErrorMsg       string
	Timestamp      time.Time
	Duration       time.Duration
}

// AggregatedError 聚合后的错误信息（按TaskID分组）
type AggregatedError struct {
	TaskID         string
	TaskDesc       string
	URL            string
	Method         string
	ExpectedStatus int
	AlertCount     int
	FirstOccurrence time.Time
	LastOccurrence  time.Time
	ErrorMsg       string
}

// GenerateReport 生成运维报告
// results: 监控结果列表
// period: 报告周期
// startDate: 报告开始日期
// endDate: 报告结束日期（不含）
func GenerateReport(results []MonitorResult, period ReportPeriod, startDate, endDate time.Time) *Report {
	report := &Report{
		Period:      period,
		StartDate:   startDate.Format("2006-01-02"),
		EndDate:     endDate.Format("2006-01-02"),
		GeneratedAt: timeutil.Now(),
	}

	for _, result := range results {
		if result.Timestamp.After(startDate) && result.Timestamp.Before(endDate) {
			if !result.Result.Passed {
				report.FailedTasks++
				report.ErrorDetails = append(report.ErrorDetails, ErrorDetail{
					TaskID:         result.TestCase.ID,
					TaskDesc:       result.TestCase.Desc,
					URL:            result.TestCase.URL,
					Method:         result.TestCase.Method,
					ExpectedStatus: result.TestCase.ExpectedStatus,
					ActualStatus:   result.Result.ActualStatus,
					ExpectedBody:   result.TestCase.ExpectedBody,
					ActualBody:     result.Result.ResponseBody,
					ErrorMsg:       result.Result.Error,
					Timestamp:      result.Timestamp,
					Duration:       result.Result.Duration,
				})
			} else {
				report.SuccessTasks++
			}
			report.TotalTasks++
		}
	}

	return report
}

// GenerateReportFromStorage 从存储中读取告警汇总数据生成运维报告
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
		return report
	}

	for _, summary := range alertSummaries {
		firstOccurrence, _ := time.Parse("2006-01-02 15:04:05", summary.FirstOccurrence)
		lastOccurrence, _ := time.Parse("2006-01-02 15:04:05", summary.LastOccurrence)

		report.AggregatedErrors = append(report.AggregatedErrors, AggregatedError{
			TaskID:         summary.TestCaseID,
			TaskDesc:       summary.TestCaseDesc,
			URL:            summary.URL,
			Method:         summary.Method,
			ExpectedStatus: summary.ExpectedStatus,
			AlertCount:     int(summary.AlertCount),
			FirstOccurrence: firstOccurrence,
			LastOccurrence:  lastOccurrence,
			ErrorMsg:       summary.ErrorMsg,
		})

		report.FailedTasks += int(summary.AlertCount)
	}

	monitorSummaries, err := storage.GetMonitorSummaryByPeriod(startDate, endDate)
	if err != nil {
		logger.Error("Failed to get monitor summary from storage", zap.Error(err))
		return report
	}

	for _, summary := range monitorSummaries {
		report.TotalTasks += int(summary.TotalCount)
		report.SuccessTasks += int(summary.SuccessCount)
	}

	if config.GlobalConfig.Scraper.Enabled && len(config.GlobalConfig.Scraper.Targets) > 0 {
		report.ResourceMetrics = loadResourceMetrics(startDate, endDate)
	}

	return report
}

func loadResourceMetrics(startDate, endDate time.Time) []ResourceMetric {
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

	metricMap := make(map[key]*ResourceMetric)

	for _, avg := range hourlyAvgs {
		k := key{
			targetName: avg.TargetName,
			metricName: avg.MetricName,
		}

		if metricMap[k] == nil {
			metricMap[k] = &ResourceMetric{
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

	var results []ResourceMetric
	for _, m := range metricMap {
		results = append(results, *m)
	}

	return results
}

// GenerateDailyReport 生成每日运维报告
func GenerateDailyReport(results []MonitorResult, date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endDate := startDate.Add(24 * time.Hour)
	return GenerateReport(results, PeriodDaily, startDate, endDate)
}

// GenerateDailyReportFromStorage 从存储生成每日运维报告
func GenerateDailyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endDate := startDate.Add(24 * time.Hour)
	return GenerateReportFromStorage(PeriodDaily, startDate, endDate)
}

// GenerateWeeklyReport 生成每周运维报告（周一到周日）
func GenerateWeeklyReport(results []MonitorResult, date time.Time) *Report {
	weekday := date.Weekday()
	daysToMonday := int(weekday - time.Monday)
	if daysToMonday < 0 {
		daysToMonday += 7
	}
	startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -daysToMonday)
	endDate := startDate.AddDate(0, 0, 7)
	return GenerateReport(results, PeriodWeekly, startDate, endDate)
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

// GenerateMonthlyReport 生成每月运维报告
func GenerateMonthlyReport(results []MonitorResult, date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(0, 1, 0)
	return GenerateReport(results, PeriodMonthly, startDate, endDate)
}

// GenerateMonthlyReportFromStorage 从存储生成每月运维报告
func GenerateMonthlyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(0, 1, 0)
	return GenerateReportFromStorage(PeriodMonthly, startDate, endDate)
}

// GenerateYearlyReport 生成年度运维报告
func GenerateYearlyReport(results []MonitorResult, date time.Time) *Report {
	startDate := time.Date(date.Year(), 1, 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(1, 0, 0)
	return GenerateReport(results, PeriodYearly, startDate, endDate)
}

// GenerateYearlyReportFromStorage 从存储生成年度运维报告
func GenerateYearlyReportFromStorage(date time.Time) *Report {
	startDate := time.Date(date.Year(), 1, 1, 0, 0, 0, 0, date.Location())
	endDate := startDate.AddDate(1, 0, 0)
	return GenerateReportFromStorage(PeriodYearly, startDate, endDate)
}

// GenerateReportContent 生成报告内容（文本格式）
func (r *Report) GenerateReportContent() string {
	var content strings.Builder

	periodNames := map[ReportPeriod]string{
		PeriodDaily:   "每日",
		PeriodWeekly:  "每周",
		PeriodMonthly: "每月",
		PeriodYearly:  "年度",
	}

	content.WriteString(fmt.Sprintf("══════════════════════════════════════════════════════════════╗\n"))
	content.WriteString(fmt.Sprintf("║              gwatch %s运维报告                              ║\n", periodNames[r.Period]))
	content.WriteString(fmt.Sprintf("╚══════════════════════════════════════════════════════════════╝\n"))
	content.WriteString(fmt.Sprintf("\n"))
	content.WriteString(fmt.Sprintf("【报告周期】%s ~ %s\n", r.StartDate, r.EndDate))
	content.WriteString(fmt.Sprintf("【生成时间】%s\n", timeutil.FormatDateTime(r.GeneratedAt)))
	content.WriteString(fmt.Sprintf("【设备名称】%s\n", getDeviceName()))
	content.WriteString(fmt.Sprintf("\n"))
	content.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"))
	content.WriteString(fmt.Sprintf("【执行统计】\n"))
	content.WriteString(fmt.Sprintf("  监控任务总数: %d\n", r.TotalTasks))
	content.WriteString(fmt.Sprintf("  ✅ 成功: %d\n", r.SuccessTasks))
	content.WriteString(fmt.Sprintf("  ❌ 失败: %d\n", r.FailedTasks))

	if len(r.ResourceMetrics) > 0 {
		content.WriteString(r.generateResourceMetricsContent())
	}

	if len(r.AggregatedErrors) > 0 {
		content.WriteString(fmt.Sprintf("\n"))
		content.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"))
		content.WriteString(fmt.Sprintf("【告警汇总】（按任务ID聚合）\n"))
		content.WriteString(fmt.Sprintf("  告警任务数: %d\n", len(r.AggregatedErrors)))
		content.WriteString(fmt.Sprintf("\n"))

		for i, aggErr := range r.AggregatedErrors {
			content.WriteString(fmt.Sprintf("┌──────────────────────────────────────────────────────────────┐\n"))
			content.WriteString(fmt.Sprintf("│ 告警 #%d\n", i+1))
			content.WriteString(fmt.Sprintf("├──────────────────────────────────────────────────────────────┤\n"))
			content.WriteString(fmt.Sprintf("│ 任务ID:         %s\n", aggErr.TaskID))
			content.WriteString(fmt.Sprintf("│ 任务描述:       %s\n", aggErr.TaskDesc))
			content.WriteString(fmt.Sprintf("│ 请求方法:       %s\n", aggErr.Method))
			content.WriteString(fmt.Sprintf("│ 请求URL:        %s\n", aggErr.URL))
			content.WriteString(fmt.Sprintf("│ 告警次数:       %d\n", aggErr.AlertCount))
			content.WriteString(fmt.Sprintf("│ 首次告警:       %s\n", timeutil.FormatDateTime(aggErr.FirstOccurrence)))
			content.WriteString(fmt.Sprintf("│ 最后告警:       %s\n", timeutil.FormatDateTime(aggErr.LastOccurrence)))
			content.WriteString(fmt.Sprintf("├──────────────────────────────────────────────────────────────┤\n"))
			content.WriteString(fmt.Sprintf("│ 错误信息:       %s\n", aggErr.ErrorMsg))
			content.WriteString(fmt.Sprintf("└──────────────────────────────────────────────────────────────┘\n"))
			content.WriteString(fmt.Sprintf("\n"))
		}
	} else {
		content.WriteString(fmt.Sprintf("\n"))
		content.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"))
		content.WriteString(fmt.Sprintf("【状态】今日所有接口监控均正常 ✅\n"))
	}

	content.WriteString(fmt.Sprintf("\n"))
	content.WriteString(fmt.Sprintf("══════════════════════════════════════════════════════════════\n"))
	content.WriteString(fmt.Sprintf("来自 gwatch 接口监控系统\n"))

	return content.String()
}

func (r *Report) generateResourceMetricsContent() string {
	var content strings.Builder

	targetMap := make(map[string][]*ResourceMetric)
	for i := range r.ResourceMetrics {
		targetMap[r.ResourceMetrics[i].TargetName] = append(targetMap[r.ResourceMetrics[i].TargetName], &r.ResourceMetrics[i])
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

	return content.String()
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

// formatBody 格式化响应体显示
func formatBody(body string) string {
	if len(body) > 500 {
		return body[:500] + "..."
	}
	return body
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

	subject := fmt.Sprintf("[gwatch] %s运维报告 - %s ~ %s", periodNames[r.Period], r.StartDate, r.EndDate)
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