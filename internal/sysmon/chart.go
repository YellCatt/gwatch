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
)

func GenerateASCIIChart(data []float64, width int, unit string, thresholds ...float64) string {
	return GenerateASCIIChartWithTime(data, width, unit, nil, thresholds...)
}

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
		timeRange = fmt.Sprintf("  时间范围: %s → %s (过去24小时)\n", timeLabels[0], timeLabels[len(timeLabels)-1])
	} else {
		now := time.Now()
		timeRange = fmt.Sprintf("  时间范围: %s → %s (过去24小时)\n",
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
		if len(timeLabels) > i && timeLabels[i] != "" {
			timeLabel = timeLabels[i]
		} else {
			now := time.Now()
			ts := now.Add(-24*time.Hour + time.Duration(binStartIdx[i])*24*time.Hour/time.Duration(len(data)))
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

func GenerateSystemReport(metrics []SystemMetric, alerts []AlertItem) string {
	var builder strings.Builder

	builder.WriteString(`
╔══════════════════════════════════════════════════════════╗
║           系统资源监控报告                              ║
╚══════════════════════════════════════════════════════════╝
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
		formatBytes(latest.MemoryUsed),
		formatBytes(latest.MemoryTotal)))
	builder.WriteString(fmt.Sprintf("  磁盘使用率:     %.2f %s (%s / %s)\n",
		latest.DiskPercent, "%",
		formatBytes(latest.DiskUsed),
		formatBytes(latest.DiskTotal)))
	builder.WriteString(fmt.Sprintf("  网络下行速度:   %s\n", formatSpeed(latest.NetDownKBps)))
	builder.WriteString(fmt.Sprintf("  网络上行速度:   %s\n", formatSpeed(latest.NetUpKBps)))
	builder.WriteString(fmt.Sprintf("  磁盘读取速度:   %s\n", formatSpeed(latest.DiskReadKBps)))
	builder.WriteString(fmt.Sprintf("  磁盘写入速度:   %s\n", formatSpeed(latest.DiskWriteKBps)))
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
					a.Level, a.Metric, formatSpeed(a.Value), formatSpeed(a.Threshold)))
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

func generateTimeLabels(metrics []SystemMetric, width int) []string {
	if len(metrics) == 0 {
		return nil
	}

	now := time.Now()
	startTime := now.Add(-24 * time.Hour)

	labels := make([]string, width)
	for i := 0; i < width; i++ {
		offset := time.Duration(float64(i) / float64(width-1) * 24 * float64(time.Hour))
		if width == 1 {
			offset = 0
		}
		ts := startTime.Add(offset)
		labels[i] = formatHourLabel(ts)
	}

	return labels
}

func formatHourLabel(t time.Time) string {
	return fmt.Sprintf("%02d:00", t.Hour())
}

func formatBytes(bytes uint64) string {
	mb := float64(bytes) / 1024 / 1024
	if mb >= 1024 {
		return fmt.Sprintf("%.2f GB", mb/1024)
	}
	return fmt.Sprintf("%.1f MB", mb)
}

func formatSpeed(kbps float64) string {
	if kbps >= 1024 {
		return fmt.Sprintf("%.2f MB/s", kbps/1024)
	}
	return fmt.Sprintf("%.2f KB/s", kbps)
}

func extractField(metrics []SystemMetric, fn func(SystemMetric) float64) []float64 {
	result := make([]float64, len(metrics))
	for i, m := range metrics {
		result[i] = fn(m)
	}
	return result
}

func SaveSystemReport(metrics []SystemMetric, alerts []AlertItem) (string, error) {
	reportDir := config.GlobalConfig.App.ReportDir
	if reportDir == "" {
		reportDir = "./reports"
	}

	sysReportDir := filepath.Join(reportDir, "system")
	if err := os.MkdirAll(sysReportDir, 0755); err != nil {
		logger.Error("Failed to create system report directory", zap.Error(err))
		return "", err
	}

	content := GenerateSystemReport(metrics, alerts)
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("system_monitor_%s.txt", timestamp)
	filePath := filepath.Join(sysReportDir, filename)

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		logger.Error("Failed to save system report", zap.Error(err))
		return "", err
	}

	logger.Info("System report saved", zap.String("file", filePath))
	return filePath, nil
}