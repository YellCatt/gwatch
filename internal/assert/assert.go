// Package assert 提供接口测试断言相关的工具函数，包括响应体压缩、正则匹配、
// JSON 精确/子集比较、流式断言以及变量提取等能力。
package assert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"gwatch/internal/logger"
)

// CompactBody 将响应体压缩为单行。
// 如果是合法 JSON，使用 json.Compact 压缩；否则移除换行符和回车符。
func CompactBody(body string) string {
	if body == "" {
		return body
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(body)); err != nil {
		body = strings.ReplaceAll(body, "\r\n", " ")
		body = strings.ReplaceAll(body, "\n", " ")
		body = strings.ReplaceAll(body, "\r", "")
		return strings.TrimSpace(body)
	}
	return buf.String()
}

// BodyRegexMatch 使用正则表达式匹配响应体内容，支持 "!" 前缀取反。
func BodyRegexMatch(body string, pattern string) (bool, string) {
	if pattern == "" {
		return true, ""
	}

	negate := false
	if strings.HasPrefix(pattern, "!") {
		negate = true
		pattern = pattern[1:]
	}

	matched, err := regexp.MatchString(pattern, body)
	if err != nil {
		return false, fmt.Sprintf("invalid regex pattern: %s", err.Error())
	}

	if negate {
		matched = !matched
	}

	if !matched {
		if negate {
			return false, fmt.Sprintf("body should NOT contain pattern '%s'", pattern)
		}
		return false, fmt.Sprintf("body should contain pattern '%s'", pattern)
	}

	return true, ""
}

// JSONMatch 比较期望 JSON 与实际响应 JSON，支持 exact 和 subset 两种匹配模式。
func JSONMatch(expected, actual string, matchMode string) (bool, string) {
	if expected == "" {
		return true, ""
	}

	expectedData := gjson.Parse(expected)
	actualData := gjson.Parse(actual)

	if matchMode == "subset" {
		return jsonSubsetMatch(expectedData, actualData)
	}

	return jsonExactMatch(expectedData, actualData)
}

// jsonExactMatch 精确比较两个 JSON 对象（键数和值必须完全一致）。
func jsonExactMatch(expected, actual gjson.Result) (bool, string) {
	if !expected.IsObject() || !actual.IsObject() {
		return compareValues(expected, actual)
	}

	expectedMap := expected.Map()
	actualMap := actual.Map()

	if len(expectedMap) != len(actualMap) {
		return false, fmt.Sprintf("expected %d keys, got %d keys", len(expectedMap), len(actualMap))
	}

	for key, expectedVal := range expectedMap {
		actualVal, exists := actualMap[key]
		if !exists {
			return false, fmt.Sprintf("missing key: %s", key)
		}

		if ok, err := compareValues(expectedVal, actualVal); !ok {
			return false, fmt.Sprintf("key '%s': %s", key, err)
		}
	}

	return true, ""
}

// jsonSubsetMatch 子集匹配比较：期望 JSON 的所有键都必须存在于实际 JSON 中，但允许实际 JSON 有额外键。
func jsonSubsetMatch(expected, actual gjson.Result) (bool, string) {
	if !expected.IsObject() || !actual.IsObject() {
		return compareValues(expected, actual)
	}

	expectedMap := expected.Map()
	actualMap := actual.Map()

	for key, expectedVal := range expectedMap {
		actualVal, exists := actualMap[key]

		if expectedVal.Str == "{{not_exists}}" {
			if exists {
				return false, fmt.Sprintf("key '%s' should NOT exist", key)
			}
			continue
		}

		if !exists {
			return false, fmt.Sprintf("missing key: %s", key)
		}

		if ok, err := compareValues(expectedVal, actualVal); !ok {
			return false, fmt.Sprintf("key '%s': %s", key, err)
		}
	}

	return true, ""
}

