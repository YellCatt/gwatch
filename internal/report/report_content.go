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

// formatAvgDuration 计算接口统计列表的加权平均响应时间并格式化输出。
// 使用「累计总耗时 ÷ 总请求次数」计算，避免不同请求量的接口被等权平均。
func formatAvgDuration(stats []InterfaceStat) string {
	totalDuration := int64(0)
	totalCount := 0
	for _, stat := range stats {
		if stat.TotalCount > 0 {
			totalDuration += stat.TotalDurationMS
			totalCount += stat.TotalCount
		} else {
			totalDuration += stat.AvgDurationMS
			totalCount++
		}
	}
	if totalCount == 0 {
		return "N/A"
	}
	avg := totalDuration / int64(totalCount)
	return formatDuration(avg)
}

// boolToEnabled 将布尔值转换为带图标的启用/禁用状态文本。
func boolToEnabled(b bool) string {
	if b {
		return "✅ 已启用"
	}
	return "❌ 已禁用"
}
