package monitor

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/httpclient"
	"gwatch/internal/logger"
	"gwatch/internal/psv"
	"gwatch/internal/report"
	"gwatch/internal/scraper"
	"gwatch/internal/storage"
	"gwatch/internal/sysmon"
	"gwatch/internal/testcase"
	"gwatch/internal/timeutil"
)

const hotReloadInterval = 30 * time.Second

type MonitorTask struct {
	TestCase psv.TestCase
	Ticker   *time.Ticker
	StopChan chan struct{}
	Running  bool
}

type MonitorResult struct {
	TestCase      psv.TestCase
	Result        testcase.TestResult
	Timestamp     time.Time
	AlertType     string
	AlertMsg      string
	StatusCodeOk  bool
	AssertionOk   bool
	AssertionText string
}

var (
	tasks     = make(map[string]*MonitorTask)
	tasksMu   sync.Mutex
	results   = make([]MonitorResult, 0, 1000)
	resultsMu sync.Mutex
	taskChan  chan psv.TestCase
	stopChan  chan struct{}
)

// SetupMonitor 初始化监控任务但不等待信号，由外部统一管理生命周期。
// 返回 false 表示没有可监控的测试用例。
func SetupMonitor(testCases []psv.TestCase) bool {
	logger.Info("启动监控模式")

	testcase.ExecuteGlobalPreConditions(testCases)

	monitorCases := filterMonitorCases(testCases)
	if len(monitorCases) == 0 {
		logger.Info("未找到启用了监控的测试用例（monitor_enabled=true）")
		return false
	}

	maxWorkers := config.GlobalConfig.Monitor.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = len(monitorCases)
	} else if maxWorkers > len(monitorCases) {
		maxWorkers = len(monitorCases)
	}

	fmt.Printf("接口监控: %d 个任务, %d 个并发\n", len(monitorCases), maxWorkers)

	taskChan = make(chan psv.TestCase, len(monitorCases)*2)
	stopChan = make(chan struct{})

	for i := 0; i < maxWorkers; i++ {
		go worker(i)
	}

	for _, tc := range monitorCases {
		startTask(tc)
	}

	go generateAndSendStartupReport(monitorCases, maxWorkers)

	go startHotReload()

	return true
}

// worker 监控 worker 协程：从 taskChan 接收任务并执行，直到从 stopChan 收到停止信号。
func worker(id int) {
	logger.Info("Worker 协程已启动", zap.Int("id", id))
	for {
		select {
		case tc := <-taskChan:
			executeAndMonitorTask(tc)
		case <-stopChan:
			logger.Info("Worker 协程已停止", zap.Int("id", id))
			return
		}
	}
}

// executeAndMonitorTask 执行单个监控任务：调用 testcase 执行、告警检查、结果记录，
// 并异步持久化存储。失败或慢响应时按用例配置触发告警邮件。
func executeAndMonitorTask(tc psv.TestCase) {
	logger.Info("执行监控任务", zap.String("id", tc.ID))

	result := testcase.ExecuteTestCase(tc)

	monitorResult := MonitorResult{
		TestCase:  tc,
		Result:    result,
		Timestamp: timeutil.Now(),
	}

	checkAlerts(&monitorResult)

	resultsMu.Lock()
	results = append(results, monitorResult)
	if len(results) > 1000 {
		results = results[len(results)-1000:]
	}
	resultsMu.Unlock()

	go persistMonitorResult(tc, result, monitorResult)

	if monitorResult.AlertType != "" && (tc.AlertOnFailure || tc.AlertOnSlow) {
		sendAlertEmail(monitorResult)
	}
}

// persistMonitorResult 将单次监控结果持久化到 CSV，包括原始结果、
// 监控汇总与告警汇总三个维度的存储更新。
func persistMonitorResult(tc psv.TestCase, result testcase.TestResult, monitorResult MonitorResult) {
	errMsg := monitorResult.AlertMsg
	if errMsg == "" {
		errMsg = result.Error
	}

	record := storage.MonitorResultRecord{
		TestCaseID:     tc.ID,
		TestCaseDesc:   tc.Desc,
		URL:            result.ProcessedURL,
		Method:         tc.Method,
		ExpectedStatus: tc.ExpectedStatus,
		ActualStatus:   result.ActualStatus,
		ExpectedBody:   tc.ExpectedBody,
		ActualBody:     result.ResponseBody,
		ErrorMsg:       errMsg,
		DurationMS:     int64(result.Duration / time.Millisecond),
		Success:        result.Passed,
		AlertType:      monitorResult.AlertType,
		Timestamp:      timeutil.Now(),
	}

	if err := storage.RecordMonitorResult(record); err != nil {
		logger.Warn("监控结果记录到 CSV 失败", zap.Error(err))
	}

	if err := storage.UpdateMonitorSummary(record); err != nil {
		logger.Warn("更新监控汇总失败", zap.Error(err))
	}

	if err := storage.UpdateAlertSummary(record); err != nil {
		logger.Warn("更新告警汇总失败", zap.Error(err))
	}
}

