// Package timeutil 提供统一的中国标准时间（东八区）处理工具。
// 所有时间格式化均基于 Asia/Shanghai 时区，避免在不同部署环境下出现时区偏差。
package timeutil

import "time"

const (
	// LayoutDateTime 标准日期时间格式
	LayoutDateTime = "2006-01-02 15:04:05"
	// LayoutDateTimeMs 带毫秒的日期时间格式
	LayoutDateTimeMs = "2006-01-02 15:04:05.000"
	// LayoutCompact 紧凑格式，用于文件名（如报告文件名）
	LayoutCompact = "20060102_150405"
)

// shanghaiLoc 东八区时区对象，由 initShanghaiLocation 在包初始化时加载。
var shanghaiLoc = initShanghaiLocation()

// initShanghaiLocation 初始化东八区时区。
// 如果系统时区数据库加载失败（例如精简版容器），则回退到固定 UTC+8 偏移量。
func initShanghaiLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return loc
}

// Now 返回当前东八区时间。
func Now() time.Time {
	return time.Now().In(shanghaiLoc)
}

// Format 使用指定 Go 格式串格式化时间为东八区时间字符串。
func Format(t time.Time, layout string) string {
	return t.In(shanghaiLoc).Format(layout)
}

// FormatDateTime 格式化为标准日期时间（YYYY-MM-DD HH:MM:SS）。
func FormatDateTime(t time.Time) string {
	return Format(t, LayoutDateTime)
}

// FormatDateTimeMs 格式化为带毫秒的日期时间（YYYY-MM-DD HH:MM:SS.mmm）。
func FormatDateTimeMs(t time.Time) string {
	return Format(t, LayoutDateTimeMs)
}

// FormatCompact 格式化为紧凑格式（YYYYMMDD_HHMMSS），常用于文件名。
func FormatCompact(t time.Time) string {
	return Format(t, LayoutCompact)
}
