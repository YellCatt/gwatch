// Package cmd 封装 gwatch 的命令行入口，基于 cobra 实现。
// 提供根命令、测试模式、监控模式、采集器与系统报告等子命令。
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/bootstrap"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/monitor"
	"gwatch/internal/testcase"
)

// rootCmd 根命令对象；当未启用 --test 时默认进入监控模式。
var rootCmd = &cobra.Command{
	Use:   "gwatch [paths...]",
	Short: "gwatch - API Testing and Monitoring Tool",
	Long:  `A powerful enterprise-grade API testing and monitoring tool written in Go.`,
	Args:  cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// 根据 --test 标志选择一次性测试模式或常驻监控模式
		if testFlag {
			var tags []string
			if tagsFlag != "" {
				for _, tag := range strings.Split(tagsFlag, ",") {
					tags = append(tags, strings.TrimSpace(tag))
				}
			}
			testcase.RunTests(args, tags)
		} else {
			var tags []string
			if tagsFlag != "" {
				for _, tag := range strings.Split(tagsFlag, ",") {
					tags = append(tags, strings.TrimSpace(tag))
				}
			}
			monitor.StartMonitorMode(args, tags)
		}
	},
}

// tagsFlag 命令行 --tags / -T 参数，用于按标签过滤测试用例。
// 多个标签之间用英文逗号分隔。
var tagsFlag string

// testFlag 命令行 --test / -t 参数；为 true 时进入一次性测试模式，
// 执行完所有用例后立即退出。
var testFlag bool

// Execute 执行根命令；若发生错误则记录日志、发送错误报告邮件并以非零码退出。
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.Error("命令执行失败", zap.Error(err))
		errorMsg := fmt.Sprintf("命令执行失败: %v", err)
		if email.Config.Enabled && email.Config.FromEmail != "" && len(email.Config.ToEmail) > 0 {
			if sendErr := email.SendErrorReportEmail(errorMsg); sendErr != nil {
				logger.Warn("发送错误报告邮件失败", zap.Error(sendErr))
			}
		}
		os.Exit(1)
	}
}

// init 注册 cobra 初始化回调以及全局参数。
func init() {
	// 在 cobra 执行前完成应用初始化（目录、配置、日志、邮件等）
	cobra.OnInitialize(bootstrap.InitApp)
	rootCmd.Flags().StringVar(&config.CfgFile, "config", "", "config file (default is ./config/config.yaml)")
	rootCmd.Flags().StringVarP(&tagsFlag, "tags", "T", "", "filter tests by tags (comma-separated)")
	rootCmd.Flags().BoolVarP(&testFlag, "test", "t", false, "run tests once (default is monitor mode)")

	initSystemReportCommand()
}
