package report

import (
	"fmt"
	"strings"
	"time"

	"gwatch/config"
	"gwatch/internal/sysmon"
	"gwatch/internal/timeutil"
)

func (r *Report) GenerateReportContent() string {
	var builder strings.Builder

	periodNames := map[ReportPeriod]string{
		PeriodDaily:   "每日",
		PeriodWeekly:  "每周",
		PeriodMonthly: "每月",
		PeriodYearly:  "年度",
	}

	builder.WriteString(fmt.Sprintf(`
gwatch %s运维报告


  【报告周期】%s ~ %s
  【生成时间】%s
  【监控设备】%s
`, periodNames[r.Period], r.StartDate, r.EndDate, timeutil.FormatDateTime(timeutil.Now()), getDeviceName()))

	successRate := 0.0
	if r.TotalTasks > 0 {
		successRate = float64(r.SuccessTasks) / float64(r.TotalTasks) * 100
	}

	builder.WriteString(fmt.Sprintf(`
📊 执行概览
  总执行次数:  %d 次
  成功次数:    %d 次
  失败次数:    %d 次
  成功率:      %.2f%%
  平均响应时间: %s

`, r.TotalTasks, r.SuccessTasks, r.FailedTasks, successRate, formatAvgDuration(r.InterfaceStats)))

	builder.WriteString("🔧 接口状态详情\n\n")
	if len(r.InterfaceStats) > 0 {
		builder.WriteString(fmt.Sprintf("%-32s %-8s %-8s %-8s %-12s %-10s\n",
			"接口ID", "总次数", "成功", "失败", "平均耗时", "最大耗时"))
		builder.WriteString(strings.Repeat("-", 90) + "\n")
		for _, stat := range r.InterfaceStats {
			successIcon := "✅"
			if stat.FailedCount > 0 {
				successIcon = "⚠️"
			}
			builder.WriteString(fmt.Sprintf("%s %-30s %-8d %-8d %-8d %-12s %-10s\n",
				successIcon,
				stat.TaskID,
				stat.TotalCount,
				stat.SuccessCount,
				stat.FailedCount,
				formatDuration(stat.AvgDurationMS),
				formatDuration(stat.MaxDurationMS)))
		}
	} else {
		builder.WriteString("  暂无接口数据\n")
	}

	builder.WriteString("\n📢 告警汇总\n\n")
	if len(r.AggregatedErrors) > 0 {
		criticalCount := 0
		warningCount := 0
		for _, err := range r.AggregatedErrors {
			_, displayName := alertLevelDisplay(err.AlertLevel)
			icon, _ := alertLevelDisplay(err.AlertLevel)
			if err.AlertLevel == "CRITICAL" {
				criticalCount++
			} else if err.AlertLevel == "WARNING" {
				warningCount++
			}
			builder.WriteString(fmt.Sprintf("%s 【%s】%s - %s\n", icon, displayName, err.TaskID, err.TaskDesc))
			builder.WriteString(fmt.Sprintf("   URL: %s %s\n", err.Method, err.URL))
			builder.WriteString(fmt.Sprintf("   期望状态码: %d\n", err.ExpectedStatus))
			builder.WriteString(fmt.Sprintf("   告警次数: %d 次\n", err.AlertCount))
			builder.WriteString(fmt.Sprintf("   首次告警: %s\n", timeutil.FormatDateTime(err.FirstOccurrence)))
			builder.WriteString(fmt.Sprintf("   最近告警: %s\n", timeutil.FormatDateTime(err.LastOccurrence)))
			builder.WriteString(fmt.Sprintf("   错误信息: %s\n\n", err.ErrorMsg))
		}
		builder.WriteString(fmt.Sprintf("  合计: CRITICAL %d 项, WARNING %d 项, 共 %d 项告警\n", criticalCount, warningCount, len(r.AggregatedErrors)))
	} else {
		builder.WriteString("  ✅ 无告警\n")
	}

	builder.WriteString("\n\n")

	builder.WriteString(r.generateSystemResourceSection())

	builder.WriteString("\n\n")
	builder.WriteString("来自 gwatch 接口监控系统\n")

	return builder.String()
}

