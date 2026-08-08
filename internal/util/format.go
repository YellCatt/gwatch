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
