// Package cmd 提供命令行接口功能
// 使用 cobra 框架实现命令行参数解析和子命令管理
package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/httpclient"
	"gwatch/internal/logger"
	"gwatch/internal/monitor"
	"gwatch/internal/psv"
	"gwatch/internal/storage"
	"gwatch/internal/testcase"
	"gwatch/internal/timeutil"
	"gwatch/internal/vars"
)

var (
	// rootCmd 是命令行应用的根命令
	rootCmd = &cobra.Command{
		Use:   "gwatch [paths...]",
		Short: "gwatch - API Testing and Monitoring Tool",
		Long:  `A powerful enterprise-grade API testing and monitoring tool written in Go.`,
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if monitorFlag {
				startMonitor(args)
			} else {
				runTests(args)
			}
		},
	}

	// tagsFlag 存储命令行指定的标签过滤参数
	tagsFlag string
	// monitorFlag 是否启用监控模式
	monitorFlag bool
)

// Execute 启动命令行应用
// 调用 cobra 的 Execute 方法处理命令行输入
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

// init 函数在包初始化时执行
// 注册初始化函数和命令行参数
func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.Flags().StringVar(&config.CfgFile, "config", "", "config file (default is ./config/config.yaml)")
	rootCmd.Flags().StringVarP(&tagsFlag, "tags", "t", "", "filter tests by tags (comma-separated)")
	rootCmd.Flags().BoolVarP(&monitorFlag, "monitor", "m", false, "enable monitor mode for API endpoints (requires monitor_enabled=true in test cases)")
}

// initConfig 初始化应用配置
// 依次初始化：必要目录和默认配置、配置、日志、全局变量、邮件配置
func initConfig() {
	// 自动创建必要的目录和默认配置文件
	initDirectories()
	
	config.InitConfig()
	
	logger.InitLogger(logger.LogConfig{
		Level:    config.AppConfig.Log.Level,
		Encoding: config.AppConfig.Log.Encoding,
		Output:   config.AppConfig.Log.Output,
	})

	// 初始化内置变量
	vars.Set("base_url", config.AppConfig.Target.BaseURL)

	// 加载用户自定义变量（支持任意变量名）
	if len(config.AppConfig.Vars) > 0 {
		vars.InitFromConfig(config.AppConfig.Vars)
		logger.Info("用户自定义变量加载完成", zap.Int("count", len(config.AppConfig.Vars)), zap.Any("vars", maskVars(config.AppConfig.Vars)))
	} else {
		logger.Info("未配置用户自定义变量")
	}
	logger.Info("当前可用变量", zap.Any("vars", vars.GetAll()))

	email.InitEmail(email.EmailConfig{
		Enabled:      config.AppConfig.Email.Enabled,
		FromEmail:    config.AppConfig.Email.From,
		ToEmail:      config.AppConfig.Email.To,
		AuthCode:     config.AppConfig.Email.AuthCode,
		SMTPServer:   config.AppConfig.Email.SMTPServer,
		SMTPPort:     config.AppConfig.Email.SMTPPort,
		DeviceName:   config.AppConfig.App.HostName,
		ErrorSubject: config.AppConfig.Email.ErrorSubject,
	})
}

// maskVars 用于日志中掩码敏感变量值
func maskVars(vars map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range vars {
		result[k] = maskString(v)
	}
	return result
}

