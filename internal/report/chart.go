package report

import (
	"fmt"
	"strings"
)

func generateASCIIChart(data []float64, labels []string, unit string, barWidth int) string {
	if len(data) == 0 {
		return "  (无数据)\n"
	}

	validData := make([]float64, 0, len(data))
	for _, v := range data {
		if v >= 0 {
			validData = append(validData, v)
		}
	}
	if len(validData) == 0 {
		return "  (无有效数据)\n"
	}

	var maxVal float64
	for _, v := range validData {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	if barWidth <= 0 {
		barWidth = 20
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("  图表 (数据点: %d)\n", len(data)))
	builder.WriteString("\n")

	for i, v := range data {
		if v < 0 {
			continue
		}

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

	builder.WriteString("\n")
	return builder.String()
}

func padRight(s string, length int) string {
	if len(s) >= length {
		return s
	}
	return s + strings.Repeat(" ", length-len(s))
}

func buildHourlyChartData(r *Report) []string {
	charts := make([]string, 0, len(r.HourlyMetrics))
	for _, m := range r.HourlyMetrics {
		values := make([]float64, 24)
		labels := make([]string, 24)
		for i, d := range m.HourlyData {
			values[i] = d.AvgValue
			labels[i] = fmt.Sprintf("%02d:00", i)
		}

		header := fmt.Sprintf("  🖥️ %s - %s (%s)\n", m.TargetName, m.MetricAlias, m.Unit)
		chart := header + generateASCIIChart(values, labels, m.Unit, 20)
		charts = append(charts, chart)
	}
	return charts
}

func buildDailyChartData(r *Report) []string {
	charts := make([]string, 0, len(r.DailyMetrics))
	for _, m := range r.DailyMetrics {
		values := make([]float64, len(m.DailyData))
		labels := make([]string, len(m.DailyData))
		for i, d := range m.DailyData {
			values[i] = d.AvgValue
			labels[i] = d.DayLabel
		}

		header := fmt.Sprintf("  🖥️ %s - %s (%s)\n", m.TargetName, m.MetricAlias, m.Unit)
		chart := header + generateASCIIChart(values, labels, m.Unit, 20)
		charts = append(charts, chart)
	}
	return charts
}

func buildMonthlyChartData(r *Report) []string {
	charts := make([]string, 0, len(r.MonthlyMetrics))
	for _, m := range r.MonthlyMetrics {
		values := make([]float64, len(m.MonthlyData))
		labels := make([]string, len(m.MonthlyData))
		for i, d := range m.MonthlyData {
			values[i] = d.AvgValue
			labels[i] = d.MonthLabel
		}

		header := fmt.Sprintf("  🖥️ %s - %s (%s)\n", m.TargetName, m.MetricAlias, m.Unit)
		chart := header + generateASCIIChart(values, labels, m.Unit, 20)
		charts = append(charts, chart)
	}
	return charts
}
