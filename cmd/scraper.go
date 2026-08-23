// Package cmd 中的 scraper 子命令：独立运行采集器，或用于调试 JSONPath 的 URL 探测工具。
package cmd

import (
	"github.com/spf13/cobra"

	"gwatch/internal/scraper"
)

// scraperCmd 运行通用指标采集器：按配置文件中的目标列表周期采集，
// 使用 JSONPath 提取指标并根据阈值触发告警。
var scraperCmd = &cobra.Command{
	Use:   "scraper",
	Short: "运行通用指标采集器",
	Long:  `从配置文件读取监控目标，使用 JSONPath 提取指标并输出`,
	Run: func(cmd *cobra.Command, args []string) {
		scraper.StartLoop()
	},
}

// probeCmd 调试工具：对单个 URL 发起请求并打印 JSON 树形结构，
// 便于开发者快速确定正确的 JSONPath 表达式。
var probeCmd = &cobra.Command{
	Use:   "probe <url>",
	Short: "探测目标 URL 并打印 JSON 结构",
	Long:  `发送 HTTP 请求到目标 URL，解析返回的 JSON 并打印树形结构，方便获取 JSONPath`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		scraper.ProbeURL(args[0])
	},
}

// init 注册采集器与 URL 探测子命令到根命令。
func init() {
	rootCmd.AddCommand(scraperCmd)
	rootCmd.AddCommand(probeCmd)
}
