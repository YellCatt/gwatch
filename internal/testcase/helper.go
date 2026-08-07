package testcase

import (
	"slices"
	"strings"
	"time"

	"gwatch/internal/psv"
)

// IsChainTestCase 判断测试用例是否为链路测试用例（ID 以 "chain" 开头）。
func IsChainTestCase(tc psv.TestCase) bool {
	return strings.HasPrefix(tc.ID, "chain")
}

// IsGlobalPreCondition 判断测试用例是否为全局前置条件（ID 以 "pre_" 开头）。
func IsGlobalPreCondition(tc psv.TestCase) bool {
	return strings.HasPrefix(tc.ID, "pre_")
}

// IsGlobalPostCondition 判断测试用例是否为全局后置条件（ID 以 "post_" 开头）。
func IsGlobalPostCondition(tc psv.TestCase) bool {
	return strings.HasPrefix(tc.ID, "post_")
}

// IsSetupTestCase 判断测试用例是否为环境准备用例（全局前置或后置条件）。
func IsSetupTestCase(tc psv.TestCase) bool {
	return IsGlobalPreCondition(tc) || IsGlobalPostCondition(tc)
}

// GetTestCaseType 获取测试用例类型（chain 或 independent）。
func GetTestCaseType(tc psv.TestCase) string {
	if IsChainTestCase(tc) {
		return "chain"
	}
	return "independent"
}

// FilterIndependentTests 过滤出所有非链路测试用例。
func FilterIndependentTests(testCases []psv.TestCase) []psv.TestCase {
	var independent []psv.TestCase
	for _, tc := range testCases {
		if !IsChainTestCase(tc) {
			independent = append(independent, tc)
		}
	}
	return independent
}

// FilterChainTests 过滤出所有链路测试用例。
func FilterChainTests(testCases []psv.TestCase) []psv.TestCase {
	var chain []psv.TestCase
	for _, tc := range testCases {
		if IsChainTestCase(tc) {
			chain = append(chain, tc)
		}
	}
	return chain
}

// FilterByTags 按标签过滤测试用例（任一标签匹配即包含）。
func FilterByTags(testCases []psv.TestCase, tags []string) []psv.TestCase {
	if len(tags) == 0 {
		return testCases
	}

	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[strings.ToLower(tag)] = true
	}

	var filtered []psv.TestCase
	for _, tc := range testCases {
		for _, tag := range tc.Tags {
			if tagSet[strings.ToLower(tag)] {
				filtered = append(filtered, tc)
				break
			}
		}
	}

	return filtered
}

// CountStatisticalTestCases 统计参与测试的用例数（链路文件算1个，独立用例按个数计算）。
func CountStatisticalTestCases(testCases []psv.TestCase) int {
	byFile := groupByFile(testCases)
	count := 0
	for _, cases := range byFile {
		if isChainFile(cases) {
			count++
		} else {
			count += len(cases)
		}
	}
	return count
}

// CountTestSummary 统计测试用例汇总：总数、链路数、独立用例数。
func CountTestSummary(testCases []psv.TestCase) (total, chain, independent int) {
	byFile := groupByFile(testCases)
	for _, cases := range byFile {
		if isChainFile(cases) {
			chain++
			total++
		} else {
			independent += len(cases)
			total += len(cases)
		}
	}
	return
}

// SummarizeResultsByType 按类型汇总测试结果，分别统计链路和独立用例的通过/失败/跳过数量及总耗时。
func SummarizeResultsByType(results []TestResult) (chainPassed, chainFailed, chainSkipped, independentPassed, independentFailed, independentSkipped int, totalDuration time.Duration) {
	aggregated := AggregateResultsByFile(results)
	for _, r := range aggregated {
		totalDuration += r.Duration
		isChain := IsChainTestCase(r.TestCase)
		if r.TestCase.Skip {
			if isChain {
				chainSkipped++
			} else {
				independentSkipped++
			}
		} else if r.Passed {
			if isChain {
				chainPassed++
			} else {
				independentPassed++
			}
		} else {
			if isChain {
				chainFailed++
			} else {
				independentFailed++
			}
		}
	}
	return
}

