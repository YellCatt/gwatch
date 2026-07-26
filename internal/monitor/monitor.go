package monitor

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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

// 热加载配置
const (
	hotReloadInterval = 30 * time.Second // 热加载扫描间隔
)

// MonitorTask 表示一个监控任务
type MonitorTask struct {
	TestCase psv.TestCase
	Ticker   *time.Ticker
	StopChan chan struct{}
	Running  bool
}

// MonitorResult 表示监控结果
type MonitorResult struct {
	TestCase  psv.TestCase
	Result    testcase.TestResult
	Timestamp time.Time
	AlertType string // "failure", "timeout", "sla", ""
	AlertMsg  string
}

var (
	tasks         = make(map[string]*MonitorTask)
	tasksMu       sync.Mutex
	results       = make([]MonitorResult, 0, 1000)
	resultsMu     sync.Mutex
	taskChan      chan psv.TestCase
	stopChan      chan struct{}
	lastAlertTime = make(map[string]time.Time) // 存储每个测试用例上次告警时间
	lastAlertMu   sync.Mutex                   // 保护 lastAlertTime 的互斥锁
)

// StartMonitor 启动监控模式
func StartMonitor(testCases []psv.TestCase) {
	logger.Info("Starting monitor mode")

	// 过滤出启用监控的测试用例
	monitorCases := filterMonitorCases(testCases)
	if len(monitorCases) == 0 {
		logger.Warn("No test cases with monitor_enabled=true found")
		return
	}

	// 获取最大并发数配置
	maxWorkers := config.GlobalConfig.Monitor.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = len(monitorCases) // 不限制，每个任务一个goroutine
	} else if maxWorkers > len(monitorCases) {
		maxWorkers = len(monitorCases) // 不能超过任务数
	}

	fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║              gwatch 接口监控模式                        ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n")
	fmt.Printf("监控任务数: %d\n", len(monitorCases))
	fmt.Printf("最大并发数: %d\n", maxWorkers)

	// 初始化任务通道和停止通道
	taskChan = make(chan psv.TestCase, len(monitorCases)*2) // 预留空间
	stopChan = make(chan struct{})

	// 启动 worker pool
	for i := 0; i < maxWorkers; i++ {
		go worker(i)
	}

	// 注册所有任务
	for _, tc := range monitorCases {
		startTask(tc)
	}

	// 发送启动通知邮件
	go sendStartupNotification(len(monitorCases))

	// 启动每日报告调度（如果启用）
	if config.GlobalConfig.Monitor.DailyReport {
		go scheduleDailyReport()
	}

	// 启动热加载协程
	go startHotReload()

	// 设置信号处理，优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n监控任务已启动，按 Ctrl+C 停止...")
	fmt.Printf("热加载已启用，每 %ds 扫描一次新测试用例\n", int(hotReloadInterval.Seconds()))

	// 阻塞等待信号
	<-sigChan

	fmt.Println("\n收到退出信号，正在停止监控任务...")
	StopAllTasks()
	fmt.Println("监控任务已全部停止")
}

// worker 执行任务的工作协程
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

// executeAndMonitorTask 执行单个监控任务
func executeAndMonitorTask(tc psv.TestCase) {
	logger.Info("Executing monitor task", zap.String("id", tc.ID))

	result := testcase.ExecuteTestCase(tc)

	// 记录监控结果
	monitorResult := MonitorResult{
		TestCase:  tc,
		Result:    result,
		Timestamp: timeutil.Now(),
	}

	// 检查是否需要告警
	checkAlerts(&monitorResult)

	// 保存结果到内存
	resultsMu.Lock()
	results = append(results, monitorResult)
	if len(results) > 1000 {
		results = results[len(results)-1000:]
	}
	resultsMu.Unlock()

	// 保存结果到CSV文件（持久化）
	go func() {
		err := storage.RecordMonitorResult(storage.MonitorResultRecord{
			TestCaseID:     tc.ID,
			TestCaseDesc:   tc.Desc,
			URL:            tc.URL,
			Method:         tc.Method,
			ExpectedStatus: tc.ExpectedStatus,
			ActualStatus:   result.ActualStatus,
			ExpectedBody:   tc.ExpectedBody,
			ActualBody:     result.ResponseBody,
			ErrorMsg:       result.Error,
			DurationMS:     int64(result.Duration / time.Millisecond),
			Success:        result.Passed,
			Timestamp:      timeutil.Now(),
		})
		if err != nil {
			logger.Error("Failed to record monitor result to CSV", zap.Error(err))
		}
	}()

	// 如果有告警，发送邮件
	if monitorResult.AlertType != "" && (tc.AlertOnFailure || tc.AlertOnSlow) {
		sendAlertEmail(monitorResult)
	}
}