// maskString 用于日志中掩码敏感信息
func maskString(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

// runTests 执行测试主流程
// paths: 用户指定的测试用例路径列表
func runTests(paths []string) {
	// 初始化 HTTP 客户端
	httpclient.InitClient()

	// 初始化 CSV 存储
	logger.Info("准备初始化 CSV 存储", zap.String("DataDir", config.AppConfig.App.DataDir))
	if err := storage.InitDB(config.AppConfig.App.DataDir); err != nil {
		logger.Warn("CSV 存储初始化失败", zap.Error(err))
	} else {
		logger.Info("CSV 存储初始化成功")

		// 检查历史记录数
		count, err := storage.GetTotalExecutionCount()
		if err != nil {
			logger.Warn("Failed to get execution count", zap.Error(err))
		} else {
			logger.Info("Historical execution records found", zap.Int("count", count))
		}
	}


	// 如果未指定路径，使用默认测试用例目录
	if len(paths) == 0 {
		paths = []string{config.AppConfig.Test.TestCaseDir}
	}

	// 解析 PSV/CSV 测试用例文件
	testCases, err := psv.ParseFiles(paths)
	if err != nil {
		logger.Error("Failed to parse PSV files", zap.Error(err))
		errorMsg := fmt.Sprintf("解析测试用例文件失败: %v", err)
		if err := email.SendErrorReportEmail(errorMsg); err != nil {
			logger.Warn("Failed to send error report email", zap.Error(err))
		}
		os.Exit(1)
	}

	// 设置所有测试用例（用于链式测试查找前置条件）
	testcase.SetAllTestCases(testCases)

	// 解析标签过滤参数
	var tags []string
	if tagsFlag != "" {
		tags = strings.Split(tagsFlag, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	// 保存原始测试用例总数（过滤前），链式文件按 1 个计数
	totalTestCaseCount, totalChainCount, totalIndependentCount := testcase.CountTestSummary(testCases)

	// 根据标签过滤测试用例
	testCases = testcase.FilterByTags(testCases, tags)


	// 如果没有测试用例，直接返回
	if len(testCases) == 0 {
		logger.Info("No test cases to run")
		return
	}

	// 开始执行测试
	logger.Info("Starting API tests", zap.Int("count", len(testCases)))

	// 计算预估执行时间
	estimatedDuration := calculateEstimatedDuration(testCases)

	// 格式化预估时间
	var estimatedDurationStr string
	if estimatedDuration > 0 {
		estimatedDurationStr = formatDuration(estimatedDuration)
	} else {
		estimatedDurationStr = "无历史数据"
	}

	// 打印本次执行的测试用例统计信息
	executedCount, executedChainCount, executedIndependentCount := testcase.CountTestSummary(testCases)

	fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ 测试用例统计信息                                       ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 解析出的测试用例总数: %-35d║\n", totalTestCaseCount)
	fmt.Printf("║   链式测试: %-43d║\n", totalChainCount)
	fmt.Printf("║   独立测试: %-43d║\n", totalIndependentCount)
	if len(tags) > 0 {
		fmt.Printf("║ 应用标签过滤: %-40s║\n", strings.Join(tags, ", "))
		fmt.Printf("║ 过滤后实际执行数: %-36d║\n", executedCount)
		fmt.Printf("║   链式测试: %-43d║\n", executedChainCount)
		fmt.Printf("║   独立测试: %-43d║\n", executedIndependentCount)
	} else {
		fmt.Printf("║ 未应用标签过滤，本次共执行 %-27d║\n", executedCount)
		fmt.Printf("║   链式测试: %-43d║\n", executedChainCount)
		fmt.Printf("║   独立测试: %-43d║\n", executedIndependentCount)
	}

	fmt.Printf("╠════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 预估执行时间: %-41s║\n", estimatedDurationStr)
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")


	// 生成报告时间戳（测试开始时生成，后续更新报告时使用同一个时间戳）
	reportTimestamp := timeutil.FormatCompact(timeutil.Now())

	// 执行全局前置条件（所有测试用例执行前运行）
	if len(config.AppConfig.Test.GlobalPre) > 0 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║ 执行全局前置条件                                       ║\n")
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

		for _, preID := range config.AppConfig.Test.GlobalPre {
			found := false
			for _, tc := range testCases {
				if tc.ID == preID {
					fmt.Printf("[全局前置] 执行: %s - %s\n", tc.ID, tc.Desc)
					result := testcase.ExecuteTestCase(tc)
					if !result.Passed {
						fmt.Printf("[全局前置] ❌ 失败: %s\n", result.Error)
						fmt.Printf("\n全局前置条件失败，终止测试执行\n")
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

	// 运行测试（串行执行），每完成一个测试就更新一次报告
	var results []testcase.TestResult
	chainFiles := testcase.GetChainFiles(testCases)
	for i, tc := range testCases {
		result := testcase.ExecuteTestCase(tc)
		results = append(results, result)

		// 每完成一个测试用例就更新一次报告（覆盖同一个文件）
		fmt.Printf("\n\n────────────────────────────────────────────────────────────\n")
		stepLabel := "测试"
		if chainFiles[tc.FileName] {
			stepLabel = "链式步骤"
		}
		fmt.Printf("第 %d/%d 个%s完成，正在更新报告...\n", i+1, len(testCases), stepLabel)
		fmt.Printf("────────────────────────────────────────────────────────────\n")


		// 生成并保存测试报告（使用同一个时间戳，覆盖之前的报告）
		allReport, errorReport := testcase.GenerateReport(results)
		allPath, errorPath := testcase.SaveReports(allReport, errorReport, reportTimestamp)

		// 输出报告路径
		fmt.Printf("\nPSV 报告已保存: %s\n", allPath)
		if errorPath != "" {
			fmt.Printf("异常用例 PSV 报告已保存: %s\n", errorPath)
		}
	}

	// 打印最终测试摘要
	testcase.PrintSummary(results)

	// 测试结束后计算并存储所有成功测试用例的平均执行时间
	if err := storage.CalculateAndStoreAverages(); err != nil {
		logger.Warn("Failed to calculate and store average durations", zap.Error(err))
	} else {
		logger.Info("Successfully calculated and stored average durations")
	}

	// 执行全局后置条件（所有测试用例执行后运行）
	if len(config.AppConfig.Test.GlobalPost) > 0 {
		fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
		fmt.Printf("║ 执行全局后置条件                                       ║\n")
		fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")

		for _, postID := range config.AppConfig.Test.GlobalPost {
			found := false
			for _, tc := range testCases {
				if tc.ID == postID {
					fmt.Printf("[全局后置] 执行: %s - %s\n", tc.ID, tc.Desc)
					result := testcase.ExecuteTestCase(tc)
					if !result.Passed {
						fmt.Printf("[全局后置] ❌ 失败: %s\n", result.Error)
					} else {
						fmt.Printf("[全局后置] ✅ 成功\n")
					}
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("[全局后置] ⚠️ 未找到测试用例: %s\n", postID)
			}
		}
		fmt.Println()
	}

	// 如果有失败的测试用例，退出码设为 1
	failedCount := 0
	for _, r := range results {
		if !r.Passed && !r.TestCase.Skip {
			failedCount++
		}
	}

	if failedCount > 0 {
		os.Exit(1)
	}
}

// calculateEstimatedDuration 根据历史执行时间计算预估总耗时
func calculateEstimatedDuration(testCases []psv.TestCase) time.Duration {
	// 获取所有 URL 的平均执行时间
	averages, err := storage.GetAllAverageDurations()
	if err != nil {
		logger.Warn("Failed to get average durations", zap.Error(err))
		return 0
	}

	if len(averages) == 0 {
		return 0
	}

	var total time.Duration
	unknownCount := 0

	for _, tc := range testCases {
		// 跳过被标记为跳过的测试用例
		if tc.Skip {
			continue
		}

		url := vars.Replace(tc.URL)
		if avg, ok := averages[url]; ok {
			total += avg
		} else {
			unknownCount++
		}
	}

	// 如果有未知 URL，使用已知 URL 的平均时间作为估算
	if unknownCount > 0 && len(averages) > 0 {
		var avgAll time.Duration
		for _, avg := range averages {
			avgAll += avg
		}
		avgAll /= time.Duration(len(averages))
		total += avgAll * time.Duration(unknownCount)
	}

	return total
}

// formatDuration 格式化时间为可读字符串
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	} else if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
}

// initDirectories 自动创建必要的目录和默认配置文件
// 如果目录不存在则创建，已存在则跳过
func initDirectories() {
	// 需要创建的目录列表（使用默认值，因为此时配置还未加载）
	directories := []string{
		"./config",     // 配置文件目录
		"./logs",       // 日志目录
		"./reports",    // 测试报告目录
		"./sql",        // 数据存储目录（CSV文件）
		"./testcases",  // 测试用例目录
	}

	for _, dir := range directories {
		if dir == "." || dir == "/" {
			continue
		}

		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("警告: 创建目录失败 '%s': %v\n", dir, err)
		}
	}

	// 检查并创建默认配置文件
	createDefaultConfigFile()
}

// createDefaultConfigFile 如果 config.yaml 不存在，创建默认配置文件
func createDefaultConfigFile() {
	configPath := "./config/config.yaml"
	
	// 检查文件是否存在
	if _, err := os.Stat(configPath); err == nil {
		// 文件已存在，跳过创建
		return
	}

	// 默认配置内容
	defaultConfig := `target:
  base_url: "https://localhost:8080"
  timeout: 30

log:
  level: "info"
  encoding: "json"
  output: "./logs/gwatch.log"

test:
  report_dir: "./reports"
  test_case_dir: "./testcases"
  data_dir: "./sql"
  severe_status:
    - 500
  global_pre: []
  global_post: []
  device_name: ""

http:
  insecure_skip_verify: false

vars: {}

email:
  enabled: false
  from: ""
  to: []
  auth_code: ""
  smtp_server: "smtp.example.com"
  smtp_port: 465

cleaner:
  enabled: true
  retention_days: 30
  log_dir: "./logs"
  report_dir: "./reports"
  data_dir: "./sql"
  include_patterns:
    - "*.log"
    - "*.json"
    - "*.csv"
    - "*.txt"
  exclude_patterns: []
  interval_hours: 24
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		fmt.Printf("警告: 创建默认配置文件失败 '%s': %v\n", configPath, err)
	} else {
		fmt.Printf("已创建默认配置文件: %s\n", configPath)
	}
}

// startMonitor 启动监控模式
func startMonitor(paths []string) {
	// 初始化 HTTP 客户端
	httpclient.InitClient()

	// 如果未指定路径，使用默认测试用例目录
	if len(paths) == 0 {
		paths = []string{config.AppConfig.Test.TestCaseDir}
	}

	// 解析 PSV/CSV 测试用例文件
	testCases, err := psv.ParseFiles(paths)
	if err != nil {
		logger.Error("Failed to parse PSV files", zap.Error(err))
		errorMsg := fmt.Sprintf("解析测试用例文件失败: %v", err)
		if err := email.SendErrorReportEmail(errorMsg); err != nil {
			logger.Warn("Failed to send error report email", zap.Error(err))
		}
		os.Exit(1)
	}

	// 设置所有测试用例（用于链式测试查找前置条件）
	testcase.SetAllTestCases(testCases)

	// 根据标签过滤测试用例
	var tags []string
	if tagsFlag != "" {
		tags = strings.Split(tagsFlag, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
		testCases = testcase.FilterByTags(testCases, tags)
	}

	// 启动监控
	monitor.StartMonitor(testCases)
}