package sysmon

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

var (
	lastAlertTimes = make(map[string]time.Time)
	lastAlertMu    sync.Mutex
)

func CheckAlerts(metric SystemMetric) []AlertItem {
	cfg := config.GlobalConfig.SystemMon
	var alerts []AlertItem

	if metric.CPUPercent >= cfg.CPUThreshold {
		alerts = append(alerts, AlertItem{
			Metric:    "CPU使用率",
			Value:     metric.CPUPercent,
			Threshold: cfg.CPUThreshold,
			Unit:      "%",
			Message:   fmt.Sprintf("CPU 使用率 %.2f%% 超过阈值 %.2f%%", metric.CPUPercent, cfg.CPUThreshold),
			Level:     "CRITICAL",
			Timestamp: metric.Timestamp,
		})
	}

	if metric.MemoryPercent >= cfg.MemoryThreshold {
		alerts = append(alerts, AlertItem{
			Metric:    "内存使用率",
			Value:     metric.MemoryPercent,
			Threshold: cfg.MemoryThreshold,
			Unit:      "%",
			Message:   fmt.Sprintf("内存使用率 %.2f%% 超过阈值 %.2f%%", metric.MemoryPercent, cfg.MemoryThreshold),
			Level:     "CRITICAL",
			Timestamp: metric.Timestamp,
		})
	}

	if metric.DiskPercent >= cfg.DiskUsageThreshold {
		alerts = append(alerts, AlertItem{
			Metric:    "磁盘使用率",
			Value:     metric.DiskPercent,
			Threshold: cfg.DiskUsageThreshold,
			Unit:      "%",
			Message:   fmt.Sprintf("磁盘使用率 %.2f%% 超过阈值 %.2f%%", metric.DiskPercent, cfg.DiskUsageThreshold),
			Level:     "WARNING",
			Timestamp: metric.Timestamp,
		})
	}

	if metric.NetDownKBps >= cfg.NetworkDownThreshold {
		alerts = append(alerts, AlertItem{
			Metric:    "网络下行速度",
			Value:     metric.NetDownKBps,
			Threshold: cfg.NetworkDownThreshold,
			Unit:      "KB/s",
			Message:   fmt.Sprintf("网络下行速度 %s 超过阈值 %s", formatSpeed(metric.NetDownKBps), formatSpeed(cfg.NetworkDownThreshold)),
			Level:     "WARNING",
			Timestamp: metric.Timestamp,
		})
	}

	if metric.NetUpKBps >= cfg.NetworkUpThreshold {
		alerts = append(alerts, AlertItem{
			Metric:    "网络上行速度",
			Value:     metric.NetUpKBps,
			Threshold: cfg.NetworkUpThreshold,
			Unit:      "KB/s",
			Message:   fmt.Sprintf("网络上行速度 %s 超过阈值 %s", formatSpeed(metric.NetUpKBps), formatSpeed(cfg.NetworkUpThreshold)),
			Level:     "WARNING",
			Timestamp: metric.Timestamp,
		})
	}

	return alerts
}

func ShouldSendAlert(alert AlertItem) bool {
	lastAlertMu.Lock()
	defer lastAlertMu.Unlock()

	cooldown := time.Duration(config.GlobalConfig.SystemMon.AlertCooldown) * time.Second
	if cooldown <= 0 {
		cooldown = 5 * time.Minute
	}

	last, ok := lastAlertTimes[alert.Metric]
	if ok && time.Since(last) < cooldown {
		return false
	}

	lastAlertTimes[alert.Metric] = time.Now()
	return true
}

func ClearAlertCooldown(metric string) {
	lastAlertMu.Lock()
	defer lastAlertMu.Unlock()
	delete(lastAlertTimes, metric)
}

