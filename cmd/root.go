package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
)

var (
	rootCmd = &cobra.Command{
		Use:   "gwatch [paths...]",
		Short: "gwatch - API Testing and Monitoring Tool",
		Long:  `A powerful enterprise-grade API testing and monitoring tool written in Go.`,
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if testFlag {
				runTests(args)
			} else {
				startMonitor(args)
			}
		},
	}

	tagsFlag string
	testFlag bool
)

// Execute 执行根命令，启动 Cobra 命令行程序。
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.Error("Failed to execute command", zap.Error(err))
		errorMsg := fmt.Sprintf("命令执行失败: %v", err)
		if email.Config.Enabled && email.Config.FromEmail != "" && len(email.Config.ToEmail) > 0 {
			if sendErr := email.SendErrorReportEmail(errorMsg); sendErr != nil {
				logger.Warn("Failed to send error report email", zap.Error(sendErr))
			}
		}
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.Flags().StringVar(&config.CfgFile, "config", "", "config file (default is ./config/config.yaml)")
	rootCmd.Flags().StringVarP(&tagsFlag, "tags", "T", "", "filter tests by tags (comma-separated)")
	rootCmd.Flags().BoolVarP(&testFlag, "test", "t", false, "run tests once (default is monitor mode)")

	initSystemReportCommand()
}
