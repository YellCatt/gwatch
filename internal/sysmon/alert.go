package sysmon

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/storage"
	"gwatch/internal/timeutil"
	"gwatch/internal/util"
)

// CheckAlerts 根据系统指标与配置阈值对比，返回触发的告警列表。
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

	for _, p := range metric.Partitions {
		if p.Percent >= cfg.DiskUsageThreshold {
			alerts = append(alerts, AlertItem{
				Metric:    fmt.Sprintf("分区使用率(%s)", p.MountPoint),
				Value:     p.Percent,
				Threshold: cfg.DiskUsageThreshold,
				Unit:      "%",
				Message:   fmt.Sprintf("分区 %s 使用率 %.2f%% 超过阈值 %.2f%%", p.MountPoint, p.Percent, cfg.DiskUsageThreshold),
				Level:     "WARNING",
				Timestamp: metric.Timestamp,
			})
		}
	}

	if metric.NetDownKBps >= cfg.NetworkDownThreshold {
		alerts = append(alerts, AlertItem{
			Metric:    "网络下行速度",
			Value:     metric.NetDownKBps,
			Threshold: cfg.NetworkDownThreshold,
			Unit:      "KB/s",
			Message:   fmt.Sprintf("网络下行速度 %s 超过严重阈值 %s", formatSpeed(metric.NetDownKBps), formatSpeed(cfg.NetworkDownThreshold)),
			Level:     "CRITICAL",
			Timestamp: metric.Timestamp,
		})
	} else if cfg.NetworkDownWarnThreshold > 0 && metric.NetDownKBps >= cfg.NetworkDownWarnThreshold {
		alerts = append(alerts, AlertItem{
			Metric:    "网络下行速度",
			Value:     metric.NetDownKBps,
			Threshold: cfg.NetworkDownWarnThreshold,
			Unit:      "KB/s",
			Message:   fmt.Sprintf("网络下行速度 %s 超过警告阈值 %s", formatSpeed(metric.NetDownKBps), formatSpeed(cfg.NetworkDownWarnThreshold)),
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
			Message:   fmt.Sprintf("网络上行速度 %s 超过严重阈值 %s", formatSpeed(metric.NetUpKBps), formatSpeed(cfg.NetworkUpThreshold)),
			Level:     "CRITICAL",
			Timestamp: metric.Timestamp,
		})
	} else if cfg.NetworkUpWarnThreshold > 0 && metric.NetUpKBps >= cfg.NetworkUpWarnThreshold {
		alerts = append(alerts, AlertItem{
			Metric:    "网络上行速度",
			Value:     metric.NetUpKBps,
			Threshold: cfg.NetworkUpWarnThreshold,
			Unit:      "KB/s",
			Message:   fmt.Sprintf("网络上行速度 %s 超过警告阈值 %s", formatSpeed(metric.NetUpKBps), formatSpeed(cfg.NetworkUpWarnThreshold)),
			Level:     "WARNING",
			Timestamp: metric.Timestamp,
		})
	}

	if len(alerts) > 0 {
		allProcs := CollectAllProcesses()
		for i := range alerts {
			var sortBy ProcessSortBy
			var label string
			switch alerts[i].Metric {
			case "CPU使用率":
				sortBy = SortByCPU
				label = "CPU 占用 Top 5 进程"
			case "内存使用率":
				sortBy = SortByMem
				label = "内存占用 Top 5 进程"
			case "网络下行速度", "网络上行速度":
				sortBy = SortByNet
				label = "网络占用 Top 5 进程"
			default:
				continue
			}
			sorted := SortProcesses(allProcs, sortBy)
			if len(sorted) > 5 {
				sorted = sorted[:5]
			}
			alerts[i].TopProcesses = sorted
			alerts[i].ProcessLabel = label
		}
	}

	return alerts
}

