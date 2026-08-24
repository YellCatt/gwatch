package monitor

import (
	"fmt"

	"gwatch/internal/assert"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/psv"
	"gwatch/internal/testcase"
	"gwatch/internal/timeutil"
	"gwatch/internal/vars"
)

// checkAlerts 根据测试用例配置和执行结果检测是否触发告警，
// 依次校验状态码断言、响应体断言、执行失败和慢响应四种类型。
func checkAlerts(result *MonitorResult) {
	tc := result.TestCase

	statusErr := checkStatusCode(tc, result.Result)
	if statusErr != "" {
		result.StatusCodeOk = false
		triggerFailureAlert(result, statusErr)
		return
	}
	result.StatusCodeOk = true

	if errMsg := checkBodyAssertions(tc, result.Result); errMsg != "" {
		result.AssertionOk = false
		result.AssertionText = errMsg
		triggerFailureAlert(result, errMsg)
		return
	}
	result.AssertionOk = true
	result.AssertionText = "断言通过"

	if !result.Result.Passed && tc.AlertOnFailure {
		result.AlertType = "failure"
		result.AlertMsg = fmt.Sprintf("接口监控告警: [%s] %s 执行失败 - %s", tc.ID, tc.Desc, result.Result.Error)
		logger.Warn(result.AlertMsg)
		return
	}

	if tc.ResponseThreshold > 0 && result.Result.Duration.Milliseconds() > int64(tc.ResponseThreshold) && tc.AlertOnSlow {
		result.AlertType = "slow"
		result.AlertMsg = fmt.Sprintf("接口监控告警: [%s] %s 响应超时 - 耗时 %.2fms > 阈值 %dms",
			tc.ID, tc.Desc, float64(result.Result.Duration.Milliseconds()), tc.ResponseThreshold)
		logger.Warn(result.AlertMsg)
	}
}

// checkStatusCode 校验 HTTP 状态码是否符合预期。
// 如果配置了 ExpectedStatus 则精确匹配，否则默认检查是否为 2xx。
func checkStatusCode(tc psv.TestCase, result testcase.TestResult) string {
	actual := result.ActualStatus
	if actual == 0 {
		return "接口无响应，状态码为 0"
	}

	if tc.ExpectedStatus > 0 {
		if actual != tc.ExpectedStatus {
			return fmt.Sprintf("状态码断言失败: 期望 %d，实际 %d", tc.ExpectedStatus, actual)
		}
		return ""
	}

	if actual < 200 || actual >= 300 {
		return fmt.Sprintf("状态码异常: %d（期望 2xx）", actual)
	}

	return ""
}

// checkBodyAssertions 校验响应体是否符合配置的断言规则（正则 + JSON 结构）。
func checkBodyAssertions(tc psv.TestCase, result testcase.TestResult) string {
	body := result.ResponseBody

	if tc.BodyRegex != "" {
		if ok, errMsg := assert.BodyRegexMatch(body, tc.BodyRegex); !ok {
			return fmt.Sprintf("响应体正则断言失败: %s", errMsg)
		}
	}

	if tc.ExpectedBody != "" {
		expectedBody := assert.CompactBody(vars.Replace(tc.ExpectedBody))
		if ok, errMsg := assert.JSONMatch(expectedBody, body, tc.MatchMode); !ok {
			return fmt.Sprintf("响应体断言失败: %s", errMsg)
		}
	}

	return ""
}

// triggerFailureAlert 触发失败告警，设置告警类型和消息。
func triggerFailureAlert(result *MonitorResult, errMsg string) {
	tc := result.TestCase
	if !tc.AlertOnFailure {
		return
	}
	result.AlertType = "failure"
	result.AlertMsg = fmt.Sprintf("接口监控告警: [%s] %s %s", tc.ID, tc.Desc, errMsg)
	logger.Warn(result.AlertMsg)
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

	statusCode := result.Result.ActualStatus
	assertion := result.AssertionText
	if assertion == "" {
		assertion = result.Result.Error
	}

	email.DispatchAlert(email.UnifiedAlert{
		Source:        email.SourceAPI,
		SourceName:    "接口监控",
		TargetName:    tc.ID,
		MetricName:    result.AlertType,
		MetricAlias:   tc.Desc,
		Value:         float64(result.Result.Duration.Milliseconds()),
		Unit:          "ms",
		Threshold:     float64(tc.ResponseThreshold),
		AlertLevel:    alertLevel,
		Message:       result.AlertMsg,
		Timestamp:     timeutil.Now(),
		StatusCode:    statusCode,
		Assertion:     assertion,
		StatusCodeOk:  result.StatusCodeOk,
		AssertionOk:   result.AssertionOk,
	})
}