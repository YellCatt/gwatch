package cmd

import (
	"github.com/spf13/cobra"

	"gwatch/internal/sysmon"
)

// initSystemReportCommand 注册系统资源报告子命令，
// 用于在终端直接打印当前机器 CPU/内存/磁盘/网络等指标的 ASCII 图表报告。
func initSystemReportCommand() {
	sysMonReportCmd := &cobra.Command{
		Use:   "sys-report",
		Short: "生成系统资源监控报告",
		Long:  `生成当前系统资源使用情况的报告，包含CPU、内存、磁盘、网络等指标的ASCII图表。`,
		Run: func(cmd *cobra.Command, args []string) {
			sysmon.RunSystemReport()
		},
	}
	rootCmd.AddCommand(sysMonReportCmd)
}
