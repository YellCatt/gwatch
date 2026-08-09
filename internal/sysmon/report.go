package sysmon

import (
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/storage"
	"gwatch/internal/timeutil"
)

func RunSystemReport() {
	if err := storage.InitDB(config.GlobalConfig.App.DataDir); err != nil {
		logger.Warn("Storage init failed", zap.Error(err))
	}

	InitStorage()
	EnsureStorage()

	metrics, err := LoadRecentMetrics(24)
	if err != nil || len(metrics) == 0 {
		metric, _ := CollectMetrics()
		metrics = []SystemMetric{metric}
		logger.Info("使用当前快照作为报告数据")
	}

	latest := metrics[len(metrics)-1]
	alerts := CheckAlerts(latest)

	content := GenerateSystemReport(metrics, alerts)
	fmt.Println(content)

	if config.GlobalConfig.SystemMon.ChartEnabled {
		path, err := SaveSystemReport(metrics, alerts)
		if err != nil {
			logger.Error("Failed to save report", zap.Error(err))
		} else {
			fmt.Printf("\n报告已保存: %s\n", path)
		}
	}

	fmt.Printf("\n生成时间: %s\n", timeutil.FormatDateTime(time.Now()))
	os.Exit(0)
}
