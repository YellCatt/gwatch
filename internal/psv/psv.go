// Package psv 提供 PSV（管道分隔值）文件解析功能
// 支持解析 .psv 和 .csv 文件，使用管道符 | 作为分隔符
package psv

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
)

// StreamAssert 表示流式断言配置
type StreamAssert struct {
	Kind      string `json:"kind"`        // 断言类型: contains/regex/json_path
	Pattern   string `json:"pattern"`     // 断言模式
	MaxWaitMs int    `json:"max_wait_ms"` // 最大等待时间（毫秒）
	MinChunks int    `json:"min_chunks"`  // 最小块数
}

// TestCase 表示测试用例
type TestCase struct {
	ID                 string            `mapstructure:"id"`                 // 测试用例唯一标识
	Name               string            `mapstructure:"name"`               // 测试用例名称
	FileName           string            `mapstructure:"file_name"`          // 所属文件名（用于数据库记录）
	Skip               bool              `mapstructure:"skip"`               // 是否跳过
	SkipReason         string            `mapstructure:"skip_reason"`        // 跳过原因
	Desc               string            `mapstructure:"desc"`               // 测试用例描述
	Method             string            `mapstructure:"method"`             // HTTP方法: GET/POST/PUT/DELETE/PATCH/HEAD
	URL                string            `mapstructure:"url"`                // 请求URL
	Endpoint           string            `mapstructure:"endpoint"`           // API端点
	Headers            map[string]string `mapstructure:"headers"`            // 请求头
	Params             map[string]string `mapstructure:"params"`             // URL参数
	Form               map[string]string `mapstructure:"form"`               // 表单数据
	JSON               string            `mapstructure:"json"`               // JSON请求体
	Body               string            `mapstructure:"body"`               // 原始请求体
	Payload            string            `mapstructure:"payload"`            // 兼容性字段
	ExpectedStatus     int               `mapstructure:"expected_status"`    // 期望状态码
	ExpectedCode       int               `mapstructure:"expected_code"`      // 期望状态码（兼容字段）
	ExpectedBody       string            `mapstructure:"expected_body"`      // 期望响应体
	Tags               []string          `mapstructure:"tags"`               // 标签列表
	Extract            string            `mapstructure:"extract"`            // 变量提取规则
	StreamMode         bool              `mapstructure:"stream_mode"`        // 是否流式模式
	StreamAssert       []StreamAssert    `mapstructure:"stream_assert"`      // 流式断言规则
	MatchMode          string            `mapstructure:"match_mode"`         // 匹配模式: exact/subset
	BodyRegex          string            `mapstructure:"body_regex"`         // 响应体正则表达式
	Pre                []string          `mapstructure:"pre"`                // 前置条件
	Post               []string          `mapstructure:"post"`               // 后置条件
	FailMode           string            `mapstructure:"fail_mode"`          // 失败模式: stop(默认)/continue
	KeepVars           bool              `mapstructure:"keep_vars"`          // 是否保留提取的变量（默认false，即自动清理）
	DelayMs            int               `mapstructure:"delay_ms"`           // 执行前延迟时间（毫秒）
	DelayAfterMs       int               `mapstructure:"delay_after_ms"`     // 执行后延迟时间（毫秒）
	// 监控相关字段
	MonitorEnabled     bool              `mapstructure:"monitor_enabled"`    // 是否启用监控
	MonitorInterval    int               `mapstructure:"monitor_interval"`   // 监控周期（秒），默认60秒
	ResponseThreshold  int               `mapstructure:"response_threshold"` // 响应时间阈值（毫秒），超过此值告警
	AlertOnFailure     bool              `mapstructure:"alert_on_failure"`   // 失败时是否告警
	AlertOnSlow        bool              `mapstructure:"alert_on_slow"`      // 响应慢时是否告警
}

// ParseFile 解析单个PSV文件
// filePath: 文件路径
// 返回: 测试用例列表和错误信息
func ParseFile(filePath string) ([]TestCase, error) {
	file, err := os.Open(filePath)
	if err != nil {
		logger.Error("Failed to open PSV file", zap.String("path", filePath), zap.Error(err))
		return nil, err
	}
	defer file.Close()

	return parseReader(file, filePath)
}

// ParseFiles 解析多个路径中的PSV文件
// paths: 文件或目录路径列表
// 返回: 所有测试用例列表和错误信息
func ParseFiles(paths []string) ([]TestCase, error) {
	var allCases []TestCase
	for _, path := range paths {
		files, err := expandPath(path)
		if err != nil {
			logger.Warn("Failed to expand path", zap.String("path", path), zap.Error(err))
			continue
		}

		for _, file := range files {
			cases, err := ParseFile(file)
			if err != nil {
				logger.Error("Failed to parse PSV file", zap.String("path", file), zap.Error(err))
				continue
			}
			allCases = append(allCases, cases...)
		}
	}
	return allCases, nil
}