// compareValues 比较两个 gjson 值，支持特殊指令（{{skip}}、{{regex:...}}、{{not_regex:...}}、{{not_exists}}）。
func compareValues(expected, actual gjson.Result) (bool, string) {
	expectedStr := expected.Str
	expectedRaw := expected.Raw

	// 检查字符串类型的期望值
	if expected.Type == gjson.String {
		if expectedStr == "{{skip}}" {
			return true, ""
		}

		if strings.HasPrefix(expectedStr, "{{regex:") && strings.HasSuffix(expectedStr, "}}") {
			pattern := expectedStr[8 : len(expectedStr)-2]
			pattern = fixRegexEscapes(pattern)
			pattern, err := validateRegexPattern(pattern)
			if err != nil {
				return false, fmt.Sprintf("invalid regex: %s", err.Error())
			}
			matched, err := regexp.MatchString(pattern, actual.String())
			if err != nil {
				return false, fmt.Sprintf("invalid regex: %s", err.Error())
			}
			if !matched {
				return false, fmt.Sprintf("value '%s' does not match regex '%s'", actual.String(), pattern)
			}
			return true, ""
		}

		if strings.HasPrefix(expectedStr, "{{not_regex:") && strings.HasSuffix(expectedStr, "}}") {
			pattern := expectedStr[12 : len(expectedStr)-2]
			pattern = fixRegexEscapes(pattern)
			pattern, err := validateRegexPattern(pattern)
			if err != nil {
				return false, fmt.Sprintf("invalid regex: %s", err.Error())
			}
			matched, err := regexp.MatchString(pattern, actual.String())
			if err != nil {
				return false, fmt.Sprintf("invalid regex: %s", err.Error())
			}
			if matched {
				return false, fmt.Sprintf("value '%s' should NOT match regex '%s'", actual.String(), pattern)
			}
			return true, ""
		}
	}

	// 检查非字符串类型的期望值（如数字、布尔等）
	if expectedRaw == "{{skip}}" {
		return true, ""
	}

	if strings.HasPrefix(expectedRaw, "{{regex:") && strings.HasSuffix(expectedRaw, "}}") {
		pattern := expectedRaw[8 : len(expectedRaw)-2]
		pattern = fixRegexEscapes(pattern)
		pattern, err := validateRegexPattern(pattern)
		if err != nil {
			return false, fmt.Sprintf("invalid regex: %s", err.Error())
		}
		matched, err := regexp.MatchString(pattern, actual.String())
		if err != nil {
			return false, fmt.Sprintf("invalid regex: %s", err.Error())
		}
		if !matched {
			return false, fmt.Sprintf("value '%s' does not match regex '%s'", actual.String(), pattern)
		}
		return true, ""
	}

	if strings.HasPrefix(expectedRaw, "{{not_regex:") && strings.HasSuffix(expectedRaw, "}}") {
		pattern := expectedRaw[12 : len(expectedRaw)-2]
		pattern = fixRegexEscapes(pattern)
		pattern, err := validateRegexPattern(pattern)
		if err != nil {
			return false, fmt.Sprintf("invalid regex: %s", err.Error())
		}
		matched, err := regexp.MatchString(pattern, actual.String())
		if err != nil {
			return false, fmt.Sprintf("invalid regex: %s", err.Error())
		}
		if matched {
			return false, fmt.Sprintf("value '%s' should NOT match regex '%s'", actual.String(), pattern)
		}
		return true, ""
	}

	// 常规类型和值比较
	if expected.Type != actual.Type {
		return false, fmt.Sprintf("type mismatch: expected %s, got %s", expected.Type, actual.Type)
	}

	if expectedStr != actual.Str {
		return false, fmt.Sprintf("value mismatch: expected '%s', got '%s'", expectedStr, actual.Str)
	}

	return true, ""
}

// StreamAssert 执行流式断言，只要有一个断言匹配成功即返回 true。
func StreamAssert(aggregatedContent string, chunkCount int, asserts []StreamAssertConfig) (bool, string) {
	for _, sa := range asserts {
		if ok, _ := checkStreamAssert(aggregatedContent, chunkCount, sa); ok {
			return true, ""
		}
	}
	return false, "no stream assertion matched"
}

// StreamAssertConfig 流式断言配置结构体。
type StreamAssertConfig struct {
	Kind      string `json:"kind"`
	Pattern   string `json:"pattern"`
	MaxWaitMs int    `json:"max_wait_ms"`
	MinChunks int    `json:"min_chunks"`
}

// checkStreamAssert 检查单条流式断言规则是否匹配，支持 contains、regex、json_path 三种类型。
func checkStreamAssert(aggregatedContent string, chunkCount int, sa StreamAssertConfig) (bool, string) {
	if chunkCount < sa.MinChunks {
		return false, fmt.Sprintf("need at least %d chunks, got %d", sa.MinChunks, chunkCount)
	}

	switch sa.Kind {
	case "contains":
		if strings.Contains(aggregatedContent, sa.Pattern) {
			return true, ""
		}
		return false, fmt.Sprintf("aggregated content does not contain '%s'", sa.Pattern)

	case "regex":
		matched, err := regexp.MatchString(sa.Pattern, aggregatedContent)
		if err != nil {
			return false, fmt.Sprintf("invalid regex: %s", err.Error())
		}
		if matched {
			return true, ""
		}
		return false, fmt.Sprintf("aggregated content does not match regex '%s'", sa.Pattern)

	case "json_path":
		result := gjson.Get(aggregatedContent, sa.Pattern)
		if result.Exists() {
			return true, ""
		}
		return false, fmt.Sprintf("JSON path '%s' not found in aggregated content", sa.Pattern)

	default:
		return false, fmt.Sprintf("unknown stream assert kind: %s", sa.Kind)
	}
}

