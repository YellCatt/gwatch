package util

import (
	"fmt"
	"os"

	"gwatch/config"
)

func FormatSpeed(kbps float64) string {
	if kbps >= 1024*1024 {
		return fmt.Sprintf("%.2f GB/s", kbps/1024/1024)
	} else if kbps >= 1024 {
		return fmt.Sprintf("%.2f MB/s", kbps/1024)
	}
	return fmt.Sprintf("%.2f KB/s", kbps)
}

// NormalizeSpeed 将阈值从用户配置的单位换算为 KB/s。
// 采集值本身已经是 KB/s，无需转换，仅阈值需要按配置单位换算。
// 支持的单位: KB/s, MB/s, GB/s, Kbps, Mbps, Gbps, b/s
// 如果单位无法识别，返回原始值，保持向后兼容。
func NormalizeSpeed(threshold float64, unit string) float64 {
	switch unit {
	case "MB/s", "MBPS", "mb/s":
		return threshold * 1024
	case "GB/s", "GBPS", "gb/s":
		return threshold * 1024 * 1024
	case "Mbps", "mbps":
		return threshold * 1024 / 8
	case "Kbps", "kbps":
		return threshold * 1024 / 8
	case "Gbps", "gbps":
		return threshold * 1024 * 1024 / 8
	case "KB/s", "KBPS", "kb/s":
		return threshold
	case "B/s", "bps", "b/s":
		return threshold / 1024
	default:
		return threshold
	}
}

// IsSpeedUnit 判断给定单位是否为速度类单位（需要归一化到 KB/s）。
func IsSpeedUnit(unit string) bool {
	switch unit {
	case "MB/s", "MBPS", "mb/s",
		"GB/s", "GBPS", "gb/s",
		"Mbps", "mbps",
		"Kbps", "kbps",
		"Gbps", "gbps",
		"KB/s", "KBPS", "kb/s",
		"B/s", "bps", "b/s":
		return true
	default:
		return false
	}
}

func FormatBytes(bytes uint64) string {
	mb := float64(bytes) / 1024 / 1024
	if mb >= 1024 {
		return fmt.Sprintf("%.2f GB", mb/1024)
	}
	return fmt.Sprintf("%.1f MB", mb)
}

func GetDeviceName() string {
	if config.GlobalConfig.App.HostName != "" {
		return config.GlobalConfig.App.HostName
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "未知设备"
	}
	return hostname
}
