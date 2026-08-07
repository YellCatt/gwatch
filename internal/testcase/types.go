package testcase

import (
	"sync"
	"time"

	"gwatch/internal/psv"
)

// TestResult 测试用例执行结果结构体。
type TestResult struct {
	TestCase       psv.TestCase
	Passed         bool
	Error          string
	Duration       time.Duration
	StartTime      time.Time
	EndTime        time.Time
	ResponseBody   string
	ActualStatus   int
	RequestHeaders string
	RequestBody    string
	ExtractedVars  string
	ProcessedURL   string
}

var (
	results      []TestResult
	resultsMu    sync.Mutex
	globalVars   = make(map[string]string)
	globalVarsMu sync.Mutex
)

var allTestCases []psv.TestCase

// SetAllTestCases 设置所有测试用例列表。
func SetAllTestCases(cases []psv.TestCase) {
	allTestCases = cases
}

// GetResults 获取所有测试执行结果的副本。
func GetResults() []TestResult {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	return append([]TestResult{}, results...)
}

// AddResult 添加一条测试执行结果到结果列表。
func AddResult(result TestResult) {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	results = append(results, result)
}