// filterMonitorCases 从所有测试用例中筛选出启用了监控（MonitorEnabled=true）的用例。
func filterMonitorCases(testCases []psv.TestCase) []psv.TestCase {
	var result []psv.TestCase
	for _, tc := range testCases {
		if tc.MonitorEnabled {
			result = append(result, tc)
		}
	}
	return result
}

// StopAllTasks 停止所有监控任务：关闭各任务的 Ticker 与 StopChan，并关闭全局 stopChan，
// 释放 worker 协程池。
func StopAllTasks() {
	tasksMu.Lock()
	for id, task := range tasks {
		task.Ticker.Stop()
		close(task.StopChan)
		task.Running = false
		delete(tasks, id)
		logger.Info("已停止监控任务", zap.String("id", id))
	}
	tasksMu.Unlock()

	if stopChan != nil {
		close(stopChan)
	}

	taskChan = nil
	stopChan = nil
}

// GetResults 获取当前监控结果列表的一份副本（线程安全）。
func GetResults() []MonitorResult {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	return append([]MonitorResult{}, results...)
}

// GetTaskCount 获取当前正在运行的监控任务数量（线程安全）。
func GetTaskCount() int {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	return len(tasks)
}

// generateAndSendStartupReport 生成并发送启动报告（包含任务列表与最大并发数等信息）。
func generateAndSendStartupReport(cases []psv.TestCase, maxWorkers int) {
	info := buildStartupInfo(cases, maxWorkers)
	r := report.GenerateStartup(info)

	_, err := r.SaveReport()
	if err != nil {
		logger.Error("保存启动报告失败", zap.Error(err))
	}

	subject, body := r.PrepareReportEmail()
	err = email.SendCustomEmail(subject, body)
	if err != nil {
		logger.Warn("发送启动报告邮件失败", zap.Error(err))
	}
}

// buildStartupInfo 根据测试用例列表构建启动报告所需的 StartupInfo 数据结构。
func buildStartupInfo(cases []psv.TestCase, maxWorkers int) *report.StartupInfo {
	info := &report.StartupInfo{
		MaxWorkers: maxWorkers,
		Tasks:      make([]report.StartupTaskInfo, 0, len(cases)),
	}
	for _, tc := range cases {
		interval := tc.MonitorInterval
		if interval <= 0 {
			interval = config.GlobalConfig.Monitor.DefaultInterval
		}
		info.Tasks = append(info.Tasks, report.StartupTaskInfo{
			ID:       tc.ID,
			Desc:     tc.Desc,
			Method:   tc.Method,
			URL:      tc.URL,
			Interval: interval,
		})
	}
	return info
}

func StartMonitorMode(paths []string, tags []string) {
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
			scraper.StartLoop()
		}()
		started = append(started, "远程资源采集")
	}

	if len(paths) == 0 {
		paths = []string{config.GlobalConfig.App.CaseDir}
	}

	testCases, err := psv.ParseFiles(paths)
	if err != nil {
		logger.Warn("PSV 文件解析失败", zap.Error(err))
		errorMsg := fmt.Sprintf("解析测试用例文件失败: %v", err)
		if err := email.SendErrorReportEmail(errorMsg); err != nil {
			logger.Warn("发送错误报告邮件失败", zap.Error(err))
		}
		os.Exit(1)
	}

	testcase.SetAllTestCases(testCases)

	if len(tags) > 0 {
		testCases = testcase.FilterByTags(testCases, tags)
	}

	if config.GlobalConfig.Monitor.Enabled {
		if SetupMonitor(testCases) {
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
			report.NewReportScheduler(email.SendCustomEmail).Start()
		}()
		started = append(started, "定期报告")
	}

	PrintUnifiedBanner(started)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("按 Ctrl+C 停止所有监控...")
	<-sigChan

	fmt.Println("\n收到退出信号，正在停止所有监控系统...")
	email.CloseDispatcher()
	scraper.StopLoop()
	StopAllTasks()
	sysmon.StopSystemMonitor()
	fmt.Println("所有监控系统已停止")

	wg.Wait()
}

func PrintUnifiedBanner(started []string) {
	fmt.Printf("\n╔══════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                   gwatch 统一监控中心                     ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════╣\n")
	for i, name := range started {
		fmt.Printf("║  [%d] %-46s ║\n", i+1, name)
	}
	fmt.Printf("╚══════════════════════════════════════════════════════════╝\n\n")
}