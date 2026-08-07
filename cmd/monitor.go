package cmd

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/httpclient"
	"gwatch/internal/logger"
	"gwatch/internal/monitor"
	"gwatch/internal/psv"
	"gwatch/internal/storage"
	"gwatch/internal/sysmon"
	"gwatch/internal/testcase"
)

// startMonitor 启动监控模式：初始化存储、系统监控、解析 PSV 文件、过滤标签并启动监控。
func startMonitor(paths []string) {
	httpclient.InitClient()

	if err := storage.InitDB(config.GlobalConfig.App.DataDir); err != nil {
		logger.Warn("CSV 存储初始化失败", zap.Error(err))
	} else {
		logger.Info("CSV 存储初始化成功")
	}

	if config.GlobalConfig.SystemMon.Enabled {
		sysmon.InitStorage()
		go sysmon.StartSystemMonitor()
	}

	if len(paths) == 0 {
		paths = []string{config.GlobalConfig.App.CaseDir}
	}

	testCases, err := psv.ParseFiles(paths)
	if err != nil {
		logger.Error("Failed to parse PSV files", zap.Error(err))
		errorMsg := fmt.Sprintf("解析测试用例文件失败: %v", err)
		if err := email.SendErrorReportEmail(errorMsg); err != nil {
			logger.Warn("Failed to send error report email", zap.Error(err))
		}
		os.Exit(1)
	}

	testcase.SetAllTestCases(testCases)

	var tags []string
	if tagsFlag != "" {
		tags = strings.Split(tagsFlag, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
		testCases = testcase.FilterByTags(testCases, tags)
	}

	monitor.StartMonitor(testCases)
}
