package testcase

import (
	"slices"
	"strings"
	"time"

	"gwatch/internal/psv"
)

func IsChainTestCase(tc psv.TestCase) bool {
	return strings.HasPrefix(tc.ID, "chain")
}

func IsGlobalPreCondition(tc psv.TestCase) bool {
	return strings.HasPrefix(tc.ID, "pre_")
}

func IsGlobalPostCondition(tc psv.TestCase) bool {
	return strings.HasPrefix(tc.ID, "post_")
}

func IsSetupTestCase(tc psv.TestCase) bool {
	return IsGlobalPreCondition(tc) || IsGlobalPostCondition(tc)
}

func GetTestCaseType(tc psv.TestCase) string {
	if IsChainTestCase(tc) {
		return "chain"
	}
	return "independent"
}

func FilterIndependentTests(testCases []psv.TestCase) []psv.TestCase {
	var independent []psv.TestCase
	for _, tc := range testCases {
		if !IsChainTestCase(tc) {
			independent = append(independent, tc)
		}
	}
	return independent
}

func FilterChainTests(testCases []psv.TestCase) []psv.TestCase {
	var chain []psv.TestCase
	for _, tc := range testCases {
		if IsChainTestCase(tc) {
			chain = append(chain, tc)
		}
	}
	return chain
}

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

func groupByFile(testCases []psv.TestCase) map[string][]psv.TestCase {
	groups := make(map[string][]psv.TestCase)
	for _, tc := range testCases {
		groups[tc.FileName] = append(groups[tc.FileName], tc)
	}
	return groups
}

func isChainFile(testCases []psv.TestCase) bool {
	for _, tc := range testCases {
		if IsChainTestCase(tc) {
			return true
		}
	}
	return false
}

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

func isChainFileByResults(results []TestResult) bool {
	for _, r := range results {
		if IsChainTestCase(r.TestCase) {
			return true
		}
	}
	return false
}

func isUsedAsPreCondition(tcID string) bool {
	return slices.ContainsFunc(allTestCases, func(tc psv.TestCase) bool {
		return slices.Contains(tc.Pre, tcID)
	})
}

func findTestCaseByID(id string) *psv.TestCase {
	for _, tc := range allTestCases {
		if tc.ID == id {
			return &tc
		}
	}
	return nil
}

func hasFileField(form map[string]string) bool {
	for _, v := range form {
		if strings.HasPrefix(v, "@") || strings.HasPrefix(v, "file://") {
			return true
		}
	}
	return false
}

func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func RunParallel(testCases []psv.TestCase) []TestResult {
	var results []TestResult

	for _, tc := range testCases {
		result := ExecuteTestCase(tc)
		results = append(results, result)
	}

	return results
}