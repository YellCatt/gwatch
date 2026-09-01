package sysmon

import (
	"fmt"
	"math"
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

// ChartAggregation 图表分桶聚合模式，决定一个图表列由多个原始数据点合并时如何取值。
type ChartAggregation int

const (
	// AggAvg 桶内取算术平均值，适用于平均值/瞬时值序列。
	AggAvg ChartAggregation = iota
	// AggMax 桶内取最大值，适用于峰值序列（CPU 最高使用率、网络峰值速率等）。
	AggMax
)

// GenerateASCIIChart 生成一个简单的 ASCII 柱状图（无时间标签版本）。
// 内部委托给 GenerateASCIIChartWithTimeEx 处理。
func GenerateASCIIChart(data []float64, width int, unit string, thresholds ...float64) string {
	return GenerateASCIIChartWithTimeEx(data, nil, AggAvg, width, unit, nil, thresholds...)
}

// GenerateASCIIChartWithTime 生成带时间标签的 ASCII 柱状图。
// 全部数据点均视为有效，桶内取平均值。
func GenerateASCIIChartWithTime(data []float64, width int, unit string, timeLabels []string, thresholds ...float64) string {
	return GenerateASCIIChartWithTimeEx(data, nil, AggAvg, width, unit, timeLabels, thresholds...)
}

// GenerateASCIIChartWithTimeEx 生成带时间标签的 ASCII 柱状图，支持数据有效性标记与聚合模式。
//
// 参数说明：
//   - data: 按时间升序排列的原始数值序列
//   - valid: 与 data 等长的有效性标记；为 nil 时表示全部有效。
//     为 false 的数据点（如没有采集记录的日期）会被直接忽略，
//     既不参与分桶聚合，也不参与最大值计算
//   - agg: 分桶聚合模式，AggAvg 取平均、AggMax 取最大
//   - width: 图表最大列数。有效点数超过 width 时按等宽分桶降采样；
//     否则每个有效点单独一列，不会为了凑宽度而复制数据
//   - timeLabels: 与 data 等长的时间标签
//   - thresholds: 阈值列表，第一个用于绘制阈值线与 ⚠️ 标记
//
// 图表不会为无效数据点伪造数值：无数据的位置直接不输出行，
// 保证"看到的柱子"都对应真实采集到的数据。
func GenerateASCIIChartWithTimeEx(data []float64, valid []bool, agg ChartAggregation, width int, unit string, timeLabels []string, thresholds ...float64) string {
	if len(data) == 0 {
		return "  (无数据)\n"
	}

	// 先筛出有效数据点，后续所有计算都只基于这些点进行。
	validIdx := make([]int, 0, len(data))
	for i := range data {
		if valid != nil && i < len(valid) && !valid[i] {
			continue
		}
		validIdx = append(validIdx, i)
	}
	if len(validIdx) == 0 {
		return "  (无有效数据)\n"
	}

	if width <= 0 {
		width = 20
	}
	buckets := len(validIdx)
	if buckets > width {
		buckets = width
	}

	type chartPoint struct {
		label string
		value float64
	}

	points := make([]chartPoint, 0, buckets)
	step := float64(len(validIdx)) / float64(buckets)
	for b := 0; b < buckets; b++ {
		start := int(float64(b) * step)
		end := int(float64(b+1) * step)
		if b == buckets-1 {
			end = len(validIdx)
		}
		if start >= end {
			continue
		}

		var value float64
		if agg == AggMax {
			value = math.Inf(-1)
			for k := start; k < end; k++ {
				if v := data[validIdx[k]]; v > value {
					value = v
				}
			}
		} else {
			sum := 0.0
			for k := start; k < end; k++ {
				sum += data[validIdx[k]]
			}
			value = sum / float64(end-start)
		}

		first := validIdx[start]
		points = append(points, chartPoint{label: chartLabelAt(timeLabels, first), value: value})
	}

	maxVal := 0.0
	for _, p := range points {
		if p.value > maxVal {
			maxVal = p.value
		}
	}
	for _, t := range thresholds {
		if t > maxVal {
			maxVal = t
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}

	barWidth := 20

	labelWidth := 0
	for _, p := range points {
		if len(p.label) > labelWidth {
			labelWidth = len(p.label)
		}
	}

	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("  图表 (有效样本: %d / %d, 时间点: %d)\n", len(validIdx), len(data), len(points)))
	builder.WriteString(fmt.Sprintf("  时间范围: %s → %s\n",
		chartLabelAt(timeLabels, validIdx[0]),
		chartLabelAt(timeLabels, validIdx[len(validIdx)-1])))

	if len(thresholds) > 0 {
		builder.WriteString(fmt.Sprintf("  阈值线: %.1f%s\n", thresholds[0], unit))
	}
	builder.WriteString("\n")

	var prevLabel string
	for _, p := range points {
		percent := p.value / maxVal
		if percent > 1.0 {
			percent = 1.0
		}
		if percent < 0 {
			percent = 0
		}

		filled := int(percent * float64(barWidth))
		barStr := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

		label := p.label
		if label == prevLabel {
			label = ""
		} else {
			prevLabel = label
		}

		thresholdMark := ""
		if len(thresholds) > 0 && p.value >= thresholds[0] {
			thresholdMark = " ⚠️"
		}

		var valueStr string
		if unit == "%" {
			valueStr = fmt.Sprintf("%6.2f%%", p.value)
		} else {
			valueStr = fmt.Sprintf("%8.2f %s", p.value, unit)
		}

		builder.WriteString(fmt.Sprintf("  %s %s %s%s\n", padChartLabel(label, labelWidth), barStr, valueStr, thresholdMark))
	}

	builder.WriteString("\n")

	return builder.String()
}

// chartLabelAt 取第 i 个数据点的时间标签，缺失时回退为序号，避免使用当前时间凭空构造标签。
func chartLabelAt(labels []string, i int) string {
	if i >= 0 && i < len(labels) && labels[i] != "" {
		return labels[i]
	}
	return fmt.Sprintf("#%d", i)
}

// padChartLabel 将标签右侧补齐空格到指定宽度，保证图表各列对齐。
func padChartLabel(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
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
	builder.WriteString(fmt.Sprintf("  CPU 使用率:     当前 %.2f%% | 历史最高 %.2f%%\n", latest.CPUPercent, latest.CPUMaxPercent))
	builder.WriteString(fmt.Sprintf("  内存使用率:     当前 %.2f%% | 历史最高 %.2f%% (%s / %s)\n",
		latest.MemoryPercent, latest.MemoryMaxPercent,
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

	builder.WriteString(fmt.Sprintf("  网络下行速度:   当前 %s | 历史最高 %s\n", util.FormatSpeed(latest.NetDownKBps), util.FormatSpeed(latest.NetDownMaxKBps)))
	builder.WriteString(fmt.Sprintf("  网络上行速度:   当前 %s | 历史最高 %s\n", util.FormatSpeed(latest.NetUpKBps), util.FormatSpeed(latest.NetUpMaxKBps)))
	builder.WriteString(fmt.Sprintf("  磁盘读取速度:   %s\n", util.FormatSpeed(latest.DiskReadKBps)))
	builder.WriteString(fmt.Sprintf("  磁盘写入速度:   %s\n", util.FormatSpeed(latest.DiskWriteKBps)))
	builder.WriteString("\n")

	builder.WriteString("  📈 历史趋势 (时间 + 进度 + 百分比)\n\n")

	cpuData := extractField(metrics, func(m SystemMetric) float64 { return m.CPUPercent })
	cpuMaxData := extractField(metrics, func(m SystemMetric) float64 { return m.CPUMaxPercent })
	memData := extractField(metrics, func(m SystemMetric) float64 { return m.MemoryPercent })
	memMaxData := extractField(metrics, func(m SystemMetric) float64 { return m.MemoryMaxPercent })
	diskData := extractField(metrics, func(m SystemMetric) float64 { return m.DiskPercent })
	netDownData := extractField(metrics, func(m SystemMetric) float64 { return m.NetDownKBps })
	netUpData := extractField(metrics, func(m SystemMetric) float64 { return m.NetUpKBps })

	cfg := config.GlobalConfig.SystemMon

	timeLabels := generateTimeLabels(metrics, 20)

	builder.WriteString("  【CPU 使用率 - 平均值趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTime(cpuData, 20, "%", timeLabels, cfg.CPUThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【CPU 使用率 - 最高值趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTimeEx(cpuMaxData, nil, AggMax, 20, "%", timeLabels, cfg.CPUThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【内存使用率 - 平均值趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTime(memData, 20, "%", timeLabels, cfg.MemoryThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【内存使用率 - 最高值趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTimeEx(memMaxData, nil, AggMax, 20, "%", timeLabels, cfg.MemoryThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【磁盘使用率趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTime(diskData, 20, "%", timeLabels, cfg.DiskUsageThreshold))
	builder.WriteString("\n")

	netDownMaxData := extractField(metrics, func(m SystemMetric) float64 { return m.NetDownMaxKBps })
	netUpMaxData := extractField(metrics, func(m SystemMetric) float64 { return m.NetUpMaxKBps })

	builder.WriteString("  【网络下行速度 - 平均值趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTime(netDownData, 20, "KB/s", timeLabels, cfg.NetworkDownThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【网络下行速度 - 最高值趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTimeEx(netDownMaxData, nil, AggMax, 20, "KB/s", timeLabels, cfg.NetworkDownThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【网络上行速度 - 平均值趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTime(netUpData, 20, "KB/s", timeLabels, cfg.NetworkUpThreshold))
	builder.WriteString("\n")

	builder.WriteString("  【网络上行速度 - 最高值趋势】\n")
	builder.WriteString(GenerateASCIIChartWithTimeEx(netUpMaxData, nil, AggMax, 20, "KB/s", timeLabels, cfg.NetworkUpThreshold))
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
			if len(a.TopProcesses) > 0 {
				builder.WriteString(fmt.Sprintf("    %s:\n", a.ProcessLabel))
				for _, p := range a.TopProcesses {
					switch a.Metric {
					case "内存使用率":
						builder.WriteString(fmt.Sprintf("      %d. %s (MEM:%.2f%%, CPU:%.2f%%, MEM:%s)\n",
							p.PID, p.Name, p.MemPercent, p.CPUPercent, util.FormatBytes(p.MemUsed)))
					case "网络下行速度", "网络上行速度":
						builder.WriteString(fmt.Sprintf("      %d. %s (NET↓:%s, NET↑:%s, CPU:%.2f%%, MEM:%.2f%%)\n",
							p.PID, p.Name, util.FormatSpeed(p.NetDownKBps), util.FormatSpeed(p.NetUpKBps), p.CPUPercent, p.MemPercent))
					default:
						builder.WriteString(fmt.Sprintf("      %d. %s (CPU:%.2f%%, MEM:%.2f%%, NET↓:%s, NET↑:%s)\n",
							p.PID, p.Name, p.CPUPercent, p.MemPercent, util.FormatSpeed(p.NetDownKBps), util.FormatSpeed(p.NetUpKBps)))
					}
				}
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
// 标签基于 metrics 自身的真实时间戳，在首尾时间之间均匀取 width 个点；
// 不再使用"当前时间往前推 24 小时"的硬编码区间，避免时间轴与实际数据不符。
func generateTimeLabels(metrics []SystemMetric, width int) []string {
	if len(metrics) == 0 || width <= 0 {
		return nil
	}

	start := metrics[0].Timestamp
	end := metrics[len(metrics)-1].Timestamp

	labels := make([]string, width)
	for i := 0; i < width; i++ {
		var ts time.Time
		if width == 1 || len(metrics) == 1 {
			ts = start
		} else {
			offset := time.Duration(float64(i) / float64(width-1) * float64(end.Sub(start)))
			ts = start.Add(offset)
		}
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