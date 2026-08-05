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
	tasks         = make(map[string]*MonitorTask)
	tasksMu       sync.Mutex
	results       = make([]MonitorResult, 0, 1000)
	resultsMu     sync.Mutex
	taskChan      chan psv.TestCase
	stopChan      chan struct{}
	lastAlertTime = make(map[string]time.Time)
	lastAlertMu   sync.Mutex
)

func StartMonitor(testCases []psv.TestCase) {
	logger.Info("Starting monitor mode")

	if len(config.GlobalConfig.App.GlobalPre) > 0 {
		executeGlobalPreConditions(testCases)
	}

	monitorCases := filterMonitorCases(testCases)
	if len(monitorCases) == 0 {
		logger.Warn("No test cases with monitor_enabled=true found")
		return
	}

	maxWorkers := config.GlobalConfig.Monitor.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = len(monitorCases)
	} else if maxWorkers > len(monitorCases) {
		maxWorkers = len(monitorCases)
	}

	fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║              gwatch 接口监控模式                        ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n")
	fmt.Printf("监控任务数: %d\n", len(monitorCases))
	fmt.Printf("最大并发数: %d\n", maxWorkers)

	taskChan = make(chan psv.TestCase, len(monitorCases)*2)
	stopChan = make(chan struct{})

	for i := 0; i < maxWorkers; i++ {
		go worker(i)
	}

	for _, tc := range monitorCases {
		startTask(tc)
	}

	go generateAndSendStartupReport()

	if config.GlobalConfig.Monitor.DailyReport ||
		config.GlobalConfig.Monitor.WeeklyReport ||
		config.GlobalConfig.Monitor.MonthlyReport ||
		config.GlobalConfig.Monitor.YearlyReport {
		go scheduleReports()
	}

	go startHotReload()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n监控任务已启动，按 Ctrl+C 停止...")
	fmt.Printf("热加载已启用，每 %ds 扫描一次新测试用例\n", int(hotReloadInterval.Seconds()))

	<-sigChan

	fmt.Println("\n收到退出信号，正在停止监控任务...")
	StopAllTasks()
	fmt.Println("监控任务已全部停止")
}

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

func filterMonitorCases(testCases []psv.TestCase) []psv.TestCase {
	var result []psv.TestCase
	for _, tc := range testCases {
		if tc.MonitorEnabled {
			result = append(result, tc)
		}
	}
	return result
}

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

func GetResults() []MonitorResult {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	return append([]MonitorResult{}, results...)
}

func GetTaskCount() int {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	return len(tasks)
}

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

	err = r.SendReportEmail()
	if err != nil {
		logger.Error("Failed to send report email", zap.Error(err))
	}
}

func generateAndSendDailyReport(date time.Time) {
	generateAndSendReport(report.PeriodDaily, date)
}

func generateAndSendStartupReport() {
	r := report.GenerateStartup()

	_, err := r.SaveReport()
	if err != nil {
		logger.Error("Failed to save startup report", zap.Error(err))
		return
	}

	err = r.SendReportEmail()
	if err != nil {
		logger.Error("Failed to send startup report email", zap.Error(err))
	}
}
