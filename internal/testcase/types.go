package testcase

import (
	"sync"
	"time"

	"gwatch/internal/psv"
)

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

func SetAllTestCases(cases []psv.TestCase) {
	allTestCases = cases
}

func GetResults() []TestResult {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	return append([]TestResult{}, results...)
}

func AddResult(result TestResult) {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	results = append(results, result)
}