// filterMonitorCases 过滤出启用监控的测试用例
func filterMonitorCases(testCases []psv.TestCase) []psv.TestCase {
	var result []psv.TestCase
	for _, tc := range testCases {
		if tc.MonitorEnabled {
			result = append(result, tc)
		}
	}
	return result
}

// startTask 启动单个监控任务
func startTask(tc psv.TestCase) {
	tasksMu.Lock()
	defer tasksMu.Unlock()

	if _, exists := tasks[tc.ID]; exists {
		logger.Warn("Task already exists", zap.String("id", tc.ID))
		return
	}

	task := &MonitorTask{
		TestCase: tc,
		Ticker:   time.NewTicker(time.Duration(tc.MonitorInterval) * time.Second),
		StopChan: make(chan struct{}),
		Running:  true,
	}
	tasks[tc.ID] = task

	// 启动任务调度协程（仅负责定时，不执行实际任务）
	go scheduleTask(task)

	fmt.Printf("启动监控任务: [%s] %s (周期: %ds)\n", tc.ID, tc.Desc, tc.MonitorInterval)
}

// scheduleTask 任务调度协程（负责定时触发）
func scheduleTask(task *MonitorTask) {
	// 立即执行第一次
	taskChan <- task.TestCase

	for {
		select {
		case <-task.Ticker.C:
			taskChan <- task.TestCase
		case <-task.StopChan:
			task.Running = false
			return
		}
	}
}

// executeAndMonitor 执行监控并处理结果
func executeAndMonitor(task *MonitorTask) {
	tc := task.TestCase
	logger.Info("Executing monitor task", zap.String("id", tc.ID))

	result := testcase.ExecuteTestCase(tc)

	// 记录监控结果
	monitorResult := MonitorResult{
		TestCase:  tc,
		Result:    result,
		Timestamp: timeutil.Now(),
	}

	// 检查是否需要告警
	checkAlerts(&monitorResult)

	// 保存结果
	resultsMu.Lock()
	results = append(results, monitorResult)
	// 保持最多1000条记录
	if len(results) > 1000 {
		results = results[len(results)-1000:]
	}
	resultsMu.Unlock()

	// 如果有告警，发送邮件
	if monitorResult.AlertType != "" && (tc.AlertOnFailure || tc.AlertOnSlow) {
		sendAlertEmail(monitorResult)
	}
}

// checkAlerts 检查是否需要告警
func checkAlerts(result *MonitorResult) {
	tc := result.TestCase

	// 检查失败告警
	if !result.Result.Passed && tc.AlertOnFailure {
		result.AlertType = "failure"
		result.AlertMsg = fmt.Sprintf("接口监控告警: [%s] %s 执行失败 - %s", tc.ID, tc.Desc, result.Result.Error)
		logger.Error(result.AlertMsg)
		return
	}

	// 检查响应时间告警（合并超时和SLA）
	if tc.ResponseThreshold > 0 && result.Result.Duration.Milliseconds() > int64(tc.ResponseThreshold) && tc.AlertOnSlow {
		result.AlertType = "slow"
		result.AlertMsg = fmt.Sprintf("接口监控告警: [%s] %s 响应超时 - 耗时 %.2fms > 阈值 %dms",
			tc.ID, tc.Desc, result.Result.Duration.Milliseconds(), tc.ResponseThreshold)
		logger.Warn(result.AlertMsg)
	}
}

