// Package testcase 提供测试用例执行和结果管理功能
// 包括 HTTP 请求执行、断言验证、变量提取和报告生成
package testcase

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"

	"pipetGo/config"
	"pipetGo/internal/assert"
	"pipetGo/internal/httpclient"
	"pipetGo/internal/logger"
	"pipetGo/internal/psv"
	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"
	"pipetGo/internal/vars"
)

// TestResult 表示测试用例执行结果
type TestResult struct {
	TestCase       psv.TestCase  // 测试用例
	Passed         bool          // 是否通过
	Error          string        // 错误信息
	Duration       time.Duration // 执行时长
	StartTime      time.Time     // 开始时间
	EndTime        time.Time     // 结束时间
	ResponseBody   string        // 响应体
	ActualStatus   int           // 实际状态码
	RequestHeaders string        // 请求头（JSON格式）
	RequestBody    string        // 请求体
	ExtractedVars  string        // 提取的变量（JSON格式）
}

// 全局变量用于存储测试结果和提取的变量
var (
	results      []TestResult              // 测试结果列表
	resultsMu    sync.Mutex                // 保护 results 的互斥锁
	globalVars   = make(map[string]string) // 全局变量存储
	globalVarsMu sync.Mutex                // 保护 globalVars 的互斥锁
)

var allTestCases []psv.TestCase

func SetAllTestCases(cases []psv.TestCase) {
	allTestCases = cases
}

func findTestCaseByID(id string) *psv.TestCase {
	for _, tc := range allTestCases {
		if tc.ID == id {
			return &tc
		}
	}
	return nil
}

func executePreConditions(preIDs []string) (TestResult, error) {
	for _, preID := range preIDs {
		preTC := findTestCaseByID(preID)
		if preTC == nil {
			errMsg := fmt.Sprintf("前置条件测试用例未找到: %s", preID)
			logger.Error(errMsg)
			return TestResult{}, fmt.Errorf(errMsg)
		}

		fmt.Printf("[前置条件] 执行: %s - %s\n", preTC.ID, preTC.Desc)
		preResult := ExecuteTestCase(*preTC)
		if !preResult.Passed {
			errMsg := fmt.Sprintf("前置条件失败: %s - %s", preID, preResult.Error)
			logger.Error(errMsg)
			return preResult, fmt.Errorf(errMsg)
		}
		fmt.Printf("[前置条件] ✅ 成功\n")
	}
	return TestResult{}, nil
}

// isUsedAsPreCondition 判断指定用例是否被其他用例作为前置条件依赖
func isUsedAsPreCondition(tcID string) bool {
	return slices.ContainsFunc(allTestCases, func(tc psv.TestCase) bool {
		return slices.Contains(tc.Pre, tcID)
	})
}

func executePostConditions(postIDs []string) {
	for _, postID := range postIDs {
		postTC := findTestCaseByID(postID)
		if postTC == nil {
			logger.Error(fmt.Sprintf("后置条件测试用例未找到: %s", postID))
			continue
		}

		fmt.Printf("[后置条件] 执行: %s - %s\n", postTC.ID, postTC.Desc)
		postResult := ExecuteTestCase(*postTC)
		if !postResult.Passed {
			fmt.Printf("[后置条件] ❌ 失败: %s\n", postResult.Error)
			logger.Warn(fmt.Sprintf("后置条件失败: %s - %s", postID, postResult.Error))
		} else {
			fmt.Printf("[后置条件] ✅ 成功\n")
		}
	}
}

