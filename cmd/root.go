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

var (
	rootCmd = &cobra.Command{
		Use:   "gwatch [paths...]",
		Short: "gwatch - API Testing and Monitoring Tool",
		Long:  `A powerful enterprise-grade API testing and monitoring tool written in Go.`,
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
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

	tagsFlag string
	testFlag bool
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.Warn("Failed to execute command", zap.Error(err))
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
	cobra.OnInitialize(bootstrap.InitApp)
	rootCmd.Flags().StringVar(&config.CfgFile, "config", "", "config file (default is ./config/config.yaml)")
	rootCmd.Flags().StringVarP(&tagsFlag, "tags", "T", "", "filter tests by tags (comma-separated)")
	rootCmd.Flags().BoolVarP(&testFlag, "test", "t", false, "run tests once (default is monitor mode)")

	initSystemReportCommand()
}
