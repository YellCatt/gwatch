package testcase

import (
	"encoding/json"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"

	"gwatch/internal/httpclient"
	"gwatch/internal/logger"
	"gwatch/internal/psv"
)

// LoadFromPSV 从 PSV 文件加载测试用例并注册到全局测试列表。
func LoadFromPSV(filePath string) error {
	testCases, err := psv.ParseFile(filePath)
	if err != nil {
		return err
	}

	for _, tc := range testCases {
		if tc.Skip {
			RegisterSkippedTest(tc.Name, tc.SkipReason)
			continue
		}

		testCase := tc
		RegisterTest(testCase.Name, func() error {
			return runPSVTestCase(&testCase)
		})
	}

	return nil
}

// runPSVTestCase 执行单个 PSV 测试用例（发送 HTTP 请求并校验状态码）。
func runPSVTestCase(tc *psv.TestCase) error {
	logger.Info("Running PSV test", zap.String("name", tc.Name), zap.String("endpoint", tc.Endpoint))

	req := httpclient.Client.R()

	for key, value := range tc.Headers {
		req.SetHeader(key, value)
	}

	if tc.Body != "" {
		req.SetBody(tc.Body)
	}

	var resp *resty.Response
	var err error

	switch tc.Method {
	case "GET":
		resp, err = req.Get(tc.Endpoint)
	case "POST":
		resp, err = req.Post(tc.Endpoint)
	case "PUT":
		resp, err = req.Put(tc.Endpoint)
	case "DELETE":
		resp, err = req.Delete(tc.Endpoint)
	case "PATCH":
		resp, err = req.Patch(tc.Endpoint)
	case "HEAD":
		resp, err = req.Head(tc.Endpoint)
	default:
		return ErrInvalidMethod(tc.Method)
	}

	if err != nil {
		return err
	}

	logger.Debug("Response",
		zap.Int("status", resp.StatusCode()),
		zap.String("body", string(resp.Body())))

	if tc.ExpectedCode > 0 && resp.StatusCode() != tc.ExpectedCode {
		return ErrUnexpectedStatus(tc.ExpectedCode, resp.StatusCode())
	}

	return nil
}

// ErrInvalidMethod 创建无效 HTTP 方法错误。
func ErrInvalidMethod(method string) error {
	return &TestError{Message: "invalid HTTP method: " + method}
}

// ErrUnexpectedStatus 创建状态码不匹配错误。
func ErrUnexpectedStatus(expected, actual int) error {
	return &TestError{Message: "expected status " + string(rune(expected+'0')) + ", got " + string(rune(actual+'0'))}
}

// TestError 测试错误类型，实现 error 接口。
type TestError struct {
	Message string
}

// Error 返回错误消息字符串。
func (e *TestError) Error() string {
	return e.Message
}

// PrettyPrintBody 将任意格式的响应体格式化为缩进 JSON 字符串。
func PrettyPrintBody(body interface{}) string {
	jsonBytes, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return ""
	}
	return string(jsonBytes)
}