func finishTestCase(tc psv.TestCase, result TestResult, startTime time.Time) TestResult {
	result.EndTime = timeutil.Now()
	result.Duration = result.EndTime.Sub(startTime)

	if result.Passed {
		logger.Info("Test passed", zap.String("id", tc.ID), zap.Duration("duration", result.Duration))
		fmt.Printf("[%s] [%s] %s ... PASS (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
		go storage.RecordExecutionTime(tc.ID, tc.Desc, tc.FileName, vars.Replace(tc.URL), result.Duration, true)
	} else {
		logger.Error("Test failed", zap.String("id", tc.ID), zap.String("error", result.Error))
		fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
		if result.Error != "" {
			fmt.Printf("            Error: %s\n", result.Error)
		}
		go storage.RecordExecutionTime(tc.ID, tc.Desc, tc.FileName, vars.Replace(tc.URL), result.Duration, false)
	}

	// 执行后置条件（无论测试成功与否都会执行）
	if len(tc.Post) > 0 {
		executePostConditions(tc.Post)
	}

	// 清理当前测试用例提取的变量（默认清理，除非设置 keep_vars=true，或被其他用例作为前置条件依赖，或是全局前置/后置条件）
	shouldCleanup := !tc.KeepVars && tc.Extract != "" && !isUsedAsPreCondition(tc.ID) && !IsSetupTestCase(tc)
	logger.Info("变量清理决策", zap.String("test", tc.ID), zap.Bool("shouldCleanup", shouldCleanup), zap.Bool("KeepVars", tc.KeepVars), zap.String("Extract", tc.Extract), zap.Bool("isUsedAsPreCondition", isUsedAsPreCondition(tc.ID)), zap.Bool("IsSetupTestCase", IsSetupTestCase(tc)))
	if shouldCleanup {
		extractParts := strings.Split(tc.Extract, ",")
		globalVarsMu.Lock()
		for _, part := range extractParts {
			part = strings.TrimSpace(part)
			if idx := strings.Index(part, "="); idx != -1 {
				varName := strings.TrimSpace(part[:idx])
				delete(globalVars, varName)
				vars.Delete(varName)
				logger.Debug("Cleaned up variable", zap.String("name", varName), zap.String("test", tc.ID))
			}
		}
		globalVarsMu.Unlock()
	}

	// 执行后延迟（如果设置了 delay_after_ms）
	if tc.DelayAfterMs > 0 {
		logger.Info("Waiting after executing test", zap.String("id", tc.ID), zap.Int("delay_after_ms", tc.DelayAfterMs))
		time.Sleep(time.Duration(tc.DelayAfterMs) * time.Millisecond)
	}

	return result
}

// ExecuteTestCase 执行单个测试用例
// tc: 测试用例
// 返回: 测试结果
func ExecuteTestCase(tc psv.TestCase) TestResult {
	startTime := timeutil.Now()

	result := TestResult{
		TestCase:  tc,
		StartTime: startTime,
	}

	logger.Info("Running test", zap.String("id", tc.ID), zap.String("desc", tc.Desc))

	// 如果标记为跳过，直接返回通过
	if tc.Skip {
		logger.Info("Skipping test", zap.String("id", tc.ID))
		result.Passed = true
		result.EndTime = timeutil.Now()
		result.Duration = result.EndTime.Sub(startTime)
		fmt.Printf("[%s] [%s] %s ... SKIP (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
		return result
	}

	// 执行前置条件（链式测试）
	if len(tc.Pre) > 0 {
		preResult, err := executePreConditions(tc.Pre)
		if err != nil {
			// 根据失败模式决定是否继续执行
			if tc.FailMode == "continue" {
				fmt.Printf("[%s] [%s] %s ... 前置条件失败但继续执行: %s\n", timeutil.FormatDateTime(timeutil.Now()), tc.ID, tc.Desc, err.Error())
			} else {
				result.Passed = false
				result.Error = err.Error()
				result.EndTime = timeutil.Now()
				result.Duration = result.EndTime.Sub(startTime)
				fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs) - 前置条件失败: %s\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds(), result.Error)
				return result
			}
		}
		_ = preResult // 忽略前置结果，继续执行当前测试
	}

	// 变量替换：将 {{var}} 替换为实际值
	logger.Info("变量替换前",
		zap.String("URL", tc.URL),
		zap.Any("Headers", tc.Headers),
		zap.String("Body", tc.Body),
		zap.String("JSON", tc.JSON))

	processedURL := vars.Replace(tc.URL)
	processedHeaders := make(map[string]string)
	for k, v := range tc.Headers {
		processedHeaders[k] = vars.Replace(v)
	}
	processedBody := vars.Replace(tc.Body)
	processedJSON := vars.Replace(tc.JSON)

	logger.Info("变量替换后",
		zap.String("processedURL", processedURL),
		zap.Any("processedHeaders", processedHeaders),
		zap.String("processedBody", processedBody),
		zap.String("processedJSON", processedJSON))
	logger.Info("当前全局变量", zap.Any("vars", vars.GetAll()))


	// 构建请求体（用于报告记录）
	var requestBody string
	if tc.JSON != "" {
		requestBody = processedJSON
	} else if tc.Body != "" {
		requestBody = processedBody
	} else if tc.Payload != "" {
		requestBody = vars.Replace(tc.Payload)
	} else if len(tc.Form) > 0 {
		formData := make(map[string]string)
		for k, v := range tc.Form {
			formData[k] = vars.Replace(v)
		}
		formJSON, _ := json.Marshal(formData)
		requestBody = string(formJSON)
	}

	// 请求头转JSON（用于报告记录）
	headersJSON, _ := json.Marshal(processedHeaders)
	result.RequestHeaders = string(headersJSON)
	result.RequestBody = requestBody

	// 创建 HTTP 请求
	req := httpclient.Client.R()
	for k, v := range processedHeaders {
		req.SetHeader(k, v)
	}

	// 设置 URL 参数
	for k, v := range tc.Params {
		req.SetQueryParam(k, vars.Replace(v))
	}

	// 处理表单数据（支持文件上传）
	if hasFileField(tc.Form) {
		formData := make(map[string]string)
		for k, v := range tc.Form {
			v = vars.Replace(v)
			if strings.HasPrefix(v, "@") || strings.HasPrefix(v, "file://") {
				filePath := strings.TrimPrefix(strings.TrimPrefix(v, "@"), "file://")
				req.SetFile(k, filePath)
			} else {
				formData[k] = v
			}
		}
		if len(formData) > 0 {
			req.SetFormData(formData)
		}
	} else if tc.JSON != "" {
		// JSON 请求体
		req.SetHeader("Content-Type", "application/json")
		req.SetBody(processedJSON)
	} else if len(tc.Form) > 0 {
		// 表单数据
		formData := make(map[string]string)
		for k, v := range tc.Form {
			formData[k] = vars.Replace(v)
		}
		req.SetHeader("Content-Type", "application/x-www-form-urlencoded")
		req.SetFormData(formData)
	} else if tc.Body != "" {
		// 原始请求体
		req.SetBody(processedBody)
	} else if tc.Payload != "" {
		// 兼容性字段
		req.SetBody(vars.Replace(tc.Payload))
	}

	var resp *resty.Response
	var err error

	// 根据 HTTP 方法执行请求
	switch tc.Method {
	case http.MethodGet:
		resp, err = req.Get(processedURL)
	case http.MethodPost:
		resp, err = req.Post(processedURL)
	case http.MethodPut:
		resp, err = req.Put(processedURL)
	case http.MethodDelete:
		resp, err = req.Delete(processedURL)
	case http.MethodPatch:
		resp, err = req.Patch(processedURL)
	case http.MethodHead:
		resp, err = req.Head(processedURL)
	default:
		err = fmt.Errorf("unsupported HTTP method: %s", tc.Method)
	}

	// 请求执行失败
	if err != nil {
		result.Error = err.Error()
		result.Passed = false
		return finishTestCase(tc, result, startTime)
	}

	result.ResponseBody = string(resp.Body())
	result.ActualStatus = resp.StatusCode()

	// 流式模式处理
	if tc.StreamMode {
		result = executeStreamAssert(tc, resp, startTime)
	} else {
		// 普通模式断言
		if tc.ExpectedStatus > 0 && resp.StatusCode() != tc.ExpectedStatus {
			result.Error = fmt.Sprintf("expected status %d, got %d", tc.ExpectedStatus, resp.StatusCode())
			result.Passed = false
			return finishTestCase(tc, result, startTime)
		}

		// 正则表达式断言
		if tc.BodyRegex != "" {
			if ok, errMsg := assert.BodyRegexMatch(result.ResponseBody, tc.BodyRegex); !ok {
				result.Error = errMsg
				result.Passed = false
				return finishTestCase(tc, result, startTime)
			}
		}

		// JSON 响应体断言
		if tc.ExpectedBody != "" {
			if ok, errMsg := assert.JSONMatch(vars.Replace(tc.ExpectedBody), result.ResponseBody, tc.MatchMode); !ok {
				result.Error = errMsg
				result.Passed = false
				return finishTestCase(tc, result, startTime)
			}
		}

	}

	// 提取变量（用于链式测试）
	if tc.Extract != "" {
		extractedVars, err := assert.ExtractVariables(result.ResponseBody, tc.Extract)
		if err == nil {
			globalVarsMu.Lock()
			for k, v := range extractedVars {
				globalVars[k] = v
				vars.Set(k, v)
			}
			globalVarsMu.Unlock()
			// 记录提取的变量到结果中
			extractedVarsJSON, _ := json.Marshal(extractedVars)
			result.ExtractedVars = string(extractedVarsJSON)
			logger.Info("Extracted variables", zap.String("id", tc.ID), zap.Any("vars", extractedVars))
		}
	}

	// 测试通过
	result.Passed = true
	return finishTestCase(tc, result, startTime)
}

