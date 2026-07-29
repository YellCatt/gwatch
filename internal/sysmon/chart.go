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

	height := 8
	if height > len(data) {
		height = len(data)
	}

	bins := make([]float64, width)
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

	var thresholdLine string
	if len(thresholds) > 0 {
		th := thresholds[0]
		thRow := int(float64(height) * th / maxVal)
		if thRow > height {
			thRow = height
		}
		if thRow > 0 && thRow <= height {
			thresholdLine = fmt.Sprintf("  -- 阈值 %.0f%s --\n", th, unit)
		}
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("  图表 (样本: %d, 宽度: %d)\n", len(data), width))
	builder.WriteString(thresholdLine)

	for row := height; row >= 0; row-- {
		thresholdMark := " "
		thLine := ""
		if len(thresholds) > 0 {
			th := thresholds[0]
			thRow := int(float64(height) * th / maxVal)
			if row == thRow {
				thresholdMark = "|"
				thLine = "--- 阈值线"
			}
		}

		var line strings.Builder
		label := fmt.Sprintf("%6.1f %s", maxVal*float64(row)/float64(height), unit)
		if row == 0 {
			label = fmt.Sprintf("%6.1f %s", 0.0, unit)
		}
		line.WriteString(fmt.Sprintf("%s %s", label, thresholdMark))
		for _, v := range bins {
			barHeight := int(float64(height) * v / maxVal)
			if row == barHeight {
				if len(thresholds) > 0 && v >= thresholds[0] {
					line.WriteString("█")
				} else {
					line.WriteString("█")
				}
			} else if row < barHeight {
				line.WriteString("█")
			} else {
				line.WriteString(" ")
			}
		}
		line.WriteString(thLine)
		builder.WriteString(line.String() + "\n")
	}

	var axisLine strings.Builder
	axisLine.WriteString("       +")
	for i := 0; i < width; i++ {
		axisLine.WriteString("-")
	}
	builder.WriteString(axisLine.String() + "\n")

	now := time.Now()
	timeLabels := fmt.Sprintf("       %s          %s",
		now.Add(-time.Duration(len(data)*config.GlobalConfig.SystemMon.Interval)*time.Second).Format("15:04"),
		now.Format("15:04"))
	builder.WriteString(timeLabels + "\n")

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
	builder.WriteString(fmt.Sprintf("  内存使用率:     %.2f %s (%.1f GB / %.1f GB)\n",
		latest.MemoryPercent, "%",
		float64(latest.MemoryUsed)/1024/1024/1024,
		float64(latest.MemoryTotal)/1024/1024/1024))
	builder.WriteString(fmt.Sprintf("  磁盘使用率:     %.2f %s (%.1f GB / %.1f GB)\n",
		latest.DiskPercent, "%",
		float64(latest.DiskUsed)/1024/1024/1024,
		float64(latest.DiskTotal)/1024/1024/1024))
	builder.WriteString(fmt.Sprintf("  网络下行速度:   %.2f KB/s\n", latest.NetDownKBps))
	builder.WriteString(fmt.Sprintf("  网络上行速度:   %.2f KB/s\n", latest.NetUpKBps))
	builder.WriteString(fmt.Sprintf("  磁盘读取速度:   %.2f KB/s\n", latest.DiskReadKBps))
	builder.WriteString(fmt.Sprintf("  磁盘写入速度:   %.2f KB/s\n", latest.DiskWriteKBps))
	builder.WriteString("\n")

	builder.WriteString("  📈 历史趋势 (ASCII 图表)\n\n")

	cpuData := extractField(metrics, func(m SystemMetric) float64 { return m.CPUPercent })
	memData := extractField(metrics, func(m SystemMetric) float64 { return m.MemoryPercent })
	diskData := extractField(metrics, func(m SystemMetric) float64 { return m.DiskPercent })
	netDownData := extractField(metrics, func(m SystemMetric) float64 { return m.NetDownKBps })
	netUpData := extractField(metrics, func(m SystemMetric) float64 { return m.NetUpKBps })

	cfg := config.GlobalConfig.SystemMon

	builder.WriteString("  【CPU 使用率趋势】\n")
	builder.WriteString(GenerateASCIIChart(cpuData, 40, "%", cfg.CPUThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【内存使用率趋势】\n")
	builder.WriteString(GenerateASCIIChart(memData, 40, "%", cfg.MemoryThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【磁盘使用率趋势】\n")
	builder.WriteString(GenerateASCIIChart(diskData, 40, "%", cfg.DiskUsageThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【网络下行速度趋势】\n")
	builder.WriteString(GenerateASCIIChart(netDownData, 40, "KB/s"))
	builder.WriteString("\n")

	builder.WriteString("  【网络上行速度趋势】\n")
	builder.WriteString(GenerateASCIIChart(netUpData, 40, "KB/s"))
	builder.WriteString("\n")

	if len(alerts) > 0 {
		builder.WriteString("  🚨 当前告警\n\n")
		for _, a := range alerts {
			builder.WriteString(fmt.Sprintf("  [%s] %s: %.2f %s (阈值: %.2f %s)\n",
				a.Level, a.Metric, a.Value, a.Unit, a.Threshold, a.Unit))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("  数据点数量: ")
	builder.WriteString(fmt.Sprintf("%d", len(metrics)))
	builder.WriteString("\n")

	return builder.String()
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