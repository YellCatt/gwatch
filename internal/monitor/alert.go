package monitor

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
	"gwatch/internal/sysmon"
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

	var sysSection string
	if config.GlobalConfig.SystemMon.Enabled {
		sysSection = buildStartupSystemSection()
	}

	body := fmt.Sprintf(`
gwatch 接口监控服务启动通知


【设备名称】%s
【启动时间】%s
【监控任务数】%d

【状态】监控服务已成功启动，开始执行监控任务。
%s
来自 gwatch 接口监控系统`, hostname, timeutil.FormatDateTime(timeutil.Now()), taskCount, sysSection)

	logger.Info("Sending startup notification email")
	err := email.SendCustomEmail(subject, body)
	if err != nil {
		logger.Error("Failed to send startup notification email", zap.Error(err))
	}
}

func buildStartupSystemSection() string {
	var builder strings.Builder

	metric, err := sysmon.CollectMetrics()
	if err != nil {
		return fmt.Sprintf("\n🖥️ 系统资源状态\n  获取系统指标失败: %v\n", err)
	}

	builder.WriteString("\n🖥️ 系统资源状态\n\n")

	builder.WriteString(fmt.Sprintf("  CPU 使用率:     %.2f %s\n", metric.CPUPercent, "%"))
	builder.WriteString(fmt.Sprintf("  内存使用率:     %.2f %s (%s)\n",
		metric.MemoryPercent, "%",
		sysmon.FormatBytes(metric.MemoryUsed, metric.MemoryTotal)))
	builder.WriteString(fmt.Sprintf("  磁盘使用率:     %.2f %s (%s)\n",
		metric.DiskPercent, "%",
		sysmon.FormatBytes(metric.DiskUsed, metric.DiskTotal)))
	builder.WriteString(fmt.Sprintf("  网络下行速度:   %.2f KB/s\n", metric.NetDownKBps))
	builder.WriteString(fmt.Sprintf("  网络上行速度:   %.2f KB/s\n", metric.NetUpKBps))
	builder.WriteString(fmt.Sprintf("  磁盘读取速度:   %.2f KB/s\n", metric.DiskReadKBps))
	builder.WriteString(fmt.Sprintf("  磁盘写入速度:   %.2f KB/s\n", metric.DiskWriteKBps))

	cfg := config.GlobalConfig.SystemMon
	alerts := sysmon.CheckAlerts(metric)
	if len(alerts) > 0 {
		builder.WriteString("\n  ⚠️ 当前系统告警:\n")
		for _, a := range alerts {
			icon := "⚠️"
			if a.Level == "CRITICAL" {
				icon = "🚨"
			}
			builder.WriteString(fmt.Sprintf("  %s [%s] %s: %.2f %s (阈值: %.2f %s)\n",
				icon, a.Level, a.Metric, a.Value, a.Unit, a.Threshold, a.Unit))
		}
	}

	metrics, loadErr := sysmon.LoadRecentMetrics(1)
	if loadErr == nil && len(metrics) > 1 {
		cpuData := make([]float64, len(metrics))
		memData := make([]float64, len(metrics))
		diskData := make([]float64, len(metrics))
		for i, m := range metrics {
			cpuData[i] = m.CPUPercent
			memData[i] = m.MemoryPercent
			diskData[i] = m.DiskPercent
		}

		builder.WriteString("\n  📈 历史趋势 (近1小时)\n\n")
		builder.WriteString("  【CPU 使用率】\n")
		builder.WriteString(sysmon.GenerateASCIIChart(cpuData, 30, "%", cfg.CPUThreshold))
		builder.WriteString("\n  【内存使用率】\n")
		builder.WriteString(sysmon.GenerateASCIIChart(memData, 30, "%", cfg.MemoryThreshold))
		builder.WriteString("\n  【磁盘使用率】\n")
		builder.WriteString(sysmon.GenerateASCIIChart(diskData, 30, "%", cfg.DiskUsageThreshold))
		builder.WriteString("\n")
	}

	return builder.String()
}