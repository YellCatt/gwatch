package cmd

import (
	"github.com/spf13/cobra"

	"gwatch/internal/scraper"
)

var scraperCmd = &cobra.Command{
	Use:   "scraper",
	Short: "运行通用指标采集器",
	Long:  `从配置文件读取监控目标，使用 JSONPath 提取指标并输出`,
	Run: func(cmd *cobra.Command, args []string) {
		scraper.StartLoop()
	},
}

var probeCmd = &cobra.Command{
	Use:   "probe <url>",
	Short: "探测目标 URL 并打印 JSON 结构",
	Long:  `发送 HTTP 请求到目标 URL，解析返回的 JSON 并打印树形结构，方便获取 JSONPath`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		scraper.ProbeURL(args[0])
	},
}

func init() {
	rootCmd.AddCommand(scraperCmd)
	rootCmd.AddCommand(probeCmd)
}