// SummarizeResults 汇总测试结果的统计数据（总数、通过/失败/跳过、耗时）。
func SummarizeResults(results []TestResult) (total, chain, independent, passed, failed, skipped int, totalDuration time.Duration) {
	aggregated := AggregateResultsByFile(results)
	for _, r := range aggregated {
		totalDuration += r.Duration
		if IsChainTestCase(r.TestCase) {
			chain++
		} else {
			independent++
		}
		total++
		if r.TestCase.Skip {
			skipped++
		} else if r.Passed {
			passed++
		} else {
			failed++
		}
	}
	return
}

// GetChainFiles 获取所有包含链路测试用例的文件名集合。
func GetChainFiles(testCases []psv.TestCase) map[string]bool {
	byFile := groupByFile(testCases)
	chainFiles := make(map[string]bool)
	for file, cases := range byFile {
		if isChainFile(cases) {
			chainFiles[file] = true
		}
	}
	return chainFiles
}

// groupByFile 按文件名分组测试用例。
func groupByFile(testCases []psv.TestCase) map[string][]psv.TestCase {
	groups := make(map[string][]psv.TestCase)
	for _, tc := range testCases {
		groups[tc.FileName] = append(groups[tc.FileName], tc)
	}
	return groups
}

// isChainFile 判断文件中的测试用例是否包含链路测试。
func isChainFile(testCases []psv.TestCase) bool {
	for _, tc := range testCases {
		if IsChainTestCase(tc) {
			return true
		}
	}
	return false
}

// AggregateResultsByFile 按文件聚合测试结果：链路用例合并为一条记录，独立用例保持原样。
func AggregateResultsByFile(results []TestResult) []TestResult {
	byFile := make(map[string][]TestResult)
	for _, r := range results {
		byFile[r.TestCase.FileName] = append(byFile[r.TestCase.FileName], r)
	}

	var aggregated []TestResult
	for _, fileResults := range byFile {
		if len(fileResults) == 0 {
			continue
		}

		if !isChainFileByResults(fileResults) {
			aggregated = append(aggregated, fileResults...)
			continue
		}

		first := fileResults[0]
		for _, r := range fileResults {
			if IsChainTestCase(r.TestCase) {
				first = r
				break
			}
		}
		agg := TestResult{
			TestCase:  first.TestCase,
			StartTime: first.StartTime,
			EndTime:   first.EndTime,
			Passed:    true,
		}
		allSkipped := true
		for _, r := range fileResults {
			if r.EndTime.After(agg.EndTime) {
				agg.EndTime = r.EndTime
			}
			agg.Duration += r.Duration
			if !r.TestCase.Skip {
				allSkipped = false
				if !r.Passed {
					agg.Passed = false
					if agg.Error == "" {
						agg.Error = r.Error
					}
				}
			}
		}
		if allSkipped {
			agg.TestCase.Skip = true
		}
		aggregated = append(aggregated, agg)
	}
	return aggregated
}

// isChainFileByResults 根据结果集判断是否为链路文件。
func isChainFileByResults(results []TestResult) bool {
	for _, r := range results {
		if IsChainTestCase(r.TestCase) {
			return true
		}
	}
	return false
}

// isUsedAsPreCondition 检查指定 ID 是否被其他测试用例作为前置条件引用。
func isUsedAsPreCondition(tcID string) bool {
	return slices.ContainsFunc(allTestCases, func(tc psv.TestCase) bool {
		return slices.Contains(tc.Pre, tcID)
	})
}

// findTestCaseByID 根据 ID 查找测试用例。
func findTestCaseByID(id string) *psv.TestCase {
	for _, tc := range allTestCases {
		if tc.ID == id {
			return &tc
		}
	}
	return nil
}

// hasFileField 检查表单中是否包含文件上传字段（以 @ 或 file:// 开头）。
func hasFileField(form map[string]string) bool {
	for _, v := range form {
		if strings.HasPrefix(v, "@") || strings.HasPrefix(v, "file://") {
			return true
		}
	}
	return false
}

// escapePipe 转义管道分隔符，用于 CSV 报告输出。
func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// RunParallel 串行执行所有测试用例（保留接口名以支持未来并行扩展）。
func RunParallel(testCases []psv.TestCase) []TestResult {
	var results []TestResult

	for _, tc := range testCases {
		result := ExecuteTestCase(tc)
		results = append(results, result)
	}

	return results
}
