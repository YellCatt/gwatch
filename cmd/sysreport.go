package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/storage"
	"gwatch/internal/sysmon"
	"gwatch/internal/timeutil"
)

// initSystemReportCommand 注册系统报告子命令到根命令。
func initSystemReportCommand() {
	sysMonReportCmd := &cobra.Command{
		Use:   "sys-report",
		Short: "生成系统资源监控报告",
		Long:  `生成当前系统资源使用情况的报告，包含CPU、内存、磁盘、网络等指标的ASCII图表。`,
		Run: func(cmd *cobra.Command, args []string) {
			runSystemReport()
		},
	}
	rootCmd.AddCommand(sysMonReportCmd)
}

// runSystemReport 运行系统报告命令：加载最近的系统指标、检查告警、生成并输出报告。
func runSystemReport() {
	if err := storage.InitDB(config.GlobalConfig.App.DataDir); err != nil {
		logger.Warn("Storage init failed", zap.Error(err))
	}

	sysmon.InitStorage()
	sysmon.EnsureStorage()

	metrics, err := sysmon.LoadRecentMetrics(24)
	if err != nil || len(metrics) == 0 {
		metric, _ := sysmon.CollectMetrics()
		metrics = []sysmon.SystemMetric{metric}
		logger.Info("使用当前快照作为报告数据")
	}

	latest := metrics[len(metrics)-1]
	alerts := sysmon.CheckAlerts(latest)

	content := sysmon.GenerateSystemReport(metrics, alerts)
	fmt.Println(content)

	if config.GlobalConfig.SystemMon.ChartEnabled {
		path, err := sysmon.SaveSystemReport(metrics, alerts)
		if err != nil {
			logger.Error("Failed to save report", zap.Error(err))
		} else {
			fmt.Printf("\n报告已保存: %s\n", path)
		}
	}

	fmt.Printf("\n生成时间: %s\n", timeutil.FormatDateTime(time.Now()))
	os.Exit(0)
}
