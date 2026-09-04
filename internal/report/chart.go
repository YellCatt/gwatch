package report

import (
	"fmt"
	"strings"

	"gwatch/internal/util"
)

// generateASCIIChart 根据数据数组和标签生成 ASCII 柱状图。
//
// 参数：
//   - data: 指标数值数组，负值表示无数据（哨兵值），会被跳过
//   - labels: 每个数据点的时间标签（如 "08-26"、"14:00"）
//   - unit: 单位字符串，"%" 时右对齐，其他单位左对齐
//   - barWidth: 柱状图的宽度（字符数），0 或负数时默认 20
//
// 柱状图使用 █（填充）和 ░（空白）字符，长度按数值在最大值中的占比计算。
// 当所有数据无效时输出 "(无有效数据)"。
func generateASCIIChart(data []float64, labels []string, unit string, barWidth int) string {
	if len(data) == 0 {
		return "  (无数据)\n"
	}

	if barWidth <= 0 {
		barWidth = 20
	}

	var maxVal float64
	for _, v := range data {
		if v < 0 {
			continue
		}
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("  图表 (数据点: %d)\n", len(data)))
	builder.WriteString("\n")

	anyValid := false
	for i, v := range data {
		if v < 0 {
			continue
		}
		anyValid = true

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

		var label string
		if i < len(labels) {
			label = labels[i]
		} else {
			label = fmt.Sprintf("%2d", i)
		}
		label = padRight(label, 6)

		builder.WriteString(fmt.Sprintf("  %s %s %s\n", label, barStr, formatChartValueASCII(v, unit)))
	}

	if !anyValid {
		builder.WriteString("  (无有效数据)\n")
	}

	builder.WriteString("\n")
	return builder.String()
}

// generateEmptyASCIIChart 在整组指标完全没有数据时生成占位图表。
//
// 按 labels 逐个槽位输出空柱子（数值 0），年度报告中即 12 个月各一行，
// 与日报按 24 小时逐行绘制的占位行为保持一致，
// 避免某个「目标×指标」的图表块因为无数据而在报告中消失。
func generateEmptyASCIIChart(labels []string, unit string, barWidth int) string {
	if len(labels) == 0 {
		return "  (无数据)\n"
	}

	if barWidth <= 0 {
		barWidth = 20
	}

	barStr := strings.Repeat("░", barWidth)

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("  图表 (有效数据点: 0 / %d)\n", len(labels)))
	builder.WriteString("  ⚠️ 周期内无有效采集数据，以下为按时间槽位绘制的占位行\n")
	builder.WriteString("\n")

	for _, label := range labels {
		builder.WriteString(fmt.Sprintf("  %s %s %s\n",
			padRight(label, 6), barStr, formatChartValueASCII(0, unit)))
	}

	builder.WriteString("\n")
	return builder.String()
}

// formatChartValueASCII 按单位格式化资源指标图表右侧的数值文本。
// 百分比单位右对齐到 6 位并附加 %；
// 速度类单位（网速、磁盘 IO 等）超过 1024 时自动进位（KB/s → MB/s → GB/s），右对齐到 12 位；
// 其他单位右对齐到 8 位并附加单位后缀。
func formatChartValueASCII(value float64, unit string) string {
	if unit == "%" {
		return fmt.Sprintf("%6.2f%%", value)
	}
	if util.IsSpeedUnit(unit) {
		return fmt.Sprintf("%12s", util.FormatSpeed(value))
	}
	return fmt.Sprintf("%8.2f %s", value, unit)
}

// padRight 将字符串右填充空格到指定长度，用于对齐柱状图标签。
// 若源字符串长度已达到或超过目标长度，原样返回。
func padRight(s string, length int) string {
	if len(s) >= length {
		return s
	}
	return s + strings.Repeat(" ", length-len(s))
}

// buildHourlyChartData 从 Report 构建每小时资源指标的 ASCII 图表列表。
// 遍历所有采集器目标的小时级指标，过滤掉哨兵值（-1），以 20 列宽度生成柱状图。
// 每个图表以目标名称和指标别名作为标题。
func buildHourlyChartData(r *Report) []string {
	charts := make([]string, 0, len(r.HourlyMetrics))
	for _, m := range r.HourlyMetrics {
		values := make([]float64, 0)
		labels := make([]string, 0)
		for _, d := range m.HourlyData {
			if d.AvgValue < 0 {
				continue
			}
			values = append(values, d.AvgValue)
			labels = append(labels, fmt.Sprintf("%02d:00", d.Hour))
		}

		if len(values) == 0 {
			continue
		}

		header := fmt.Sprintf("  🖥️ %s - %s (%s)\n", m.TargetName, m.MetricAlias, m.Unit)
		chart := header + generateASCIIChart(values, labels, m.Unit, 20)
		charts = append(charts, chart)
	}
	return charts
}

// buildDailyChartData 从 Report 构建每日资源指标的 ASCII 图表列表。
// 遍历所有采集器目标的日级指标，过滤掉哨兵值（-1），以 20 列宽度生成柱状图。
// 每个图表以目标名称和指标别名作为标题。
func buildDailyChartData(r *Report) []string {
	charts := make([]string, 0, len(r.DailyMetrics))
	for _, m := range r.DailyMetrics {
		values := make([]float64, 0)
		labels := make([]string, 0)
		for _, d := range m.DailyData {
			if d.AvgValue < 0 {
				continue
			}
			values = append(values, d.AvgValue)
			labels = append(labels, d.DayLabel)
		}

		if len(values) == 0 {
			continue
		}

		header := fmt.Sprintf("  🖥️ %s - %s (%s)\n", m.TargetName, m.MetricAlias, m.Unit)
		chart := header + generateASCIIChart(values, labels, m.Unit, 20)
		charts = append(charts, chart)
	}
	return charts
}

// buildMonthlyChartData 从 Report 构建每月资源指标的 ASCII 图表列表。
// 遍历所有采集器目标的月级指标，过滤掉哨兵值（-1），以 20 列宽度生成柱状图。
// 每个图表以目标名称和指标别名作为标题。
//
// 即使某组指标 12 个月全为哨兵值（年度报告统计的年度没有任何采集数据），
// 也会照常生成带标题的图表块，并按 12 个月槽位逐行绘制空占位（数值 0），
// 与日报按 24 小时逐行绘制的占位行为一致，
// 保证年报里每个受监控的「目标×指标」都有对应位置。
func buildMonthlyChartData(r *Report) []string {
	charts := make([]string, 0, len(r.MonthlyMetrics))
	for _, m := range r.MonthlyMetrics {
		values := make([]float64, 0)
		labels := make([]string, 0)
		monthLabels := make([]string, 0, len(m.MonthlyData))
		for _, d := range m.MonthlyData {
			monthLabels = append(monthLabels, d.MonthLabel)
			if d.AvgValue < 0 {
				continue
			}
			values = append(values, d.AvgValue)
			labels = append(labels, d.MonthLabel)
		}

		header := fmt.Sprintf("  🖥️ %s - %s (%s)\n", m.TargetName, m.MetricAlias, m.Unit)

		var chart string
		if len(values) == 0 {
			// 整年 12 个月全是哨兵值：按月份槽位逐行绘制空占位，
			// 与日报 24 小时逐行绘制的行为保持一致。
			chart = header + generateEmptyASCIIChart(monthLabels, m.Unit, 20)
		} else {
			chart = header + generateASCIIChart(values, labels, m.Unit, 20)
		}
		charts = append(charts, chart)
	}
	return charts
}