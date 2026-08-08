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
	"gwatch/internal/logger"
	"gwatch/internal/psv"
	"gwatch/internal/report"
	"gwatch/internal/storage"
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
	TestCase  psv.TestCase
	Result    testcase.TestResult
	Timestamp time.Time
	AlertType string
	AlertMsg  string
}

var (
	tasks     = make(map[string]*MonitorTask)
	tasksMu   sync.Mutex
	results   = make([]MonitorResult, 0, 1000)
	resultsMu sync.Mutex
	taskChan  chan psv.TestCase
	stopChan  chan struct{}
)

// StartMonitor 启动监控模式：执行全局前置条件、过滤启用监控的测试用例、
// 启动 worker 协程池、为每个用例开启定时监控、启动报告调度与热加载，
// 并阻塞等待系统退出信号。
func StartMonitor(testCases []psv.TestCase) {
	if !SetupMonitor(testCases) {
		return
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n监控任务已启动，按 Ctrl+C 停止...")
	fmt.Printf("热加载已启用，每 %ds 扫描一次新测试用例\n", int(hotReloadInterval.Seconds()))

	<-sigChan

	fmt.Println("\n收到退出信号，正在停止监控任务...")
	StopAllTasks()
	fmt.Println("监控任务已全部停止")
}

// SetupMonitor 初始化监控任务但不等待信号，由外部统一管理生命周期。
// 返回 false 表示没有可监控的测试用例。
func SetupMonitor(testCases []psv.TestCase) bool {
	logger.Info("Starting monitor mode")

	if len(config.GlobalConfig.App.GlobalPre) > 0 {
		executeGlobalPreConditions(testCases)
	}

	monitorCases := filterMonitorCases(testCases)
	if len(monitorCases) == 0 {
		logger.Warn("No test cases with monitor_enabled=true found")
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

// executeGlobalPreConditions 按配置顺序执行全局前置条件测试用例，
// 若任一前置条件失败则发送错误邮件并终止进程。
func executeGlobalPreConditions(testCases []psv.TestCase) {
	fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ 执行全局前置条件                                       ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

	for _, preID := range config.GlobalConfig.App.GlobalPre {
		found := false
		for _, tc := range testCases {
			if tc.ID == preID {
				fmt.Printf("[全局前置] 执行: %s - %s\n", tc.ID, tc.Desc)
				result := testcase.ExecuteTestCase(tc)
				if !result.Passed {
					fmt.Printf("[全局前置] ❌ 失败: %s\n", result.Error)
					fmt.Printf("\n全局前置条件失败，终止监控启动\n")
					errorMsg := fmt.Sprintf("全局前置条件 '%s' 执行失败: %s", tc.ID, result.Error)
					if err := email.SendErrorReportEmail(errorMsg); err != nil {
						logger.Warn("Failed to send error report email", zap.Error(err))
					}
					os.Exit(1)
				}
				fmt.Printf("[全局前置] ✅ 成功\n")
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("[全局前置] ⚠️ 未找到测试用例: %s\n", preID)
		}
	}
	fmt.Println()
}

// worker 监控 worker 协程：从 taskChan 接收任务并执行，直到从 stopChan 收到停止信号。
func worker(id int) {
	logger.Info("Worker started", zap.Int("id", id))
	for {
		select {
		case tc := <-taskChan:
			executeAndMonitorTask(tc)
		case <-stopChan:
			logger.Info("Worker stopped", zap.Int("id", id))
			return
		}
	}
}

// executeAndMonitorTask 执行单个监控任务：调用 testcase 执行、告警检查、结果记录，
// 并异步持久化存储。失败或慢响应时按用例配置触发告警邮件。
func executeAndMonitorTask(tc psv.TestCase) {
	logger.Info("Executing monitor task", zap.String("id", tc.ID))

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
	record := storage.MonitorResultRecord{
		TestCaseID:     tc.ID,
		TestCaseDesc:   tc.Desc,
		URL:            result.ProcessedURL,
		Method:         tc.Method,
		ExpectedStatus: tc.ExpectedStatus,
		ActualStatus:   result.ActualStatus,
		ExpectedBody:   tc.ExpectedBody,
		ActualBody:     result.ResponseBody,
		ErrorMsg:       result.Error,
		DurationMS:     int64(result.Duration / time.Millisecond),
		Success:        result.Passed,
		AlertType:      monitorResult.AlertType,
		Timestamp:      timeutil.Now(),
	}

	if err := storage.RecordMonitorResult(record); err != nil {
		logger.Error("Failed to record monitor result to CSV", zap.Error(err))
	}

	if err := storage.UpdateMonitorSummary(record); err != nil {
		logger.Error("Failed to update monitor summary", zap.Error(err))
	}

	if err := storage.UpdateAlertSummary(record); err != nil {
		logger.Error("Failed to update alert summary", zap.Error(err))
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
		logger.Info("Stopped monitor task", zap.String("id", id))
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

// generateAndSendReport 根据指定周期与日期计算时间区间，生成报告并保存、发送邮件。
func generateAndSendReport(period report.ReportPeriod, date time.Time) {
	var startDate, endDate time.Time
	switch period {
	case report.PeriodDaily:
		startDate = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		endDate = startDate.Add(24 * time.Hour)
	case report.PeriodWeekly:
		weekday := date.Weekday()
		daysToMonday := int(weekday - time.Monday)
		if daysToMonday < 0 {
			daysToMonday += 7
		}
		startDate = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).AddDate(0, 0, -daysToMonday)
		endDate = startDate.AddDate(0, 0, 7)
	case report.PeriodMonthly:
		startDate = time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
		endDate = startDate.AddDate(0, 1, 0)
	case report.PeriodYearly:
		startDate = time.Date(date.Year(), 1, 1, 0, 0, 0, 0, date.Location())
		endDate = startDate.AddDate(1, 0, 0)
	default:
		logger.Error("Unknown report period", zap.String("period", string(period)))
		return
	}

	r := report.GenerateReportFromStorage(period, startDate, endDate)

	_, err := r.SaveReport()
	if err != nil {
		logger.Error("Failed to save report", zap.Error(err))
		return
	}

	subject, body := r.PrepareReportEmail()
	err = email.SendCustomEmail(subject, body)
	if err != nil {
		logger.Error("Failed to send report email", zap.Error(err))
	}
}

// generateAndSendDailyReport 便捷函数：生成并发送指定日期的每日报告。
func generateAndSendDailyReport(date time.Time) {
	generateAndSendReport(report.PeriodDaily, date)
}

// generateAndSendStartupReport 生成并发送启动报告（包含任务列表与最大并发数等信息）。
func generateAndSendStartupReport(cases []psv.TestCase, maxWorkers int) {
	info := buildStartupInfo(cases, maxWorkers)
	r := report.GenerateStartup(info)

	_, err := r.SaveReport()
	if err != nil {
		logger.Error("Failed to save startup report", zap.Error(err))
		return
	}

	subject, body := r.PrepareReportEmail()
	err = email.SendCustomEmail(subject, body)
	if err != nil {
		logger.Error("Failed to send startup report email", zap.Error(err))
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