// expandPath 展开路径，支持文件和目录
// 如果是目录，递归查找所有 .psv 和 .csv 文件
// path: 路径（文件或目录）
// 返回: 文件路径列表和错误信息
func expandPath(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		files, err := doublestar.Glob(os.DirFS("."), path)
		if err != nil {
			return nil, err
		}
		return files, nil
	}

	if info.IsDir() {
		matches, err := doublestar.Glob(os.DirFS(path), "**/*.{psv,csv}")
		if err != nil {
			return nil, err
		}
		var files []string
		for _, match := range matches {
			files = append(files, filepath.Join(path, match))
		}
		return files, nil
	}
	return []string{path}, nil
}

// parseReader 从读取器解析PSV内容
// reader: 输入流
// filePath: 文件路径（用于日志）
// 返回: 测试用例列表和错误信息
func parseReader(reader io.Reader, filePath string) ([]TestCase, error) {
	var testCases []TestCase
	scanner := bufio.NewScanner(reader)
	var header []string
	lineNum := 0

	fileName := filepath.Base(filePath)

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// 跳过空行和注释行
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 第一行是表头
		if lineNum == 1 {
			header = parseLine(line)
			continue
		}

		fields := parseLine(line)
		tc, err := parseTestCase(header, fields)
		if err != nil {
			logger.Warn("Failed to parse test case", zap.String("file", filePath), zap.Int("line", lineNum), zap.Error(err))
			continue
		}
		tc.FileName = fileName
		testCases = append(testCases, tc)
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Error reading PSV file", zap.String("file", filePath), zap.Error(err))
		return nil, err
	}

	logger.Info("Successfully parsed PSV file", zap.String("path", filePath), zap.Int("count", len(testCases)))
	return testCases, nil
}

// parseLine 解析单行 PSV 内容
// 使用 encoding/csv 正确处理带引号的字段，避免 JSON 内部的双引号被误删
// line: 单行文本
// 返回: 字段数组
func parseLine(line string) []string {
	// 去掉 Windows 换行残留的 \r
	line = strings.TrimSuffix(line, "\r")

	r := csv.NewReader(strings.NewReader(line))
	r.Comma = '|'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	records, err := r.Read()
	if err != nil {
		// 退化方案：直接按管道符分割
		fields := strings.Split(line, "|")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		return fields
	}

	for i := range records {
		records[i] = strings.TrimSpace(records[i])
	}
	return records
}

// parseTestCase 将字段解析为测试用例
// header: 表头数组
// fields: 字段数组
// 返回: 测试用例和错误信息
func parseTestCase(header []string, fields []string) (TestCase, error) {
	// 从配置文件获取监控默认值
	monitorCfg := config.AppConfig.Monitor
	
	tc := TestCase{
		Headers:          make(map[string]string),
		Params:           make(map[string]string),
		Form:             make(map[string]string),
		MatchMode:        "exact",
		MonitorEnabled:   monitorCfg.Enabled,          // 从配置读取是否启用监控
		MonitorInterval:  monitorCfg.DefaultInterval,  // 从配置读取默认监控周期
		AlertOnFailure:   monitorCfg.AlertOnFailure,   // 从配置读取失败告警设置
		AlertOnSlow:      monitorCfg.AlertOnSlow,      // 从配置读取响应慢告警设置
	}

	// 遍历表头和字段，映射到测试用例结构
	for i, h := range header {
		if i >= len(fields) {
			continue
		}
		value := fields[i]
		trimmedHeader := strings.ToLower(strings.TrimSpace(h))

		switch trimmedHeader {
		case "id":
			tc.ID = value
		case "skip":
			tc.Skip = value == "1" || strings.EqualFold(value, "true")
		case "desc":
			tc.Desc = value
		case "method":
			tc.Method = strings.ToUpper(value)
		case "url":
			tc.URL = value
		case "headers":
			tc.Headers = parseKeyValueMap(value)
		case "params":
			tc.Params = parseKeyValueMap(value)
		case "form":
			tc.Form = parseKeyValueMap(value)
		case "json":
			tc.JSON = value
		case "body":
			tc.Body = value
		case "payload":
			tc.Payload = value
		case "expected_status":
			tc.ExpectedStatus = parseInt(value)
		case "expected_body":
			tc.ExpectedBody = value
		case "tags":
			tc.Tags = parseTags(value)
		case "extract":
			tc.Extract = value
		case "stream_mode":
			tc.StreamMode = value == "1" || strings.EqualFold(value, "true")
		case "stream_assert":
			if value != "" {
				json.Unmarshal([]byte(value), &tc.StreamAssert)
			}
		case "match_mode":
			tc.MatchMode = value
		case "body_regex":
			tc.BodyRegex = value
		case "pre":
			tc.Pre = parseDelimited(value, ";")
		case "post":
			tc.Post = parseDelimited(value, ";")
		case "fail_mode":
			tc.FailMode = value
		case "keep_vars":
			tc.KeepVars = value == "1" || strings.EqualFold(value, "true")
		case "delay_ms":
			tc.DelayMs = parseInt(value)
		case "delay_after_ms":
			tc.DelayAfterMs = parseInt(value)
		// 监控相关字段
		case "monitor_enabled":
			tc.MonitorEnabled = value == "1" || strings.EqualFold(value, "true")
		case "monitor_interval":
			tc.MonitorInterval = parseInt(value)
		case "response_threshold":
			tc.ResponseThreshold = parseInt(value)
		case "sla_threshold":      // 兼容旧字段名
			tc.ResponseThreshold = parseInt(value)
		case "timeout_threshold":  // 兼容旧字段名
			tc.ResponseThreshold = parseInt(value)
		case "alert_on_failure":
			tc.AlertOnFailure = value == "1" || strings.EqualFold(value, "true")
		case "alert_on_slow":
			tc.AlertOnSlow = value == "1" || strings.EqualFold(value, "true")
		case "alert_on_timeout":   // 兼容旧字段名
			tc.AlertOnSlow = value == "1" || strings.EqualFold(value, "true")
		case "alert_on_sla":       // 兼容旧字段名
			tc.AlertOnSlow = value == "1" || strings.EqualFold(value, "true")
		}
	}

	// 如果没有指定ID，自动生成
	if tc.ID == "" {
		tc.ID = generateID(tc)
	}

	// 如果没有指定method，默认为GET
	if tc.Method == "" {
		tc.Method = "GET"
	}

	return tc, nil
}

