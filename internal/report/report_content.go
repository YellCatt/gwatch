package report

import (
	"fmt"
)

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

func boolToEnabled(b bool) string {
	if b {
		return "✅ 已启用"
	}
	return "❌ 已禁用"
}

func formatBytes(bytes uint64) string {
	mb := float64(bytes) / 1024 / 1024
	if mb >= 1024 {
		return fmt.Sprintf("%.2f GB", mb/1024)
	}
	return fmt.Sprintf("%.1f MB", mb)
}

func DisplayReport(report *Report) {
	fmt.Println(report.GenerateContent())
}