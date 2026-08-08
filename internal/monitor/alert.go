package monitor

import (
	"fmt"

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

// sendAlertEmail 将告警发送到统一调度器，由调度器批量处理后统一发送邮件。
func sendAlertEmail(result MonitorResult) {
	if !email.Config.Enabled {
		return
	}

	tc := result.TestCase

	alertLevel := "WARNING"
	if result.AlertType == "failure" {
		alertLevel = "CRITICAL"
	}

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
