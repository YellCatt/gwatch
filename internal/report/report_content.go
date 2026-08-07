package report

import (
	"fmt"
)

// formatDuration 将毫秒时长格式化为易读形式（ms、s 或 m+s）。
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

// formatAvgDuration 计算接口统计列表的平均响应时间并格式化输出。
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

// boolToEnabled 将布尔值转换为带图标的启用/禁用状态文本。
func boolToEnabled(b bool) string {
	if b {
		return "✅ 已启用"
	}
	return "❌ 已禁用"
}

// formatBytes 将字节数格式化为 MB 或 GB 形式。
func formatBytes(bytes uint64) string {
	mb := float64(bytes) / 1024 / 1024
	if mb >= 1024 {
		return fmt.Sprintf("%.2f GB", mb/1024)
	}
	return fmt.Sprintf("%.1f MB", mb)
}

// DisplayReport 在控制台打印报告内容。
func DisplayReport(report *Report) {
	fmt.Println(report.GenerateContent())
}
