package sysmon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
	"gwatch/internal/util"
)

// GenerateASCIIChart 生成一个简单的 ASCII 柱状图（无时间标签版本）。
// 内部委托给 GenerateASCIIChartWithTime 处理。
func GenerateASCIIChart(data []float64, width int, unit string, thresholds ...float64) string {
	return GenerateASCIIChartWithTime(data, width, unit, nil, thresholds...)
}

// GenerateASCIIChartWithTime 生成带时间标签的 ASCII 柱状图。
// 将数据按宽度分桶求平均值，绘制为 █/░ 组合的柱状图，
// 并在超过阈值时添加 ⚠️ 标记。
func GenerateASCIIChartWithTime(data []float64, width int, unit string, timeLabels []string, thresholds ...float64) string {
	if len(data) == 0 {
		return "(无数据)"
	}

	var maxVal float64
	for _, v := range data {
		if v > maxVal {
			maxVal = v
		}
	}
	for _, t := range thresholds {
		if t > maxVal {
			maxVal = t
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	if width <= 0 {
		width = 20
	}

	bins := make([]float64, width)
	binStartIdx := make([]int, width)
	step := float64(len(data)) / float64(width)
	for i := 0; i < width; i++ {
		start := int(float64(i) * step)
		end := int(float64(i+1) * step)
		if end > len(data) {
			end = len(data)
		}
		if start >= len(data) {
			break
		}
		binStartIdx[i] = start
		sum := 0.0
		count := 0
		for j := start; j < end; j++ {
			sum += data[j]
			count++
		}
		if count > 0 {
			bins[i] = sum / float64(count)
		}
	}

	barWidth := 20

	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("  图表 (样本: %d, 时间点: %d)\n", len(data), width))

	var timeRange string
	if len(timeLabels) >= 2 {
		timeRange = fmt.Sprintf("  时间范围: %s → %s\n", timeLabels[0], timeLabels[len(timeLabels)-1])
	} else {
		now := timeutil.Now()
		timeRange = fmt.Sprintf("  时间范围: %s → %s\n",
			formatHourLabel(now.Add(-24*time.Hour)),
			formatHourLabel(now))
	}
	builder.WriteString(timeRange)

	if len(thresholds) > 0 {
		builder.WriteString(fmt.Sprintf("  阈值线: %.1f%s\n", thresholds[0], unit))
	}
	builder.WriteString("\n")

	for i, v := range bins {
		percent := v / maxVal
		if percent > 1.0 {
			percent = 1.0
		}
		if percent < 0 {
			percent = 0
		}

		filled := int(percent * float64(barWidth))
		empty := barWidth - filled

		barStr := strings.Repeat("█", filled) + strings.Repeat("░", empty)

		var timeLabel string
		idx := binStartIdx[i]
		if len(timeLabels) > idx && timeLabels[idx] != "" {
			timeLabel = timeLabels[idx]
		} else {
			now := timeutil.Now()
			ts := now.Add(-24*time.Hour + time.Duration(idx)*24*time.Hour/time.Duration(len(data)))
			timeLabel = formatHourLabel(ts)
		}

		thresholdMark := ""
		if len(thresholds) > 0 && v >= thresholds[0] {
			thresholdMark = " ⚠️"
		}

		var valueStr string
		if unit == "%" {
			valueStr = fmt.Sprintf("%6.2f%%", v)
		} else {
			valueStr = fmt.Sprintf("%8.2f %s", v, unit)
		}

		builder.WriteString(fmt.Sprintf("  %s %s %s%s\n", timeLabel, barStr, valueStr, thresholdMark))
	}

	builder.WriteString("\n")

	return builder.String()
}

// GenerateSystemReport 根据系统指标和告警列表生成完整的系统状态报告文本。
// 包含当前状态概览、CPU/内存/磁盘/网络的历史趋势 ASCII 图表以及告警摘要。
func GenerateSystemReport(metrics []SystemMetric, alerts []AlertItem) string {
	var builder strings.Builder

	builder.WriteString(`
系统资源监控报告
`)

	hostname, platform := GetHostInfo()
	builder.WriteString(fmt.Sprintf("  主机名:   %s\n", hostname))
	builder.WriteString(fmt.Sprintf("  系统:     %s\n", platform))
	builder.WriteString(fmt.Sprintf("  生成时间: %s\n", timeutil.FormatDateTime(timeutil.Now())))
	builder.WriteString("\n")

	if len(metrics) == 0 {
		builder.WriteString("  暂无采集数据\n")
		return builder.String()
	}

	latest := metrics[len(metrics)-1]

	builder.WriteString("  📊 当前状态概览\n\n")
	builder.WriteString(fmt.Sprintf("  CPU 使用率:     %.2f %s\n", latest.CPUPercent, "%"))
	builder.WriteString(fmt.Sprintf("  内存使用率:     %.2f %s (%s / %s)\n",
		latest.MemoryPercent, "%",
		util.FormatBytes(latest.MemoryUsed),
		util.FormatBytes(latest.MemoryTotal)))
	builder.WriteString(fmt.Sprintf("  磁盘使用率:     %.2f %s (%s / %s)\n",
		latest.DiskPercent, "%",
		util.FormatBytes(latest.DiskUsed),
		util.FormatBytes(latest.DiskTotal)))

	if len(latest.Partitions) > 0 {
		builder.WriteString("  各分区使用率:\n")
		for _, p := range latest.Partitions {
			builder.WriteString(fmt.Sprintf("    %s (%s): %.2f%% (%s / %s)\n",
				p.MountPoint, p.Fstype, p.Percent,
				util.FormatBytes(p.Used),
				util.FormatBytes(p.Total)))
		}
	}

	builder.WriteString(fmt.Sprintf("  网络下行速度:   %s\n", util.FormatSpeed(latest.NetDownKBps)))
	builder.WriteString(fmt.Sprintf("  网络上行速度:   %s\n", util.FormatSpeed(latest.NetUpKBps)))
	builder.WriteString(fmt.Sprintf("  磁盘读取速度:   %s\n", util.FormatSpeed(latest.DiskReadKBps)))
	builder.WriteString(fmt.Sprintf("  磁盘写入速度:   %s\n", util.FormatSpeed(latest.DiskWriteKBps)))
	builder.WriteString("\n")

	builder.WriteString("  📈 历史趋势 (时间 + 进度 + 百分比)\n\n")

	cpuData := extractField(metrics, func(m SystemMetric) float64 { return m.CPUPercent })
	memData := extractField(metrics, func(m SystemMetric) float64 { return m.MemoryPercent })
	diskData := extractField(metrics, func(m SystemMetric) float64 { return m.DiskPercent })
	netDownData := extractField(metrics, func(m SystemMetric) float64 { return m.NetDownKBps })
	netUpData := extractField(metrics, func(m SystemMetric) float64 { return m.NetUpKBps })

	cfg := config.GlobalConfig.SystemMon

	timeLabels := generateTimeLabels(metrics, 20)

	builder.WriteString("  【CPU 使用率趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTime(cpuData, 20, "%", timeLabels, cfg.CPUThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【内存使用率趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTime(memData, 20, "%", timeLabels, cfg.MemoryThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【磁盘使用率趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTime(diskData, 20, "%", timeLabels, cfg.DiskUsageThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【网络下行速度趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTime(netDownData, 20, "KB/s", timeLabels, cfg.NetworkDownThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【网络上行速度趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTime(netUpData, 20, "KB/s", timeLabels, cfg.NetworkUpThreshold))
	builder.WriteString("\n")

	if len(alerts) > 0 {
		builder.WriteString("  🚨 当前告警\n\n")
		for _, a := range alerts {
			if a.Unit == "KB/s" {
				builder.WriteString(fmt.Sprintf("  [%s] %s: %s (阈值: %s)\n",
					a.Level, a.Metric, util.FormatSpeed(a.Value), util.FormatSpeed(a.Threshold)))
			} else {
				builder.WriteString(fmt.Sprintf("  [%s] %s: %.2f %s (阈值: %.2f %s)\n",
					a.Level, a.Metric, a.Value, a.Unit, a.Threshold, a.Unit))
			}
		}
		builder.WriteString("\n")
	}

	builder.WriteString("  数据点数量: ")
	builder.WriteString(fmt.Sprintf("%d", len(metrics)))
	builder.WriteString("\n")

	return builder.String()
}

// generateTimeLabels 生成一组时间标签，用于图表的横轴。
// 从当前时间往前推 24 小时，均匀分为 width 个时间点。
func generateTimeLabels(metrics []SystemMetric, width int) []string {
	if len(metrics) == 0 {
		return nil
	}

	now := timeutil.Now()
	startTime := now.Add(-24 * time.Hour)

	labels := make([]string, width)
	for i := 0; i < width; i++ {
		offset := time.Duration(float64(i) / float64(width-1) * 24 * float64(time.Hour))
		if width == 1 {
			offset = 0
		}
		ts := startTime.Add(offset)
		labels[i] = ts.Format("01-02 15:04")
	}

	return labels
}

// formatHourLabel 将时间格式化为 "MM-DD HH:00" 形式的带日期小时标签。
func formatHourLabel(t time.Time) string {
	return t.Format("01-02 15:04")
}

// extractField 从系统指标列表中提取指定字段值，返回一个 float64 切片。
func extractField(metrics []SystemMetric, fn func(SystemMetric) float64) []float64 {
	result := make([]float64, len(metrics))
	for i, m := range metrics {
		result[i] = fn(m)
	}
	return result
}

// SaveSystemReport 生成系统报告并保存到 reports/system/ 目录下，返回生成的文件路径。
func SaveSystemReport(metrics []SystemMetric, alerts []AlertItem) (string, error) {
	reportDir := config.GlobalConfig.App.ReportDir
	if reportDir == "" {
		reportDir = "./reports"
	}

	sysReportDir := filepath.Join(reportDir, "system")
	if err := os.MkdirAll(sysReportDir, 0755); err != nil {
		logger.Error("创建系统报告目录失败", zap.Error(err))
		return "", err
	}

	content := GenerateSystemReport(metrics, alerts)
	timestamp := timeutil.FormatCompact(timeutil.Now())
	filename := fmt.Sprintf("system_monitor_%s.txt", timestamp)
	filePath := filepath.Join(sysReportDir, filename)

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		logger.Error("保存系统报告失败", zap.Error(err))
		return "", err
	}

	logger.Info("系统报告已保存", zap.String("文件", filePath))
	return filePath, nil
}