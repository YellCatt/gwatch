package testcase

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
	"gwatch/internal/vars"
)

// GenerateReport 根据测试执行结果生成全量报告和错误报告（CSV 格式，管道分隔）。
func GenerateReport(results []TestResult) (string, string) {
	var allReport strings.Builder
	var errorReport strings.Builder

	header := "status|id|desc|method|url|request_headers|request_body|tags|duration_s|expect_status|actual_status|actual_body|expect_body|pre_conditions|post_conditions|extracted_vars|start_time|end_time|diff\n"
	allReport.WriteString(header)

	var failedLines []string

	for _, result := range results {
		status := "P"
		if result.TestCase.Skip {
			status = "S"
		} else if !result.Passed {
			status = "F"
		}

		tags := strings.Join(result.TestCase.Tags, ",")

		preConditions := strings.Join(result.TestCase.Pre, ";")

		postConditions := strings.Join(result.TestCase.Post, ";")

		diff := result.Error

		processedURL := vars.Replace(result.TestCase.URL)
		processedExpectedBody := vars.Replace(result.TestCase.ExpectedBody)

		startTime := timeutil.FormatDateTime(result.StartTime)
		endTime := timeutil.FormatDateTime(result.EndTime)

		line := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%.3f|%d|%d|%s|%s|%s|%s|%s|%s|%s|%s\n",
			status,
			escapePipe(result.TestCase.ID),
			escapePipe(result.TestCase.Desc),
			result.TestCase.Method,
			escapePipe(processedURL),
			escapePipe(result.RequestHeaders),
			escapePipe(result.RequestBody),
			tags,
			result.Duration.Seconds(),
			result.TestCase.ExpectedStatus,
			result.ActualStatus,
			escapePipe(result.ResponseBody),
			escapePipe(processedExpectedBody),
			escapePipe(preConditions),
			escapePipe(postConditions),
			escapePipe(result.ExtractedVars),
			startTime,
			endTime,
			escapePipe(diff),
		)
		allReport.WriteString(line)

		if !result.Passed && !result.TestCase.Skip {
			failedLines = append(failedLines, line)
		}
	}

	if len(failedLines) > 0 {
		errorReport.WriteString(header)
		for _, line := range failedLines {
			errorReport.WriteString(line)
		}
	}

	return allReport.String(), errorReport.String()
}

// SaveReports 将报告内容保存到文件系统，返回全量报告路径和错误报告路径。
func SaveReports(allReport, errorReport string, timestamp ...string) (string, string) {
	ts := timeutil.FormatCompact(timeutil.Now())
	if len(timestamp) > 0 && timestamp[0] != "" {
		ts = timestamp[0]
	}

	reportDir := config.GlobalConfig.App.ReportDir

	if err := os.MkdirAll(reportDir, 0755); err != nil {
		logger.Error("Failed to create report directory", zap.Error(err))
		return "", ""
	}

	allPath := fmt.Sprintf("%s/report_%s.csv", reportDir, ts)
	if err := os.WriteFile(allPath, []byte(allReport), 0644); err != nil {
		logger.Error("Failed to save report", zap.Error(err))
	}

	var errorPath string
	if errorReport != "" {
		errorPath = fmt.Sprintf("%s/report_%s_error.csv", reportDir, ts)
		if err := os.WriteFile(errorPath, []byte(errorReport), 0644); err != nil {
			logger.Error("Failed to save error report", zap.Error(err))
		}
	}

	logger.Info("Reports saved", zap.String("all", allPath), zap.String("error", errorPath))
	return allPath, errorPath
}

// PrintSummary 打印测试执行摘要到控制台，包括通过/失败/跳过统计和耗时。
func PrintSummary(results []TestResult) {
	var passed, failed, skipped int
	var setupPassed, setupFailed int
	var totalDuration time.Duration

	aggregated := AggregateResultsByFile(results)

	for _, r := range aggregated {
		totalDuration += r.Duration

		if IsSetupTestCase(r.TestCase) {
			if r.Passed {
				setupPassed++
			} else {
				setupFailed++
			}
			continue
		}

		if r.TestCase.Skip {
			skipped++
		} else if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║              gwatch 接口测试                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("测试用例: %d 通过, %d 失败, %d 跳过\n", passed, failed, skipped)
	if setupPassed+setupFailed > 0 {
		fmt.Printf("环境准备: %d 通过, %d 失败\n", setupPassed, setupFailed)
	}
	fmt.Printf("总时长: %.3fs\n", totalDuration.Seconds())
}