// DispatchSystemAlerts 将系统告警分发到统一告警调度器。
func DispatchSystemAlerts(alerts []AlertItem) {
	if !config.GlobalConfig.SystemMon.EmailEnabled {
		return
	}
	if !email.Config.Enabled {
		return
	}
	if len(alerts) == 0 {
		return
	}

	for _, a := range alerts {
		emailAlert := email.UnifiedAlert{
			Source:      email.SourceSystem,
			SourceName:  "系统资源监控",
			TargetName:  a.Metric,
			MetricName:  a.Metric,
			MetricAlias: a.Metric,
			Value:       a.Value,
			Unit:        a.Unit,
			Threshold:   a.Threshold,
			AlertLevel:  a.Level,
			Message:     a.Message,
			Timestamp:   a.Timestamp,
		}
		if len(a.TopProcesses) > 0 {
			var procMsgs []string
			for i, p := range a.TopProcesses {
				switch a.Metric {
				case "内存使用率":
					procMsgs = append(procMsgs, fmt.Sprintf("  %d. %s (PID:%d, MEM:%.2f%%, CPU:%.2f%%, MEM:%s)",
						i+1, p.Name, p.PID, p.MemPercent, p.CPUPercent, util.FormatBytes(p.MemUsed)))
				case "网络下行速度", "网络上行速度":
					procMsgs = append(procMsgs, fmt.Sprintf("  %d. %s (PID:%d, NET↓: %s, NET↑: %s, CPU:%.2f%%, MEM:%.2f%%)",
						i+1, p.Name, p.PID, util.FormatSpeed(p.NetDownKBps), util.FormatSpeed(p.NetUpKBps), p.CPUPercent, p.MemPercent))
				default:
					procMsgs = append(procMsgs, fmt.Sprintf("  %d. %s (PID:%d, CPU:%.2f%%, MEM:%.2f%%, NET↓: %s, NET↑: %s)",
						i+1, p.Name, p.PID, p.CPUPercent, p.MemPercent, util.FormatSpeed(p.NetDownKBps), util.FormatSpeed(p.NetUpKBps)))
				}
			}
			emailAlert.TopProcesses = procMsgs
			emailAlert.TopProcessesLabel = a.ProcessLabel
		}
		email.DispatchAlert(emailAlert)

		dateStr := a.Timestamp.Format("2006-01-02")
		timestampStr := a.Timestamp.Format("2006-01-02 15:04:05")
		storage.UpdateSystemAlertSummary(storage.SystemAlertRecord{
			Date:            dateStr,
			Metric:          a.Metric,
			MetricAlias:     a.Metric,
			Value:           a.Value,
			Threshold:       a.Threshold,
			Unit:            a.Unit,
			AlertLevel:      a.Level,
			FirstOccurrence: timestampStr,
			LastOccurrence:  timestampStr,
			Message:         a.Message,
		})
	}
}

// SendSystemStatusEmail 发送系统状态邮件：汇总最新指标并生成状态报告通过邮件发送。
func SendSystemStatusEmail(metrics []SystemMetric) error {
	if !config.GlobalConfig.SystemMon.EmailEnabled {
		return nil
	}
	if !email.Config.Enabled {
		logger.Info("全局邮件已禁用，跳过系统监控邮件")
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

`, hostname,
		now,
		config.GlobalConfig.SystemMon.Interval,
		config.GlobalConfig.SystemMon.CPUThreshold,
		config.GlobalConfig.SystemMon.MemoryThreshold,
		config.GlobalConfig.SystemMon.DiskUsageThreshold,
		formatSpeed(config.GlobalConfig.SystemMon.NetworkDownThreshold),
		formatSpeed(config.GlobalConfig.SystemMon.NetworkUpThreshold),
		config.GlobalConfig.SystemMon.ChartEnabled,
		config.GlobalConfig.SystemMon.EmailEnabled))

	var reportMetrics []SystemMetric
	if len(metrics) > 0 {
		reportMetrics = metrics
	} else {
		recent, err := LoadRecentMetrics(24)
		if err != nil {
			logger.Error("加载近期指标用于系统邮件失败", zap.Error(err))
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

	logger.Info("正在发送系统状态邮件", zap.String("主题", subject))
	return email.SendCustomEmail(subject, bodyBuilder.String())
}

func formatSpeed(kbps float64) string {
	return util.FormatSpeed(kbps)
}
