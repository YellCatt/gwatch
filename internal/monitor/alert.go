package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

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

func sendAlertEmail(result MonitorResult) {
	if !email.Config.Enabled {
		return
	}

	tc := result.TestCase

	alertInterval := time.Duration(config.GlobalConfig.Monitor.AlertInterval) * time.Second
	if alertInterval <= 0 {
		alertInterval = 6 * time.Hour
	}

	lastAlertMu.Lock()
	if last, ok := lastAlertTime[tc.ID]; ok && timeutil.Now().Sub(last) < alertInterval {
		lastAlertMu.Unlock()
		logger.Info("Alert email suppressed due to alert interval",
			zap.String("id", tc.ID),
			zap.Duration("since_last", timeutil.Now().Sub(last)),
			zap.Duration("interval", alertInterval))
		return
	}
	lastAlertTime[tc.ID] = timeutil.Now()
	lastAlertMu.Unlock()

	alertLevel := "WARNING"
	alertIcon := "⚠️"
	if result.AlertType == "failure" {
		alertLevel = "CRITICAL"
		alertIcon = "🚨"
	} else if result.AlertType == "slow" {
		alertLevel = "WARNING"
		alertIcon = "⏱️"
	}

	subject := fmt.Sprintf("[%s] gwatch 接口监控告警 - %s", alertLevel, tc.ID)
	body := fmt.Sprintf(`%s ===== 接口监控告警 ===== %s

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


【时间信息】
  开始时间:   %s
  结束时间:   %s

来自 gwatch 接口监控系统`,
		alertIcon, alertIcon,
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
		tc.URL,
		tc.Method,
		timeutil.FormatDateTime(result.Result.StartTime),
		timeutil.FormatDateTime(result.Result.EndTime),
	)

	saveAlertRecord(body, tc.ID)

	if err := email.SendCustomEmail(subject, body); err != nil {
		logger.Warn("Failed to send alert email", zap.Error(err))
	}
}

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

func getDeviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return "Unknown"
	}
	return name
}

func sendStartupNotification(taskCount int) {
	if !email.Config.Enabled {
		logger.Info("Email is disabled, skipping startup notification")
		return
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Unknown"
	}

	subject := "[gwatch] 监控服务已启动"
	body := fmt.Sprintf(`
gwatch 接口监控服务启动通知


【设备名称】%s
【启动时间】%s
【监控任务数】%d

【状态】监控服务已成功启动，开始执行监控任务。

来自 gwatch 接口监控系统`, hostname, timeutil.FormatDateTime(timeutil.Now()), taskCount)

	logger.Info("Sending startup notification email")
	err := email.SendCustomEmail(subject, body)
	if err != nil {
		logger.Error("Failed to send startup notification email", zap.Error(err))
	}
}