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
	"gwatch/internal/monitor"
	"gwatch/internal/timeutil"
)

// DailyReport 每日运维报告
type DailyReport struct {
	Date         string
	TotalTasks   int
	FailedTasks  int
	SuccessTasks int
	ErrorDetails []ErrorDetail
	GeneratedAt  time.Time
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

// GenerateDailyReport 生成每日运维报告
func GenerateDailyReport(date time.Time) (*DailyReport, error) {
	report := &DailyReport{
		Date:        date.Format("2006-01-02"),
		GeneratedAt: timeutil.Now(),
	}

	// 获取今天的监控结果
	results := monitor.GetResults()
	todayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	todayEnd := todayStart.Add(24 * time.Hour)

	for _, result := range results {
		// 过滤今天的结果
		if result.Timestamp.After(todayStart) && result.Timestamp.Before(todayEnd) {
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

	return report, nil
}

// GenerateReportContent 生成报告内容（文本格式）
func (r *DailyReport) GenerateReportContent() string {
	var content strings.Builder

	content.WriteString(fmt.Sprintf("══════════════════════════════════════════════════════════════╗\n"))
	content.WriteString(fmt.Sprintf("║              gwatch 每日运维报告                              ║\n"))
	content.WriteString(fmt.Sprintf("╚══════════════════════════════════════════════════════════════╝\n"))
	content.WriteString(fmt.Sprintf("\n"))
	content.WriteString(fmt.Sprintf("【报告日期】%s\n", r.Date))
	content.WriteString(fmt.Sprintf("【生成时间】%s\n", timeutil.FormatDateTime(r.GeneratedAt)))
	content.WriteString(fmt.Sprintf("【设备名称】%s\n", getDeviceName()))
	content.WriteString(fmt.Sprintf("\n"))
	content.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"))
	content.WriteString(fmt.Sprintf("【执行统计】\n"))
	content.WriteString(fmt.Sprintf("  监控任务总数: %d\n", r.TotalTasks))
	content.WriteString(fmt.Sprintf("  ✅ 成功: %d\n", r.SuccessTasks))
	content.WriteString(fmt.Sprintf("  ❌ 失败: %d\n", r.FailedTasks))

	if r.FailedTasks > 0 {
		content.WriteString(fmt.Sprintf("\n"))
		content.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"))
		content.WriteString(fmt.Sprintf("【错误详情】\n"))
		content.WriteString(fmt.Sprintf("\n"))

		for i, detail := range r.ErrorDetails {
			content.WriteString(fmt.Sprintf("┌──────────────────────────────────────────────────────────────┐\n"))
			content.WriteString(fmt.Sprintf("│ 错误 #%d\n", i+1))
			content.WriteString(fmt.Sprintf("├──────────────────────────────────────────────────────────────┤\n"))
			content.WriteString(fmt.Sprintf("│ 任务ID:     %s\n", detail.TaskID))
			content.WriteString(fmt.Sprintf("│ 任务描述:   %s\n", detail.TaskDesc))
			content.WriteString(fmt.Sprintf("│ 请求方法:   %s\n", detail.Method))
			content.WriteString(fmt.Sprintf("│ 请求URL:    %s\n", detail.URL))
			content.WriteString(fmt.Sprintf("│ 执行时间:   %s\n", timeutil.FormatDateTime(detail.Timestamp)))
			content.WriteString(fmt.Sprintf("│ 耗时:       %.3fms\n", detail.Duration.Milliseconds()))
			content.WriteString(fmt.Sprintf("├──────────────────────────────────────────────────────────────┤\n"))
			content.WriteString(fmt.Sprintf("│ 错误信息:   %s\n", detail.ErrorMsg))
			content.WriteString(fmt.Sprintf("├──────────────────────────────────────────────────────────────┤\n"))
			content.WriteString(fmt.Sprintf("│ HTTP状态码断言:\n"))
			content.WriteString(fmt.Sprintf("│   期望: %d\n", detail.ExpectedStatus))
			content.WriteString(fmt.Sprintf("│   实际: %d\n", detail.ActualStatus))

			if detail.ExpectedBody != "" {
				content.WriteString(fmt.Sprintf("├──────────────────────────────────────────────────────────────┤\n"))
				content.WriteString(fmt.Sprintf("│ 响应体断言:\n"))
				content.WriteString(fmt.Sprintf("│   期望:\n"))
				content.WriteString(fmt.Sprintf("│     %s\n", formatBody(detail.ExpectedBody)))
				content.WriteString(fmt.Sprintf("│   实际:\n"))
				content.WriteString(fmt.Sprintf("│     %s\n", formatBody(detail.ActualBody)))
			}
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

// formatBody 格式化响应体显示
func formatBody(body string) string {
	if len(body) > 500 {
		return body[:500] + "..."
	}
	return body
}

// SaveReport 保存报告到文件
func (r *DailyReport) SaveReport() (string, error) {
	reportDir := config.GlobalConfig.App.ReportDir
	if reportDir == "" {
		reportDir = "./reports"
	}

	// 创建报告目录
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		logger.Error("Failed to create report directory", zap.Error(err))
		return "", err
	}

	// 生成文件名
	filename := fmt.Sprintf("daily_report_%s.txt", r.Date)
	filePath := filepath.Join(reportDir, filename)

	// 写入报告内容
	content := r.GenerateReportContent()
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		logger.Error("Failed to save daily report", zap.String("file", filePath), zap.Error(err))
		return "", err
	}

	logger.Info("Daily report saved", zap.String("file", filePath))
	return filePath, nil
}

// SendReportEmail 发送报告邮件
func (r *DailyReport) SendReportEmail() error {
	if !email.Config.Enabled {
		logger.Info("Email is disabled, skipping report email")
		return nil
	}

	subject := fmt.Sprintf("[gwatch] 每日运维报告 - %s", r.Date)
	body := r.GenerateReportContent()

	logger.Info("Sending daily report email")
	return email.SendCustomEmail(subject, body)
}

// GenerateAndSendDailyReport 生成并发送每日报告
func GenerateAndSendDailyReport(date time.Time) error {
	report, err := GenerateDailyReport(date)
	if err != nil {
		logger.Error("Failed to generate daily report", zap.Error(err))
		return err
	}

	// 保存报告文件
	_, err = report.SaveReport()
	if err != nil {
		logger.Error("Failed to save daily report", zap.Error(err))
		return err
	}

	// 发送邮件（如果有错误或配置了始终发送）
	if report.FailedTasks > 0 || config.GlobalConfig.Monitor.AlertOnFailure {
		err = report.SendReportEmail()
		if err != nil {
			logger.Error("Failed to send daily report email", zap.Error(err))
			return err
		}
	}

	return nil
}

// getDeviceName 获取设备名称
func getDeviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return "Unknown"
	}
	return name
}

// ScheduleDailyReport 调度每日报告生成
func ScheduleDailyReport() {
	go func() {
		for {
			now := timeutil.Now()
			// 获取配置的报告时间，默认为00:00
			reportTimeStr := config.GlobalConfig.Monitor.ReportTime
			if reportTimeStr == "" {
				reportTimeStr = "00:00"
			}

			// 解析报告时间
			var reportHour, reportMinute int
			fmt.Sscanf(reportTimeStr, "%d:%d", &reportHour, &reportMinute)

			// 计算今天报告时间
			reportTime := time.Date(now.Year(), now.Month(), now.Day(), reportHour, reportMinute, 0, 0, now.Location())

			// 如果今天的报告时间已经过了，计算明天的报告时间
			if now.After(reportTime) {
				reportTime = reportTime.Add(24 * time.Hour)
			}

			duration := reportTime.Sub(now)

			logger.Info("Scheduling daily report", zap.Time("next_run", reportTime))

			time.Sleep(duration)

			// 生成前一天的报告
			yesterday := timeutil.Now().Add(-24 * time.Hour)
			logger.Info("Generating daily report for", zap.String("date", yesterday.Format("2006-01-02")))
			GenerateAndSendDailyReport(yesterday)
		}
	}()
}
