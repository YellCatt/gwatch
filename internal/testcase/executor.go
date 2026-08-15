package testcase

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"

	"gwatch/internal/assert"
	"gwatch/internal/httpclient"
	"gwatch/internal/logger"
	"gwatch/internal/psv"
	"gwatch/internal/storage"
	"gwatch/internal/timeutil"
	"gwatch/internal/vars"
)

// executePreConditions 按顺序执行前置条件测试用例，任一失败则返回错误。
func executePreConditions(preIDs []string) (TestResult, error) {
	for _, preID := range preIDs {
		preTC := findTestCaseByID(preID)
		if preTC == nil {
			errMsg := fmt.Sprintf("前置条件测试用例未找到: %s", preID)
			logger.Warn(errMsg)
			return TestResult{}, fmt.Errorf(errMsg)
		}

		fmt.Printf("[前置条件] 执行: %s - %s\n", preTC.ID, preTC.Desc)
		preResult := ExecuteTestCase(*preTC)
		if !preResult.Passed {
			errMsg := fmt.Sprintf("前置条件失败: %s - %s", preID, preResult.Error)
			logger.Warn(errMsg)
			return preResult, fmt.Errorf(errMsg)
		}
		fmt.Printf("[前置条件] ✅ 成功\n")
	}
	return TestResult{}, nil
}