// sendAlertEmail 发送告警邮件并保存告警记录
func sendAlertEmail(result MonitorResult) {
	if !email.Config.Enabled {
		return
	}

	tc := result.TestCase

	// 根据配置计算告警间隔（默认 6 小时）
	alertInterval := time.Duration(config.GlobalConfig.Monitor.AlertInterval) * time.Second
	if alertInterval <= 0 {
		alertInterval = 6 * time.Hour
	}

	// 检查同一接口是否在告警间隔内已发送过邮件
	lastAlertMu.Lock()
	if last, ok := lastAlertTime[tc.ID]; ok && timeutil.Now().Sub(last) < alertInterval {
		lastAlertMu.Unlock()
		logger.Info("Alert email suppressed due to alert interval",
			zap.String("id", tc.ID),
			zap.Duration("since_last", timeutil.Now().Sub(last)),
			zap.Duration("interval", alertInterval))
		return
	}
	lastAlertTime[tc.ID] = timeutil.Now()
	lastAlertMu.Unlock()

	// 根据告警类型确定优先级和图标
	alertLevel := "WARNING"
	alertIcon := "⚠️"
	if result.AlertType == "failure" {
		alertLevel = "CRITICAL"
		alertIcon = "🚨"
	} else if result.AlertType == "slow" {
		alertLevel = "WARNING"
		alertIcon = "⏱️"
	}

	subject := fmt.Sprintf("[%s] gwatch 接口监控告警 - %s", alertLevel, tc.ID)
	body := fmt.Sprintf(`%s ===== 接口监控告警 ===== %s

【告警级别】%s
【告警时间】%s
【监控设备】%s

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

【测试用例】
  ID:         %s
  描述:       %s
  监控周期:   %ds

【告警详情】
  类型:       %s
  消息:       %s

【执行结果】
  状态:       %s
  耗时:       %.2fms
  HTTP状态码: %d

【请求信息】
  URL:        %s
  方法:       %s

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

【时间信息】
  开始时间:   %s
  结束时间:   %s

========================================
来自 gwatch 接口监控系统`,
		alertIcon, alertIcon,
		alertLevel,
		timeutil.FormatDateTime(timeutil.Now()),
		getDeviceName(),
		tc.ID,
		tc.Desc,
		tc.MonitorInterval,
		result.AlertType,
		result.AlertMsg,
		map[bool]string{true: "✅ 通过", false: "❌ 失败"}[result.Result.Passed],
		result.Result.Duration.Milliseconds(),
		result.Result.ActualStatus,
		tc.URL,
		tc.Method,
		timeutil.FormatDateTime(result.Result.StartTime),
		timeutil.FormatDateTime(result.Result.EndTime),
	)

	// 保存告警记录到文件
	saveAlertRecord(body, tc.ID)

	// 发送告警邮件
	if err := email.SendCustomEmail(subject, body); err != nil {
		logger.Warn("Failed to send alert email", zap.Error(err))
	}
}

// saveAlertRecord 保存告警记录到文件
func saveAlertRecord(content, testCaseID string) {
	// 构建告警目录路径
	reportDir := config.GlobalConfig.App.ReportDir
	if reportDir == "" {
		reportDir = "./reports"
	}

	// 创建告警子目录
	alertDir := filepath.Join(reportDir, "alerts", timeutil.Now().Format("20060102"))
	if err := os.MkdirAll(alertDir, 0755); err != nil {
		logger.Warn("Failed to create alert directory", zap.Error(err))
		return
	}

	// 生成文件名：alert_{timestamp}_{testcase_id}.log
	timestamp := timeutil.Now().Format("150405")
	filename := fmt.Sprintf("alert_%s_%s.log", timestamp, testCaseID)
	filePath := filepath.Join(alertDir, filename)

	// 写入告警内容
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		logger.Warn("Failed to save alert record", zap.String("file", filePath), zap.Error(err))
	} else {
		logger.Info("Alert record saved", zap.String("file", filePath))
	}
}

// getDeviceName 获取设备名称
func getDeviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return "Unknown"
	}
	return name
}

// StopAllTasks 停止所有监控任务
func StopAllTasks() {
	// 先停止所有调度协程
	tasksMu.Lock()
	for id, task := range tasks {
		task.Ticker.Stop()
		close(task.StopChan)
		task.Running = false
		delete(tasks, id)
		logger.Info("Stopped monitor task", zap.String("id", id))
	}
	tasksMu.Unlock()

	// 通知所有 worker 停止
	if stopChan != nil {
		close(stopChan)
	}

	// 重置通道
	taskChan = nil
	stopChan = nil
}

// GetResults 获取监控结果
func GetResults() []MonitorResult {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	return append([]MonitorResult{}, results...)
}

// GetTaskCount 获取监控任务数量
func GetTaskCount() int {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	return len(tasks)
}

// startHotReload 启动热加载协程
func startHotReload() {
	ticker := time.NewTicker(hotReloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hotReload()
		case <-stopChan:
			logger.Info("Hot reload stopped")
			return
		}
	}
}

// hotReload 执行热加载逻辑
func hotReload() {
	logger.Debug("Checking for new test cases")

	// 获取当前测试用例目录
	caseDir := config.GlobalConfig.App.CaseDir
	if caseDir == "" {
		caseDir = "./demo"
	}

	// 重新解析所有测试用例
	newCases, err := psv.ParseFiles([]string{caseDir})
	if err != nil {
		logger.Warn("Failed to parse files during hot reload", zap.Error(err))
		return
	}

	// 过滤出启用监控的测试用例
	newMonitorCases := filterMonitorCases(newCases)

	// 获取当前已注册的任务ID
	tasksMu.Lock()
	currentTaskIDs := make(map[string]bool)
	for id := range tasks {
		currentTaskIDs[id] = true
	}
	tasksMu.Unlock()

	// 添加新的测试用例
	newCount := 0
	for _, tc := range newMonitorCases {
		tasksMu.Lock()
		_, exists := tasks[tc.ID]
		tasksMu.Unlock()

		if !exists {
			startTask(tc)
			newCount++
		}
	}

	// 移除已删除的测试用例（可选功能，默认不启用）
	// removeDeletedTestCases(newMonitorCases)

	if newCount > 0 {
		logger.Info("Hot reload completed", zap.Int("new_tasks", newCount))
		fmt.Printf("\n[热加载] 发现 %d 个新测试用例，已自动添加到监控\n", newCount)
	}
}

