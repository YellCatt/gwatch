// Package testcase 提供测试用例管理和执行功能
// 支持测试用例的注册、执行和结果汇总
package testcase

import (
	"sync"

	"go.uber.org/zap"

	"pipetGo/internal/httpclient"
	"pipetGo/internal/logger"
)

// TestCase 表示一个测试用例
type TestCase struct {
	Name       string        // 测试用例名称
	Run        func() error  // 测试执行函数
	Skip       bool          // 是否跳过
	SkipReason string        // 跳过原因
}

// testCases 存储所有注册的测试用例
var testCases []TestCase

// mu 用于保护 testCases 的并发访问
var mu sync.Mutex

// RegisterTest 注册一个测试用例
// name: 测试用例名称
// run: 测试执行函数
func RegisterTest(name string, run func() error) {
	mu.Lock()
	defer mu.Unlock()
	testCases = append(testCases, TestCase{
		Name: name,
		Run:  run,
	})
}

// RegisterSkippedTest 注册一个被跳过的测试用例
// name: 测试用例名称
// reason: 跳过原因
func RegisterSkippedTest(name string, reason string) {
	mu.Lock()
	defer mu.Unlock()
	testCases = append(testCases, TestCase{
		Name:       name,
		Skip:       true,
		SkipReason: reason,
	})
}

// RunAll 执行所有注册的测试用例
// 串行执行，确保测试用例之间的依赖关系正确处理
func RunAll() {
	// 初始化 HTTP 客户端
	httpclient.InitClient()

	logger.Info("Starting API tests...")

	var results []testResult

	// 串行执行每个测试用例
	for _, tc := range testCases {
		if tc.Skip {
			logger.Info("Skipping test", zap.String("name", tc.Name), zap.String("reason", tc.SkipReason))
			results = append(results, testResult{name: tc.Name, skipped: true, skipReason: tc.SkipReason})
			continue
		}

		logger.Info("Running test", zap.String("name", tc.Name))
		err := tc.Run()
		if err != nil {
			logger.Error("Test failed", zap.String("name", tc.Name), zap.Error(err))
			results = append(results, testResult{name: tc.Name, failed: true, err: err})
		} else {
			logger.Info("Test passed", zap.String("name", tc.Name))
			results = append(results, testResult{name: tc.Name, passed: true})
		}
	}

	// 汇总测试结果
	summarizeResults(results)
}

// testResult 表示单个测试用例的执行结果
type testResult struct {
	name       string  // 测试用例名称
	passed     bool    // 是否通过
	failed     bool    // 是否失败
	skipped    bool    // 是否跳过
	skipReason string  // 跳过原因
	err        error   // 失败时的错误信息
}

// summarizeResults 汇总测试结果并输出日志
// results: 测试结果列表
func summarizeResults(results []testResult) {
	var passed, failed, skipped int

	for _, r := range results {
		if r.passed {
			passed++
		} else if r.failed {
			failed++
		} else if r.skipped {
			skipped++
		}
	}

	logger.Info("Test summary",
		zap.Int("total", passed+failed+skipped),
		zap.Int("passed", passed),
		zap.Int("failed", failed),
		zap.Int("skipped", skipped))

	if failed > 0 {
		logger.Error("Some tests failed")
	} else {
		logger.Info("All tests passed!")
	}
}