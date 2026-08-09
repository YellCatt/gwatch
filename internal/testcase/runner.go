package testcase

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/psv"
	"gwatch/internal/storage"
	"gwatch/internal/timeutil"
	"gwatch/internal/vars"
)

func RunTests(paths []string, tags []string) {
	if err := storage.InitDB(config.GlobalConfig.App.DataDir); err != nil {
		logger.Warn("CSV 存储初始化失败", zap.Error(err))
	} else {
		logger.Info("CSV 存储初始化成功")
		count, err := storage.GetTotalExecutionCount()
		if err != nil {
			logger.Warn("Failed to get execution count", zap.Error(err))
		} else {
			logger.Info("Historical execution records found", zap.Int("count", count))
		}
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

	SetAllTestCases(testCases)

	totalTestCaseCount, totalChainCount, totalIndependentCount := CountTestSummary(testCases)

	testCases = FilterByTags(testCases, tags)

	if len(testCases) == 0 {
		logger.Info("No test cases to run", zap.Strings("paths", paths))
		return
	}

	logger.Info("Starting API tests", zap.Int("count", len(testCases)))

	estimatedDuration := CalculateEstimatedDuration(testCases)

	var estimatedDurationStr string
	if estimatedDuration > 0 {
		estimatedDurationStr = FormatDuration(estimatedDuration)
	} else {
		estimatedDurationStr = "无历史数据"
	}

	executedCount, executedChainCount, executedIndependentCount := CountTestSummary(testCases)

	PrintTaskSummary(totalTestCaseCount, totalChainCount, totalIndependentCount, tags, executedCount, executedChainCount, executedIndependentCount, estimatedDurationStr)

	reportTimestamp := timeutil.FormatCompact(timeutil.Now())

	ExecuteGlobalPreConditions(testCases)

	var results []TestResult
	chainFiles := GetChainFiles(testCases)
	for i, tc := range testCases {
		result := ExecuteTestCase(tc)
		results = append(results, result)

		fmt.Printf("\n\n────────────────────────────────────────────────────────────\n")
		stepLabel := "测试"
		if chainFiles[tc.FileName] {
			stepLabel = "链式步骤"
		}
		fmt.Printf("第 %d/%d 个%s完成，正在更新报告...\n", i+1, len(testCases), stepLabel)
		fmt.Printf("────────────────────────────────────────────────────────────\n")

		allReport, errorReport := GenerateReport(results)
		allPath, errorPath := SaveReports(allReport, errorReport, reportTimestamp)

		fmt.Printf("\nPSV 报告已保存: %s\n", allPath)
		if errorPath != "" {
			fmt.Printf("异常用例 PSV 报告已保存: %s\n", errorPath)
		}
	}

	PrintSummary(results)

	if err := storage.CalculateAndStoreAverages(); err != nil {
		logger.Warn("Failed to calculate and store average durations", zap.Error(err))
	} else {
		logger.Info("Successfully calculated and stored average durations")
	}

	ExecuteGlobalPostConditions(testCases)

	vars.CleanupGlobalPreVariables()
	logger.Info("Cleaned up global pre variables")

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

func PrintTaskSummary(totalTestCaseCount, totalChainCount, totalIndependentCount int, tags []string, executedCount, executedChainCount, executedIndependentCount int, estimatedDurationStr string) {
	fmt.Printf("\n════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ 任务统计信息                                           ║\n")
	fmt.Printf("╠════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 解析出的任务总数: %-37d║\n", totalTestCaseCount)
	fmt.Printf("║   链式任务: %-43d║\n", totalChainCount)
	fmt.Printf("║   独立任务: %-43d║\n", totalIndependentCount)
	if len(tags) > 0 {
		fmt.Printf("║ 应用标签过滤: %-40s║\n", strings.Join(tags, ", "))
		fmt.Printf("║ 过滤后实际执行数: %-36d║\n", executedCount)
		fmt.Printf("║   链式任务: %-43d║\n", executedChainCount)
		fmt.Printf("║   独立任务: %-43d║\n", executedIndependentCount)
	} else {
		fmt.Printf("║ 未应用标签过滤，本次共执行 %-27d║\n", executedCount)
		fmt.Printf("║   链式任务: %-43d║\n", executedChainCount)
		fmt.Printf("║   独立任务: %-43d║\n", executedIndependentCount)
	}

	fmt.Printf("╠════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 预估执行时间: %-41s║\n", estimatedDurationStr)
	fmt.Printf("╚════════════════════════════════════════════════════════╝\n\n")
}

func CalculateEstimatedDuration(testCases []psv.TestCase) time.Duration {
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

func FormatDuration(d time.Duration) string {
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
