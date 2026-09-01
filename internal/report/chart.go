package report

import (
	"fmt"
	"strings"
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

		var valueStr string
		if unit == "%" {
			valueStr = fmt.Sprintf("%6.2f%%", v)
		} else {
			valueStr = fmt.Sprintf("%8.2f %s", v, unit)
		}

		builder.WriteString(fmt.Sprintf("  %s %s %s\n", label, barStr, valueStr))
	}

	if !anyValid {
		builder.WriteString("  (无有效数据)\n")
	}

	builder.WriteString("\n")
	return builder.String()
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
// 也会照常生成带标题的图表块，图表内部显示"无数据"占位，
// 保证年报里每个受监控的「目标×指标」都有对应位置。
func buildMonthlyChartData(r *Report) []string {
	charts := make([]string, 0, len(r.MonthlyMetrics))
	for _, m := range r.MonthlyMetrics {
		values := make([]float64, 0)
		labels := make([]string, 0)
		for _, d := range m.MonthlyData {
			if d.AvgValue < 0 {
				continue
			}
			values = append(values, d.AvgValue)
			labels = append(labels, d.MonthLabel)
		}

		header := fmt.Sprintf("  🖥️ %s - %s (%s)\n", m.TargetName, m.MetricAlias, m.Unit)
		chart := header + generateASCIIChart(values, labels, m.Unit, 20)
		charts = append(charts, chart)
	}
	return charts
}