func (r *Report) GenerateHourlyResourceContent() string {
	var builder strings.Builder

	periodNames := map[ReportPeriod]string{
		PeriodDaily:   "每日",
		PeriodWeekly:  "每周",
		PeriodMonthly: "每月",
		PeriodYearly:  "年度",
	}

	builder.WriteString(fmt.Sprintf(`
  📈 %s报表 - 每小时资源指标趋势

  【报告周期】%s ~ %s
`, periodNames[r.Period], r.StartDate, r.EndDate))

	if len(r.HourlyMetrics) == 0 {
		builder.WriteString("  （无小时级别指标数据）\n")
		return builder.String()
	}

	for _, metric := range r.HourlyMetrics {
		builder.WriteString(fmt.Sprintf("  🖥️ %s - %s (%s)\n", metric.TargetName, metric.MetricAlias, metric.Unit))
		builder.WriteString(fmt.Sprintf("     %-6s", "时刻"))
		for i := 0; i < 24; i++ {
			builder.WriteString(fmt.Sprintf(" %02d:00", i))
		}
		builder.WriteString("\n     ")
		builder.WriteString(strings.Repeat("-", 6+7*24))
		builder.WriteString("\n     ")
		for _, data := range metric.HourlyData {
			if data.AvgValue >= 0 {
				builder.WriteString(fmt.Sprintf(" %7.1f", data.AvgValue))
			} else {
				builder.WriteString(fmt.Sprintf(" %7s", "-"))
			}
		}
		builder.WriteString("\n\n")
	}

	return builder.String()
}

func (r *Report) GenerateDailyResourceContent() string {
	var builder strings.Builder

	periodNames := map[ReportPeriod]string{
		PeriodDaily:   "每日",
		PeriodWeekly:  "每周",
		PeriodMonthly: "每月",
		PeriodYearly:  "年度",
	}

	builder.WriteString(fmt.Sprintf(`
  📈 %s报表 - 每日资源指标趋势

  【报告周期】%s ~ %s
`, periodNames[r.Period], r.StartDate, r.EndDate))

	if len(r.DailyMetrics) == 0 {
		builder.WriteString("  （无日级别指标数据）\n")
		return builder.String()
	}

	for _, metric := range r.DailyMetrics {
		builder.WriteString(fmt.Sprintf("  🖥️ %s - %s (%s)\n", metric.TargetName, metric.MetricAlias, metric.Unit))
		builder.WriteString(fmt.Sprintf("     %-6s", "日期"))
		for _, data := range metric.DailyData {
			builder.WriteString(fmt.Sprintf(" %-10s", data.DayLabel))
		}
		builder.WriteString("\n     ")
		for _, data := range metric.DailyData {
			if data.AvgValue >= 0 {
				builder.WriteString(fmt.Sprintf(" %-10.1f", data.AvgValue))
			} else {
				builder.WriteString(fmt.Sprintf(" %-10s", "-"))
			}
		}
		builder.WriteString("\n\n")
	}

	return builder.String()
}

func (r *Report) GenerateMonthlyResourceContent() string {
	var builder strings.Builder

	periodNames := map[ReportPeriod]string{
		PeriodDaily:   "每日",
		PeriodWeekly:  "每周",
		PeriodMonthly: "每月",
		PeriodYearly:  "年度",
	}

	builder.WriteString(fmt.Sprintf(`
  📈 %s报表 - 月度资源指标趋势

  【报告周期】%s ~ %s
`, periodNames[r.Period], r.StartDate, r.EndDate))

	if len(r.MonthlyMetrics) == 0 {
		builder.WriteString("  （无月级别指标数据）\n")
		return builder.String()
	}

	for _, metric := range r.MonthlyMetrics {
		builder.WriteString(fmt.Sprintf("  🖥️ %s - %s (%s)\n", metric.TargetName, metric.MetricAlias, metric.Unit))
		builder.WriteString(fmt.Sprintf("     %-6s", "月份"))
		for _, data := range metric.MonthlyData {
			builder.WriteString(fmt.Sprintf(" %-8s", data.MonthLabel))
		}
		builder.WriteString("\n     ")
		for _, data := range metric.MonthlyData {
			if data.AvgValue >= 0 {
				builder.WriteString(fmt.Sprintf(" %-8.1f", data.AvgValue))
			} else {
				builder.WriteString(fmt.Sprintf(" %-8s", "-"))
			}
		}
		builder.WriteString("\n\n")
	}

	return builder.String()
}

func (r *Report) GenerateFullContent() string {
	var builder strings.Builder

	builder.WriteString(r.GenerateReportContent())

	if len(r.HourlyMetrics) > 0 {
		builder.WriteString(r.GenerateHourlyResourceContent())
	}

	if len(r.DailyMetrics) > 0 {
		builder.WriteString(r.GenerateDailyResourceContent())
	}

	if len(r.MonthlyMetrics) > 0 {
		builder.WriteString(r.GenerateMonthlyResourceContent())
	}

	return builder.String()
}