// parseKeyValueMap 解析键值对字符串
// 支持两种格式:
// 1. JSON对象格式: {"key1":"value1","key2":"value2"}
// 2. URL查询字符串格式: key1=value1&key2=value2
// str: 输入字符串
// 返回: 键值对map
func parseKeyValueMap(str string) map[string]string {
	m := make(map[string]string)
	if str == "" || str == "{}" {
		return m
	}

	// JSON对象格式 - 使用标准JSON解析
	if strings.HasPrefix(str, "{") && strings.HasSuffix(str, "}") {
		var jsonMap map[string]interface{}
		if err := json.Unmarshal([]byte(str), &jsonMap); err == nil {
			for k, v := range jsonMap {
				m[k] = fmt.Sprintf("%v", v)
			}
			return m
		}
		// 如果标准解析失败，回退到字符串分割方式
		str = strings.TrimPrefix(strings.TrimSuffix(str, "}"), "{")
		pairs := strings.Split(str, ",")
		for _, pair := range pairs {
			kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
			if len(kv) == 2 {
				key := strings.TrimSpace(strings.Trim(kv[0], "\"'"))
				value := strings.TrimSpace(strings.Trim(kv[1], "\"'"))
				m[key] = value
			}
		}
	} else if strings.Contains(str, "&") {
		// URL查询字符串格式
		pairs := strings.Split(str, "&")
		for _, pair := range pairs {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				value := strings.TrimSpace(kv[1])
				m[key] = value
			} else if len(kv) == 1 {
				m[strings.TrimSpace(kv[0])] = ""
			}
		}
	}

	return m
}

// parseTags 解析标签字符串
// str: 逗号分隔的标签字符串
// 返回: 标签数组
func parseTags(str string) []string {
	if str == "" {
		return nil
	}
	tags := strings.Split(str, ",")
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}
	return tags
}

// parseDelimited 解析分隔符分隔的字符串
// str: 输入字符串
// delimiter: 分隔符
// 返回: 分割后的字符串数组
func parseDelimited(str string, delimiter string) []string {
	if str == "" {
		return nil
	}
	parts := strings.Split(str, delimiter)
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return parts
}

// parseInt 从字符串中提取整数
// s: 输入字符串
// 返回: 提取的整数（如果没有数字返回0）
func parseInt(s string) int {
	if s == "" {
		return 0
	}
	re := regexp.MustCompile(`\d+`)
	match := re.FindString(s)
	if match == "" {
		return 0
	}
	var result int
	for _, c := range match {
		result = result*10 + int(c-'0')
	}
	return result
}

// generateID 为测试用例生成唯一ID
// 根据HTTP方法和URL生成ID
// tc: 测试用例
// 返回: 唯一ID字符串
func generateID(tc TestCase) string {
	base := strings.ToLower(tc.Method) + "_" + strings.ReplaceAll(strings.ToLower(tc.URL), "/", "_")
	base = regexp.MustCompile(`[^a-z0-9_]`).ReplaceAllString(base, "")
	return base
}