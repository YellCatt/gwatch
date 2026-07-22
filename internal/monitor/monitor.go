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
	"gwatch/internal/testcase"
	"gwatch/internal/timeutil"
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
	TestCase    psv.TestCase
	Result      testcase.TestResult
	Timestamp   time.Time
	AlertType   string // "failure", "timeout", "sla", ""
	AlertMsg    string
}

var (
	tasks      = make(map[string]*MonitorTask)
	tasksMu    sync.Mutex
	results    = make([]MonitorResult, 0, 1000)
	resultsMu  sync.Mutex
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

	fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║              gwatch 接口监控模式                        ║\n")
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n")
	fmt.Printf("监控任务数: %d\n", len(monitorCases))

	for _, tc := range monitorCases {
		startTask(tc)
	}

	// 设置信号处理，优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n监控任务已启动，按 Ctrl+C 停止...")

	// 阻塞等待信号
	<-sigChan

	fmt.Println("\n收到退出信号，正在停止监控任务...")
	StopAllTasks()
	fmt.Println("监控任务已全部停止")
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

	go runTask(task)

	fmt.Printf("启动监控任务: [%s] %s (周期: %ds)\n", tc.ID, tc.Desc, tc.MonitorInterval)
}

// runTask 运行监控任务
func runTask(task *MonitorTask) {
	// 立即执行第一次
	executeAndMonitor(task)

	for {
		select {
		case <-task.Ticker.C:
			executeAndMonitor(task)
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
	reportDir := config.AppConfig.Test.ReportDir
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
	tasksMu.Lock()
	defer tasksMu.Unlock()

	for id, task := range tasks {
		task.Ticker.Stop()
		close(task.StopChan)
		task.Running = false
		delete(tasks, id)
		logger.Info("Stopped monitor task", zap.String("id", id))
	}
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