func SendAlertEmail(alerts []AlertItem) error {
	if !config.GlobalConfig.SystemMon.EmailEnabled {
		logger.Info("System monitor email alerts disabled")
		return nil
	}

	if !email.Config.Enabled {
		logger.Warn("Email is disabled globally, skipping system alert email")
		return nil
	}

	if len(alerts) == 0 {
		return nil
	}

	var filteredAlerts []AlertItem
	for _, a := range alerts {
		if ShouldSendAlert(a) {
			filteredAlerts = append(filteredAlerts, a)
		}
	}

	if len(filteredAlerts) == 0 {
		return nil
	}

	hostname, _ := GetHostInfo()

	var criticalCount, warningCount int
	for _, a := range filteredAlerts {
		if a.Level == "CRITICAL" {
			criticalCount++
		} else if a.Level == "WARNING" {
			warningCount++
		}
	}

	subject := fmt.Sprintf("【系统告警】%s - %d项告警 (严重:%d 警告:%d)",
		timeutil.FormatDateTime(timeutil.Now()), len(filteredAlerts), criticalCount, warningCount)

	var body strings.Builder
	body.WriteString("===== 系统资源监控告警报告 =====\n\n")
	body.WriteString(fmt.Sprintf("告警时间: %s\n", timeutil.FormatDateTime(timeutil.Now())))
	body.WriteString(fmt.Sprintf("监控设备: %s\n\n", hostname))
	body.WriteString(fmt.Sprintf("告警数量: %d (严重:%d 警告:%d)\n\n", len(filteredAlerts), criticalCount, warningCount))

	for _, a := range filteredAlerts {
		icon := "⚠️"
		if a.Level == "CRITICAL" {
			icon = "🚨"
		}
		body.WriteString(fmt.Sprintf("%s [%s] %s\n", icon, a.Level, a.Metric))
		if a.Unit == "KB/s" {
			body.WriteString(fmt.Sprintf("   当前值:  %s\n", formatSpeed(a.Value)))
			body.WriteString(fmt.Sprintf("   阈值:    %s\n", formatSpeed(a.Threshold)))
		} else {
			body.WriteString(fmt.Sprintf("   当前值:  %.2f %s\n", a.Value, a.Unit))
			body.WriteString(fmt.Sprintf("   阈值:    %.2f %s\n", a.Threshold, a.Unit))
		}
		body.WriteString(fmt.Sprintf("   消息:    %s\n\n", a.Message))
	}

	body.WriteString("===== 报告结束 =====\n")
	body.WriteString("来自 gwatch 系统监控")

	logger.Info("Sending system alert email", zap.Int("alerts", len(filteredAlerts)))
	return email.SendCustomEmail(subject, body.String())
}

func SendSystemStatusEmail(metrics []SystemMetric) error {
	if !config.GlobalConfig.SystemMon.EmailEnabled {
		return nil
	}
	if !email.Config.Enabled {
		logger.Info("Email is disabled globally, skipping system monitor email")
		return nil
	}

	hostname, _ := GetHostInfo()
	now := timeutil.FormatDateTime(timeutil.Now())

	var bodyBuilder strings.Builder

	bodyBuilder.WriteString(fmt.Sprintf(`gwatch 系统资源监控服务通知

【设备名称】%s
【通知时间】%s
【采集间隔】%d 秒

【监控阈值】
  CPU:        %.0f%%
  内存:       %.0f%%
  磁盘:       %.0f%%
  网络下行:   %s
  网络上行:   %s

【功能状态】
  图表生成:   %v
  邮件告警:   %v
  数据保留:   %d 小时

`, hostname,
		now,
		config.GlobalConfig.SystemMon.Interval,
		config.GlobalConfig.SystemMon.CPUThreshold,
		config.GlobalConfig.SystemMon.MemoryThreshold,
		config.GlobalConfig.SystemMon.DiskUsageThreshold,
		formatSpeed(config.GlobalConfig.SystemMon.NetworkDownThreshold),
		formatSpeed(config.GlobalConfig.SystemMon.NetworkUpThreshold),
		config.GlobalConfig.SystemMon.ChartEnabled,
		config.GlobalConfig.SystemMon.EmailEnabled,
		config.GlobalConfig.SystemMon.RetentionHours))

	var reportMetrics []SystemMetric
	if len(metrics) > 0 {
		reportMetrics = metrics
	} else {
		recent, err := LoadRecentMetrics(24)
		if err != nil {
			logger.Warn("Failed to load recent metrics for system email", zap.Error(err))
		} else {
			reportMetrics = recent
		}
	}

	if len(reportMetrics) > 0 {
		latest := reportMetrics[len(reportMetrics)-1]
		alerts := CheckAlerts(latest)
		report := GenerateSystemReport(reportMetrics, alerts)
		bodyBuilder.WriteString("【系统状态报告】\n\n")
		bodyBuilder.WriteString(report)
		bodyBuilder.WriteString(fmt.Sprintf("\n设备: %s\n", hostname))
	} else {
		bodyBuilder.WriteString("【历史数据】暂无历史数据（服务首次启动，待采集积累数据后下次启动将显示趋势图表）\n")
	}

	bodyBuilder.WriteString("\n来自 gwatch 系统监控")

	var subject string
	if len(metrics) > 0 {
		latest := metrics[len(metrics)-1]
		subject = fmt.Sprintf("【系统状态报告】%s - CPU:%.1f%% MEM:%.1f%% DISK:%.1f%%",
			now, latest.CPUPercent, latest.MemoryPercent, latest.DiskPercent)
	} else {
		subject = "[gwatch] 系统资源监控服务已启动"
	}

	logger.Info("Sending system status email", zap.String("subject", subject))
	return email.SendCustomEmail(subject, bodyBuilder.String())
}