// removeDeletedTestCases 移除已删除的测试用例
func removeDeletedTestCases(activeCases []psv.TestCase) {
	activeIDs := make(map[string]bool)
	for _, tc := range activeCases {
		activeIDs[tc.ID] = true
	}

	tasksMu.Lock()
	var toRemove []string
	for id := range tasks {
		if !activeIDs[id] {
			toRemove = append(toRemove, id)
		}
	}
	tasksMu.Unlock()

	for _, id := range toRemove {
		removeTask(id)
		fmt.Printf("[热加载] 测试用例 %s 已移除\n", id)
	}
}

// removeTask 移除单个任务
func removeTask(id string) {
	tasksMu.Lock()
	task, exists := tasks[id]
	if exists {
		task.Ticker.Stop()
		close(task.StopChan)
		delete(tasks, id)
	}
	tasksMu.Unlock()

	if exists {
		logger.Info("Removed monitor task", zap.String("id", id))
	}
}

// scheduleDailyReport 调度每日报告生成
func scheduleDailyReport() {
	for {
		now := timeutil.Now()
		reportTimeStr := config.GlobalConfig.Monitor.ReportTime
		if reportTimeStr == "" {
			reportTimeStr = "00:00"
		}

		var reportHour, reportMinute int
		fmt.Sscanf(reportTimeStr, "%d:%d", &reportHour, &reportMinute)

		reportTime := time.Date(now.Year(), now.Month(), now.Day(), reportHour, reportMinute, 0, 0, now.Location())
		if now.After(reportTime) {
			reportTime = reportTime.Add(24 * time.Hour)
		}

		duration := reportTime.Sub(now)
		logger.Info("Scheduling daily report", zap.Time("next_run", reportTime))

		time.Sleep(duration)

		yesterday := timeutil.Now().Add(-24 * time.Hour)
		logger.Info("Generating daily report for", zap.String("date", yesterday.Format("2006-01-02")))
		generateAndSendDailyReport(yesterday)
	}
}

// generateAndSendDailyReport 生成并发送每日报告
func generateAndSendDailyReport(date time.Time) {
	// 从CSV文件中读取指定日期的监控结果
	csvResults, err := storage.GetMonitorResultsByDate(date)
	if err != nil {
		logger.Error("Failed to get monitor results from CSV", zap.Error(err))
		return
	}

	// 转换为 report.MonitorResult 类型
	reportResults := make([]report.MonitorResult, len(csvResults))
	for i, r := range csvResults {
		reportResults[i] = report.MonitorResult{
			TestCase: psv.TestCase{
				ID:             r.TestCaseID,
				Desc:           r.TestCaseDesc,
				URL:            r.URL,
				Method:         r.Method,
				ExpectedStatus: r.ExpectedStatus,
				ExpectedBody:   r.ExpectedBody,
			},
			Result: testcase.TestResult{
				ActualStatus: r.ActualStatus,
				ResponseBody: r.ActualBody,
				Error:        r.ErrorMsg,
				Duration:     time.Duration(r.DurationMS) * time.Millisecond,
				Passed:       r.Success,
			},
			Timestamp: r.Timestamp,
		}
	}

	// 生成报告
	r := report.GenerateDailyReport(reportResults, date)

	// 保存报告
	_, err = r.SaveReport()
	if err != nil {
		logger.Error("Failed to save daily report", zap.Error(err))
		return
	}

	// 发送邮件
	if r.FailedTasks > 0 || config.GlobalConfig.Monitor.AlertOnFailure {
		err = r.SendReportEmail()
		if err != nil {
			logger.Error("Failed to send daily report email", zap.Error(err))
		}
	}
}

// sendStartupNotification 发送监控启动通知邮件
func sendStartupNotification(taskCount int) {
	if !email.Config.Enabled {
		logger.Info("Email is disabled, skipping startup notification")
		return
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Unknown"
	}

	subject := "[gwatch] 监控服务已启动"
	body := fmt.Sprintf(`══════════════════════════════════════════════════════════════╗
║              gwatch 接口监控服务启动通知                     ║
╚══════════════════════════════════════════════════════════════╝

【设备名称】%s
【启动时间】%s
【监控任务数】%d

【状态】监控服务已成功启动，开始执行监控任务。

来自 gwatch 接口监控系统`, hostname, timeutil.FormatDateTime(timeutil.Now()), taskCount)

	logger.Info("Sending startup notification email")
	err := email.SendCustomEmail(subject, body)
	if err != nil {
		logger.Error("Failed to send startup notification email", zap.Error(err))
	}
}
