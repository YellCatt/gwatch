package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/httpclient"
	"gwatch/internal/logger"
	"gwatch/internal/monitor"
	"gwatch/internal/psv"
	"gwatch/internal/report"
	"gwatch/internal/storage"
	"gwatch/internal/sysmon"
	"gwatch/internal/testcase"
)

// startMonitor 启动统一监控模式：API 接口监控 + 远程采集器 + 本机系统监控，
// 三大系统并行运行、统一等待信号、统一停止。
func startMonitor(paths []string) {
	httpclient.InitClient()

	if err := storage.InitDB(config.GlobalConfig.App.DataDir); err != nil {
		logger.Warn("CSV 存储初始化失败", zap.Error(err))
	} else {
		logger.Info("CSV 存储初始化成功")
	}

	var wg sync.WaitGroup
	started := make([]string, 0, 3)

	if config.GlobalConfig.SystemMon.Enabled {
		if sysmon.InitStorage() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sysmon.StartSystemMonitor()
			}()
			started = append(started, "本机系统监控")
		}
	}

	if config.GlobalConfig.Scraper.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ScraperLoop()
		}()
		started = append(started, "远程资源采集")
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

	if config.GlobalConfig.Monitor.Enabled {
		if monitor.SetupMonitor(testCases) {
			started = append(started, "API 接口监控")
		}
	}

	if config.GlobalConfig.Monitor.DailyReport ||
		config.GlobalConfig.Monitor.WeeklyReport ||
		config.GlobalConfig.Monitor.MonthlyReport ||
		config.GlobalConfig.Monitor.YearlyReport {
		wg.Add(1)
		go func() {
			defer wg.Done()
			report.NewReportScheduler().Start()
		}()
		started = append(started, "定期报告")
	}

	printUnifiedBanner(started)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("按 Ctrl+C 停止所有监控...")
	<-sigChan

	fmt.Println("\n收到退出信号，正在停止所有监控系统...")
	email.CloseDispatcher()
	StopScraper()
	monitor.StopAllTasks()
	sysmon.StopSystemMonitor()
	fmt.Println("所有监控系统已停止")

	wg.Wait()
}

// printUnifiedBanner 打印统一启动横幅，展示当前运行的所有监控系统。
func printUnifiedBanner(started []string) {
	fmt.Printf("\n╔══════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                   gwatch 统一监控中心                     ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════╣\n")
	for i, name := range started {
		fmt.Printf("║  [%d] %-46s ║\n", i+1, name)
	}
	fmt.Printf("╚══════════════════════════════════════════════════════════╝\n\n")
}