// executePostConditions 按顺序执行后置条件测试用例，失败仅记录警告不中断。
func executePostConditions(postIDs []string) {
	for _, postID := range postIDs {
		postTC := findTestCaseByID(postID)
		if postTC == nil {
			logger.Warn(fmt.Sprintf("后置条件测试用例未找到: %s", postID))
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

// finishTestCase 完成测试用例执行：计算耗时、记录执行时间、执行后置条件、清理变量、等待延迟。
func finishTestCase(tc psv.TestCase, result TestResult, startTime time.Time) TestResult {
	result.EndTime = timeutil.Now()
	result.Duration = result.EndTime.Sub(startTime)

	if result.Passed {
		logger.Info("Test passed", zap.String("id", tc.ID), zap.Duration("duration", result.Duration))
		fmt.Printf("[%s] [%s] %s ... PASS (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
		go storage.RecordExecutionTime(tc.ID, tc.Desc, tc.FileName, vars.Replace(tc.URL), result.Duration, true)
	} else {
		logger.Warn("Test failed", zap.String("id", tc.ID), zap.String("error", result.Error))
		fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
		if result.Error != "" {
			fmt.Printf("            Error: %s\n", result.Error)
		}
		go storage.RecordExecutionTime(tc.ID, tc.Desc, tc.FileName, vars.Replace(tc.URL), result.Duration, false)
	}

	if len(tc.Post) > 0 {
		executePostConditions(tc.Post)
	}

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

	if tc.DelayAfterMs > 0 {
		logger.Info("Waiting after executing test", zap.String("id", tc.ID), zap.Int("delay_after_ms", tc.DelayAfterMs))
		time.Sleep(time.Duration(tc.DelayAfterMs) * time.Millisecond)
	}

	return result
}

// ExecuteTestCase 执行单个测试用例：处理前置条件、发送 HTTP 请求、校验响应、提取变量、调用 finishTestCase。
func ExecuteTestCase(tc psv.TestCase) TestResult {
	startTime := timeutil.Now()

	result := TestResult{
		TestCase:  tc,
		StartTime: startTime,
	}

	logger.Info("Running test", zap.String("id", tc.ID), zap.String("desc", tc.Desc))

	if tc.Skip {
		logger.Info("Skipping test", zap.String("id", tc.ID))
		result.Passed = true
		result.EndTime = timeutil.Now()
		result.Duration = result.EndTime.Sub(startTime)
		fmt.Printf("[%s] [%s] %s ... SKIP (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
		return result
	}

	if len(tc.Pre) > 0 {
		preResult, err := executePreConditions(tc.Pre)
		if err != nil {
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
		_ = preResult
	}

	logger.Info("变量替换前",
		zap.String("URL", tc.URL),
		zap.Any("Headers", tc.Headers),
		zap.String("Body", tc.Body),
		zap.String("JSON", tc.JSON))

	processedURL := vars.Replace(tc.URL)
	result.ProcessedURL = processedURL
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

	headersJSON, _ := json.Marshal(processedHeaders)
	result.RequestHeaders = string(headersJSON)
	result.RequestBody = requestBody

	req := httpclient.Client.R()
	for k, v := range processedHeaders {
		req.SetHeader(k, v)
	}

	for k, v := range tc.Params {
		req.SetQueryParam(k, vars.Replace(v))
	}

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
		req.SetHeader("Content-Type", "application/json")
		req.SetBody(processedJSON)
	} else if len(tc.Form) > 0 {
		formData := make(map[string]string)
		for k, v := range tc.Form {
			formData[k] = vars.Replace(v)
		}
		req.SetHeader("Content-Type", "application/x-www-form-urlencoded")
		req.SetFormData(formData)
	} else if tc.Body != "" {
		req.SetBody(processedBody)
	} else if tc.Payload != "" {
		req.SetBody(vars.Replace(tc.Payload))
	}

	var resp *resty.Response
	var err error

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

	if err != nil {
		result.Error = err.Error()
		result.Passed = false
		return finishTestCase(tc, result, startTime)
	}

	result.ResponseBody = assert.CompactBody(string(resp.Body()))
	result.ActualStatus = resp.StatusCode()

	if tc.StreamMode {
		result = executeStreamAssert(tc, resp, startTime)
	} else {
		if tc.ExpectedStatus > 0 && resp.StatusCode() != tc.ExpectedStatus {
			result.Error = fmt.Sprintf("expected status %d, got %d", tc.ExpectedStatus, resp.StatusCode())
			result.Passed = false
			return finishTestCase(tc, result, startTime)
		}

		if tc.BodyRegex != "" {
			if ok, errMsg := assert.BodyRegexMatch(result.ResponseBody, tc.BodyRegex); !ok {
				result.Error = errMsg
				result.Passed = false
				return finishTestCase(tc, result, startTime)
			}
		}

		if tc.ExpectedBody != "" {
			expectedBody := assert.CompactBody(vars.Replace(tc.ExpectedBody))
			if ok, errMsg := assert.JSONMatch(expectedBody, result.ResponseBody, tc.MatchMode); !ok {
				result.Error = errMsg
				result.Passed = false
				return finishTestCase(tc, result, startTime)
			}
		}

	}

	if tc.Extract != "" {
		extractedVars, err := assert.ExtractVariables(result.ResponseBody, tc.Extract)
		if err == nil {
			globalVarsMu.Lock()
			for k, v := range extractedVars {
				globalVars[k] = v
				vars.Set(k, v)
				if IsGlobalPreCondition(tc) {
					vars.MarkAsGlobalPre(k)
				}
			}
			globalVarsMu.Unlock()
			extractedVarsJSON, _ := json.Marshal(extractedVars)
			result.ExtractedVars = string(extractedVarsJSON)
			logger.Info("Extracted variables", zap.String("id", tc.ID), zap.Any("vars", extractedVars))
		}
	}

	result.Passed = true
	return finishTestCase(tc, result, startTime)
}

// executeStreamAssert 执行流式断言：解析 SSE 响应中的数据块，聚合内容后进行断言匹配。
func executeStreamAssert(tc psv.TestCase, resp *resty.Response, startTime time.Time) TestResult {
	result := TestResult{
		TestCase:  tc,
		StartTime: startTime,
	}

	body := string(resp.Body())
	scanner := bufio.NewScanner(strings.NewReader(body))

	var aggregatedContent strings.Builder
	chunkCount := 0

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

	aggregatedResult := assert.CompactBody(assert.BuildAggregatedResult(aggregatedContent.String(), chunkCount))
	result.ResponseBody = aggregatedResult

	if tc.ExpectedBody != "" {
		expectedBody := assert.CompactBody(vars.Replace(tc.ExpectedBody))
		if ok, errMsg := assert.JSONMatch(expectedBody, aggregatedResult, tc.MatchMode); !ok {
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