// ExtractVariables 从响应体中提取变量，支持格式 "key=jsonpath"，多个变量用逗号分隔。
func ExtractVariables(responseBody string, extractExpr string) (map[string]string, error) {
	if extractExpr == "" {
		return nil, nil
	}

	result := make(map[string]string)
	parts := strings.Split(extractExpr, ",")

	logger.Info("开始提取变量", zap.String("extractExpr", extractExpr), zap.String("responseBody", responseBody))

	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			logger.Info("extract 表达式格式错误，跳过", zap.String("part", part))
			continue
		}

		key := strings.TrimSpace(kv[0])
		path := strings.TrimSpace(kv[1])
		// 兼容 JSONPath 风格的 $. 前缀；gjson 标准路径不需要 $
		path = strings.TrimPrefix(path, "$.")

		value := gjson.Get(responseBody, path)
		if value.Exists() {
			result[key] = value.String()
			logger.Info("变量提取成功", zap.String("key", key), zap.String("path", path), zap.String("value", maskValue(value.String())))
		} else {
			logger.Info("变量提取失败，路径不存在", zap.String("key", key), zap.String("path", path))
		}
	}

	logger.Info("变量提取完成", zap.Any("result", result))
	return result, nil
}

// maskValue 对长度较长的值做掩码，避免日志泄露完整 token
func maskValue(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:6] + "***" + s[len(s)-6:]
}

// BuildAggregatedResult 构建包含聚合内容和块数的 JSON 结果字符串。
func BuildAggregatedResult(aggregatedContent string, chunkCount int) string {
	result := map[string]interface{}{
		"aggregated_content": aggregatedContent,
		"chunk_count":        chunkCount,
	}
	data, _ := json.Marshal(result)
	return string(data)
}

// fixRegexEscapes 修复正则表达式中丢失的反斜杠
// 当用户在 PSV 文件中写 {{regex:\d+}} 时，经过 JSON 解析后 \d 会变成 d
// 此函数自动检测并修复常见的正则表达式转义序列
func fixRegexEscapes(pattern string) string {
	if pattern == "" {
		return pattern
	}

	var result strings.Builder
	i := 0

	for i < len(pattern) {
		c := pattern[i]

		// 如果当前字符是反斜杠，直接保留并跳过下一个字符
		if c == '\\' && i+1 < len(pattern) {
			result.WriteByte(c)
			result.WriteByte(pattern[i+1])
			i += 2
			continue
		}

		// 检查是否是需要转义的字符（正则表达式中的特殊字符）
		switch c {
		case 'd', 'D', 'w', 'W', 's', 'S', 'b', 'B', 'n', 't', 'r':
			result.WriteByte('\\')
		}

		result.WriteByte(c)
		i++
	}

	return result.String()
}

// validateRegexPattern 验证正则表达式模式是否有效
// 如果模式以重复操作符开头，自动添加 . 作为前缀
func validateRegexPattern(pattern string) (string, error) {
	if pattern == "" {
		return "", fmt.Errorf("正则模式为空")
	}

	// 检查模式是否以重复操作符开头
	firstChar := pattern[0]
	if firstChar == '*' || firstChar == '+' || firstChar == '?' || firstChar == '{' {
		// 如果以重复操作符开头，在前面添加 . 匹配任意字符
		pattern = "." + pattern
	}

	// 检查模式中是否有孤立的重复操作符（前面没有有效字符）
	for i := 1; i < len(pattern); i++ {
		c := pattern[i]
		if c == '*' || c == '+' || c == '?' {
			prevChar := pattern[i-1]
			// 如果前一个字符也是特殊字符，需要处理
			if prevChar == '(' || prevChar == '[' || prevChar == '|' || prevChar == '^' || prevChar == '$' {
				// 在重复操作符前插入 .
				pattern = pattern[:i] + "." + pattern[i:]
				i++ // 跳过新插入的字符
			}
		}
	}

	return pattern, nil
}