func (r *Report) generateSystemResourceSection() string {
	var builder strings.Builder

	builder.WriteString("🖥️ 本机资源监控\n\n")

	if len(r.SystemMetrics) == 0 {
		builder.WriteString("  （无系统资源监控数据）\n")
		return builder.String()
	}

	latest := r.SystemMetrics[len(r.SystemMetrics)-1]

	builder.WriteString(fmt.Sprintf("  CPU 使用率:     %.2f %s\n", latest.CPUPercent, "%"))
	builder.WriteString(fmt.Sprintf("  内存使用率:     %.2f %s (%s)\n",
		latest.MemoryPercent, "%",
		sysmon.FormatBytes(latest.MemoryUsed, latest.MemoryTotal)))
	builder.WriteString(fmt.Sprintf("  磁盘使用率:     %.2f %s (%s)\n",
		latest.DiskPercent, "%",
		sysmon.FormatBytes(latest.DiskUsed, latest.DiskTotal)))
	builder.WriteString(fmt.Sprintf("  网络下行速度:   %.2f KB/s\n", latest.NetDownKBps))
	builder.WriteString(fmt.Sprintf("  网络上行速度:   %.2f KB/s\n", latest.NetUpKBps))
	builder.WriteString(fmt.Sprintf("  磁盘读取速度:   %.2f KB/s\n", latest.DiskReadKBps))
	builder.WriteString(fmt.Sprintf("  磁盘写入速度:   %.2f KB/s\n", latest.DiskWriteKBps))
	builder.WriteString("\n")

	cpuData := extractSystemField(r.SystemMetrics, func(m sysmon.SystemMetric) float64 { return m.CPUPercent })
	memData := extractSystemField(r.SystemMetrics, func(m sysmon.SystemMetric) float64 { return m.MemoryPercent })
	diskData := extractSystemField(r.SystemMetrics, func(m sysmon.SystemMetric) float64 { return m.DiskPercent })
	netDownData := extractSystemField(r.SystemMetrics, func(m sysmon.SystemMetric) float64 { return m.NetDownKBps })
	netUpData := extractSystemField(r.SystemMetrics, func(m sysmon.SystemMetric) float64 { return m.NetUpKBps })

	cfg := config.GlobalConfig.SystemMon

	builder.WriteString("  📈 历史趋势 (ASCII 图表)\n\n")

	builder.WriteString("  【CPU 使用率趋势】\n")
	builder.WriteString(sysmon.GenerateASCIIChart(cpuData, 40, "%", cfg.CPUThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【内存使用率趋势】\n")
	builder.WriteString(sysmon.GenerateASCIIChart(memData, 40, "%", cfg.MemoryThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【磁盘使用率趋势】\n")
	builder.WriteString(sysmon.GenerateASCIIChart(diskData, 40, "%", cfg.DiskUsageThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【网络下行速度趋势】\n")
	builder.WriteString(sysmon.GenerateASCIIChart(netDownData, 40, "KB/s"))
	builder.WriteString("\n")

	builder.WriteString("  【网络上行速度趋势】\n")
	builder.WriteString(sysmon.GenerateASCIIChart(netUpData, 40, "KB/s"))
	builder.WriteString("\n")

	if len(r.SystemAlerts) > 0 {
		builder.WriteString("  🚨 当前系统告警\n\n")
		for _, a := range r.SystemAlerts {
			icon := "⚠️"
			if a.Level == "CRITICAL" {
				icon = "🚨"
			}
			builder.WriteString(fmt.Sprintf("  %s [%s] %s: %.2f %s (阈值: %.2f %s)\n",
				icon, a.Level, a.Metric, a.Value, a.Unit, a.Threshold, a.Unit))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

func extractSystemField(metrics []sysmon.SystemMetric, fn func(sysmon.SystemMetric) float64) []float64 {
	result := make([]float64, len(metrics))
	for i, m := range metrics {
		result[i] = fn(m)
	}
	return result
}

func formatDuration(durationMs int64) string {
	if durationMs < 1000 {
		return fmt.Sprintf("%dms", durationMs)
	} else if durationMs < 60000 {
		return fmt.Sprintf("%.2fs", float64(durationMs)/1000)
	} else {
		minutes := durationMs / 60000
		seconds := (durationMs % 60000) / 1000
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
}

func formatAvgDuration(stats []InterfaceStat) string {
	totalDuration := int64(0)
	count := 0
	for _, stat := range stats {
		totalDuration += stat.AvgDurationMS
		count++
	}
	if count == 0 {
		return "N/A"
	}
	avg := totalDuration / int64(count)
	return formatDuration(avg)
}

func GenerateReport(period ReportPeriod, startDate, endDate time.Time) *Report {
	return GenerateReportFromStorage(period, startDate, endDate)
}

func DisplayReport(report *Report) {
	fmt.Println(report.GenerateReportContent())

	if len(report.HourlyMetrics) > 0 {
		fmt.Println(report.GenerateHourlyResourceContent())
	}

	if len(report.DailyMetrics) > 0 {
		fmt.Println(report.GenerateDailyResourceContent())
	}

	if len(report.MonthlyMetrics) > 0 {
		fmt.Println(report.GenerateMonthlyResourceContent())
	}
}