// hasFileField 检查表单是否包含文件上传字段
// form: 表单数据
// 返回: 是否包含文件字段
func hasFileField(form map[string]string) bool {
	for _, v := range form {
		if strings.HasPrefix(v, "@") || strings.HasPrefix(v, "file://") {
			return true
		}
	}
	return false
}

// executeStreamAssert 执行流式响应断言
// tc: 测试用例
// resp: HTTP 响应
// startTime: 开始时间
// 返回: 测试结果
func executeStreamAssert(tc psv.TestCase, resp *resty.Response, startTime time.Time) TestResult {
	result := TestResult{
		TestCase:  tc,
		StartTime: startTime,
	}

	body := string(resp.Body())
	scanner := bufio.NewScanner(strings.NewReader(body))

	var aggregatedContent strings.Builder
	chunkCount := 0

	// 解析 SSE（Server-Sent Events）格式响应
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			chunkCount++

			var jsonData map[string]interface{}
			if json.Unmarshal([]byte(data), &jsonData) == nil {
				if choices, ok := jsonData["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						if delta, ok := choice["delta"].(map[string]interface{}); ok {
							if content, ok := delta["content"].(string); ok {
								aggregatedContent.WriteString(content)
							}
						}
					}
				}
			}
		}
	}

	// 执行流式断言
	if len(tc.StreamAssert) > 0 {
		assertConfigs := make([]assert.StreamAssertConfig, len(tc.StreamAssert))
		for i, sa := range tc.StreamAssert {
			assertConfigs[i] = assert.StreamAssertConfig{
				Kind:      sa.Kind,
				Pattern:   vars.Replace(sa.Pattern),
				MaxWaitMs: sa.MaxWaitMs,
				MinChunks: sa.MinChunks,
			}
		}

		if ok, errMsg := assert.StreamAssert(aggregatedContent.String(), chunkCount, assertConfigs); ok {
			result.Passed = true
			result.EndTime = timeutil.Now()
			result.Duration = result.EndTime.Sub(startTime)
			logger.Info("Stream assertion passed", zap.String("id", tc.ID))
			fmt.Printf("[%s] [%s] %s ... PASS (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
			return result
		} else {
			result.Error = errMsg
			result.Passed = false
			result.EndTime = timeutil.Now()
			result.Duration = result.EndTime.Sub(startTime)
			fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
			fmt.Printf("            Error: %s\n", result.Error)
			return result
		}

	}

	// 构建聚合结果
	aggregatedResult := assert.BuildAggregatedResult(aggregatedContent.String(), chunkCount)
	result.ResponseBody = aggregatedResult

	// JSON 响应体断言
	if tc.ExpectedBody != "" {
		if ok, errMsg := assert.JSONMatch(vars.Replace(tc.ExpectedBody), aggregatedResult, tc.MatchMode); !ok {
			result.Error = errMsg
			result.Passed = false
			result.EndTime = timeutil.Now()
			result.Duration = result.EndTime.Sub(startTime)
			fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
			fmt.Printf("            Error: %s\n", result.Error)
			return result
		}
	}

	result.Passed = true
	result.EndTime = timeutil.Now()
	result.Duration = result.EndTime.Sub(startTime)
	fmt.Printf("[%s] [%s] %s ... PASS (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
	return result
}

// IsChainTestCase 判断是否为链式测试用例（通过ID前缀判断）
// tc: 测试用例
// 返回: 是否为链式测试
func IsChainTestCase(tc psv.TestCase) bool {
	return strings.HasPrefix(tc.ID, "chain")
}

// IsGlobalPreCondition 判断是否为全局前置条件测试用例（通过ID前缀判断）
// tc: 测试用例
// 返回: 是否为全局前置条件
func IsGlobalPreCondition(tc psv.TestCase) bool {
	return strings.HasPrefix(tc.ID, "pre_")
}

// IsGlobalPostCondition 判断是否为全局后置条件测试用例（通过ID前缀判断）
// tc: 测试用例
// 返回: 是否为全局后置条件
func IsGlobalPostCondition(tc psv.TestCase) bool {
	return strings.HasPrefix(tc.ID, "post_")
}

// IsSetupTestCase 判断是否为环境准备类测试用例（前置/后置条件）
// tc: 测试用例
// 返回: 是否为环境准备类测试用例
func IsSetupTestCase(tc psv.TestCase) bool {
	return IsGlobalPreCondition(tc) || IsGlobalPostCondition(tc)
}

// GetTestCaseType 获取测试用例类型
// tc: 测试用例
// 返回: "chain" 或 "independent"
func GetTestCaseType(tc psv.TestCase) string {
	if IsChainTestCase(tc) {
		return "chain"
	}
	return "independent"
}

// CountStatisticalTestCases 按文件统计测试用例数
// 如果文件中存在 ID 以 chain 开头的用例，则该文件计为 1 个链式测试
// 否则按文件内的独立用例数计数
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

// CountTestSummary 统计测试用例摘要
// 返回：总用例数、链式测试文件数、独立测试用例数
// 链式文件判断标准：文件中存在 ID 以 chain 开头的用例
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

// SummarizeResultsByType 按链式/独立分类统计测试结果
// 返回：链式通过/失败/跳过数、独立通过/失败/跳过数、总耗时
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

// SummarizeResults 统计测试结果摘要
// 返回：总用例数、链式测试文件数、独立测试用例数、通过/失败/跳过数、总耗时
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

// GetChainFiles 返回所有全为链式用例的文件名集合
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

// groupByFile 按文件名分组测试用例
func groupByFile(testCases []psv.TestCase) map[string][]psv.TestCase {
	groups := make(map[string][]psv.TestCase)
	for _, tc := range testCases {
		groups[tc.FileName] = append(groups[tc.FileName], tc)
	}
	return groups
}

// isChainFile 判断文件是否为链式测试文件
// 只要文件中存在 ID 以 chain 开头的用例，即视为链式测试文件
func isChainFile(testCases []psv.TestCase) bool {
	for _, tc := range testCases {
		if IsChainTestCase(tc) {
			return true
		}
	}
	return false
}

// AggregateResultsByFile 按文件聚合测试结果
// 如果文件中存在 ID 以 chain 开头的用例，则整体计为 1 个链式结果：任一非跳过步骤失败则整体失败
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

		// 文件内存在 chain 用例则视为链式文件，整体聚合为 1 个结果
		if !isChainFileByResults(fileResults) {
			aggregated = append(aggregated, fileResults...)
			continue
		}

		first := fileResults[0]
		// 链式文件的代表用例应优先选择第一个 chain 用例，确保后续按 ID 判断类型正确
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

// isChainFileByResults 判断文件是否为链式测试文件（基于结果）
func isChainFileByResults(results []TestResult) bool {
	for _, r := range results {
		if IsChainTestCase(r.TestCase) {
			return true
		}
	}
	return false
}

// FilterIndependentTests 过滤独立测试用例（ID不以chain_开头的）
// testCases: 测试用例列表
// 返回: 独立测试用例列表
func FilterIndependentTests(testCases []psv.TestCase) []psv.TestCase {
	var independent []psv.TestCase
	for _, tc := range testCases {
		if !IsChainTestCase(tc) {
			independent = append(independent, tc)
		}
	}
	return independent
}

// FilterChainTests 过滤链式测试用例（ID以chain_开头的）
// testCases: 测试用例列表
// 返回: 链式测试用例列表
func FilterChainTests(testCases []psv.TestCase) []psv.TestCase {
	var chain []psv.TestCase
	for _, tc := range testCases {
		if IsChainTestCase(tc) {
			chain = append(chain, tc)
		}
	}
	return chain
}

// FilterByTags 根据标签过滤测试用例
// testCases: 测试用例列表
// tags: 标签列表
// 返回: 过滤后的测试用例列表
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

// RunParallel 执行测试用例（串行模式，适用于资源受限设备）
// testCases: 测试用例列表
// 返回: 测试结果列表
func RunParallel(testCases []psv.TestCase) []TestResult {
	var results []TestResult

	for _, tc := range testCases {
		result := ExecuteTestCase(tc)
		results = append(results, result)
	}

	return results
}

// GetResults 获取所有测试结果
// 返回: 测试结果列表
func GetResults() []TestResult {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	return append([]TestResult{}, results...)
}

// AddResult 添加测试结果
// result: 测试结果
func AddResult(result TestResult) {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	results = append(results, result)
}

// GenerateReport 生成测试报告
// results: 测试结果列表
// 返回: 全部报告内容, 错误报告内容
func GenerateReport(results []TestResult) (string, string) {
	var allReport strings.Builder
	var errorReport strings.Builder

	// 使用新的报告格式（status放在最前面，简化状态表示，diff放在最后）
	header := "status|id|desc|method|url|request_headers|request_body|tags|duration_s|expect_status|actual_status|actual_body|expect_body|pre_conditions|post_conditions|extracted_vars|start_time|end_time|diff\n"
	allReport.WriteString(header)

	// 先收集失败的测试用例行
	var failedLines []string

	for _, result := range results {
		// 确定测试状态（简化表示：P=通过，F=失败，S=跳过）
		status := "P"
		if result.TestCase.Skip {
			status = "S"
		} else if !result.Passed {
			status = "F"
		}

		// 标签列表转字符串
		tags := strings.Join(result.TestCase.Tags, ",")

		// 前置条件转字符串
		preConditions := strings.Join(result.TestCase.Pre, ";")

		// 后置条件转字符串
		postConditions := strings.Join(result.TestCase.Post, ";")

		// 差异信息（错误信息）
		diff := result.Error

		processedURL := vars.Replace(result.TestCase.URL)
		processedExpectedBody := vars.Replace(result.TestCase.ExpectedBody)

		// 格式化东八区时间
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

		// 收集失败的测试用例行
		if !result.Passed && !result.TestCase.Skip {
			failedLines = append(failedLines, line)
		}
	}

	// 只有当有失败的测试用例时才生成错误报告
	if len(failedLines) > 0 {
		errorReport.WriteString(header)
		for _, line := range failedLines {
			errorReport.WriteString(line)
		}
	}

	return allReport.String(), errorReport.String()
}

// escapePipe 转义管道符
// s: 输入字符串
// 返回: 转义后的字符串
func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// SaveReports 保存测试报告到文件

// allReport: 全部报告内容
// errorReport: 错误报告内容
// 返回: 全部报告路径, 错误报告路径
func SaveReports(allReport, errorReport string, timestamp ...string) (string, string) {
	// 如果没有提供时间戳，使用当前时间
	ts := timeutil.FormatCompact(timeutil.Now())
	if len(timestamp) > 0 && timestamp[0] != "" {
		ts = timestamp[0]
	}

	reportDir := config.AppConfig.Test.ReportDir

	// 创建报告目录
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		logger.Error("Failed to create report directory", zap.Error(err))
		return "", ""
	}

	// 保存全部报告（使用 .csv 扩展名，内容保持管道符分隔）
	allPath := fmt.Sprintf("%s/report_%s.csv", reportDir, ts)
	if err := os.WriteFile(allPath, []byte(allReport), 0644); err != nil {
		logger.Error("Failed to save report", zap.Error(err))
	}

	// 保存错误报告（如果有失败的测试）
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

// PrintSummary 打印测试摘要（排除全局前置/后置条件）
// 同一个文件中存在 chain_ 用例的按 1 个链式用例聚合统计
// results: 测试结果列表
func PrintSummary(results []TestResult) {
	var passed, failed, skipped int
	var setupPassed, setupFailed int
	var totalDuration time.Duration

	// 按文件聚合链式结果后再统计
	aggregated := AggregateResultsByFile(results)

	// 统计结果
	for _, r := range aggregated {
		totalDuration += r.Duration

		// 排除全局前置/后置条件
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

	// 打印汇总信息
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║              pipet 接口测试                          ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("测试用例: %d 通过, %d 失败, %d 跳过\n", passed, failed, skipped)
	if setupPassed+setupFailed > 0 {
		fmt.Printf("环境准备: %d 通过, %d 失败\n", setupPassed, setupFailed)
	}
	fmt.Printf("总时长: %.3fs\n", totalDuration.Seconds())
}