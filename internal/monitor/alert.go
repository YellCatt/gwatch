package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

// checkAlerts 根据测试用例配置和执行结果检测是否触发告警，
// 包括失败告警和慢响应告警两种类型。
func checkAlerts(result *MonitorResult) {
	tc := result.TestCase

	if !result.Result.Passed && tc.AlertOnFailure {
		result.AlertType = "failure"
		result.AlertMsg = fmt.Sprintf("接口监控告警: [%s] %s 执行失败 - %s", tc.ID, tc.Desc, result.Result.Error)
		logger.Error(result.AlertMsg)
		return
	}

	if tc.ResponseThreshold > 0 && result.Result.Duration.Milliseconds() > int64(tc.ResponseThreshold) && tc.AlertOnSlow {
		result.AlertType = "slow"
		result.AlertMsg = fmt.Sprintf("接口监控告警: [%s] %s 响应超时 - 耗时 %.2fms > 阈值 %dms",
			tc.ID, tc.Desc, float64(result.Result.Duration.Milliseconds()), tc.ResponseThreshold)
		logger.Warn(result.AlertMsg)
	}
}

// sendAlertEmail 发送告警邮件到统一调度器，同时保存本地告警记录。
func sendAlertEmail(result MonitorResult) {
	if !email.Config.Enabled {
		return
	}

	tc := result.TestCase

	alertLevel := "WARNING"
	if result.AlertType == "failure" {
		alertLevel = "CRITICAL"
	}

	requestParams := formatMap(tc.Params)
	requestHeaders := result.Result.RequestHeaders
	requestBody := truncateStr(result.Result.RequestBody, 2000)
	responseBody := truncateStr(result.Result.ResponseBody, 2000)

	body := fmt.Sprintf(`===== 接口监控告警 =====

【告警级别】%s
【告警时间】%s
【监控设备】%s

【测试用例】
  ID:         %s
  描述:       %s
  监控周期:   %ds

【告警详情】
  类型:       %s
  消息:       %s

【执行结果】
  状态:       %s
  耗时:       %.2fms
  HTTP状态码: %d

【请求信息】
  URL:        %s
  方法:       %s
  URL参数:    %s
  请求头:     %s
  请求体:     %s

【响应信息】
  响应体:     %s

【时间信息】
  开始时间:   %s
  结束时间:   %s

来自 gwatch 接口监控系统`,
		alertLevel,
		timeutil.FormatDateTime(timeutil.Now()),
		getDeviceName(),
		tc.ID,
		tc.Desc,
		tc.MonitorInterval,
		result.AlertType,
		result.AlertMsg,
		map[bool]string{true: "✅ 通过", false: "❌ 失败"}[result.Result.Passed],
		float64(result.Result.Duration.Milliseconds()),
		result.Result.ActualStatus,
		result.Result.ProcessedURL,
		tc.Method,
		requestParams,
		requestHeaders,
		requestBody,
		responseBody,
		timeutil.FormatDateTime(result.Result.StartTime),
		timeutil.FormatDateTime(result.Result.EndTime),
	)

	saveAlertRecord(body, tc.ID)

	email.DispatchAlert(email.UnifiedAlert{
		Source:      email.SourceAPI,
		SourceName:  "接口监控",
		TargetName:  tc.ID,
		MetricName:  result.AlertType,
		MetricAlias: tc.Desc,
		Value:       float64(result.Result.Duration.Milliseconds()),
		Unit:        "ms",
		Threshold:   float64(tc.ResponseThreshold),
		AlertLevel:  alertLevel,
		Message:     result.AlertMsg,
		Timestamp:   timeutil.Now(),
	})
}

// saveAlertRecord 将告警内容以日志文件形式保存到 reports/alerts/<date>/ 目录。
func saveAlertRecord(content, testCaseID string) {
	reportDir := config.GlobalConfig.App.ReportDir
	if reportDir == "" {
		reportDir = "./reports"
	}

	alertDir := filepath.Join(reportDir, "alerts", timeutil.Now().Format("20060102"))
	if err := os.MkdirAll(alertDir, 0755); err != nil {
		logger.Warn("Failed to create alert directory", zap.Error(err))
		return
	}

	timestamp := timeutil.Now().Format("150405")
	filename := fmt.Sprintf("alert_%s_%s.log", timestamp, testCaseID)
	filePath := filepath.Join(alertDir, filename)

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		logger.Warn("Failed to save alert record", zap.String("file", filePath), zap.Error(err))
	} else {
		logger.Info("Alert record saved", zap.String("file", filePath))
	}
}

// getDeviceName 获取当前主机名，用于在告警邮件中标识监控设备。
func getDeviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return "Unknown"
	}
	return name
}

// formatMap 将 map[string]string 格式化为 "k1=v1&k2=v2" 形式的字符串，空 map 返回 "(无)"。
func formatMap(m map[string]string) string {
	if len(m) == 0 {
		return "(无)"
	}
	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, "&")
}

// truncateStr 截断字符串，超过 maxLen 的部分以 "...(已截断)" 结尾。空字符串返回 "(无)"。
func truncateStr(s string, maxLen int) string {
	if s == "" {
		return "(无)"
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(已截断)"
}
