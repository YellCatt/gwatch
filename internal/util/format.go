package util

import (
	"fmt"
	"os"

	"gwatch/config"
)

func FormatSpeed(kbps float64) string {
	if kbps >= 1024 {
		return fmt.Sprintf("%.2f MB/s", kbps/1024)
	}
	return fmt.Sprintf("%.2f KB/s", kbps)
}

// NormalizeSpeed 将阈值按单位换算为 KB/s。
// 支持的单位: KB/s, MB/s, GB/s, Kbps, Mbps, Gbps, b/s, Kb, Mb, Gb
// 如果单位无法识别，返回原始值，保持向后兼容。
func NormalizeSpeed(threshold float64, unit string) float64 {
	switch unit {
	case "MB/s", "MBPS", "mb/s", "mbps":
		return threshold * 1024
	case "GB/s", "GBPS", "gb/s", "gbps":
		return threshold * 1024 * 1024
	case "Mbps", "mbps":
		return threshold * 1024 / 8
	case "Kbps", "kbps":
		return threshold * 1024 / 8
	case "Gbps", "gbps":
		return threshold * 1024 * 1024 / 8
	case "KB/s", "KBPS", "kb/s", "kbps":
		return threshold
	case "B/s", "bps", "b/s":
		return threshold / 1024
	default:
		return threshold
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
