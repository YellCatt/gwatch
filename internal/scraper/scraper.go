// Package scraper 提供通用监控指标采集功能
// 支持通过配置文件定义监控目标，使用 JSONPath 提取指标
package scraper

import (
	"crypto/tls"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

// MetricConfig 表示单个指标配置
type MetricConfig struct {
	Name             string  `mapstructure:"name"`              // 指标名称
	Path             string  `mapstructure:"path"`              // JSONPath 路径
	Alias            string  `mapstructure:"alias"`             // 指标别名（可选）
	Unit             string  `mapstructure:"unit"`              // 单位（可选）
	Threshold        float64 `mapstructure:"threshold"`         // 阈值（可选）
	Alert            bool    `mapstructure:"alert"`             // 是否启用告警（兼容旧配置）
	Optional         bool    `mapstructure:"optional"`          // 是否为可选指标，不存在时不报错
	Scale            float64 `mapstructure:"scale"`             // 缩放因子（如 100 表示乘以100）
	AutoPercent      bool    `mapstructure:"auto_percent"`      // 自动处理百分比（值<1时乘以100）
	CompareOp        string  `mapstructure:"compare_op"`        // 比较操作符：gt(大于), lt(小于), eq(等于), ge(大于等于), le(小于等于)
	AlertLevel       string  `mapstructure:"alert_level"`       // 告警级别：info, warn, error
	Consecutive      int     `mapstructure:"consecutive"`       // 连续超过阈值多少次才告警
	WarningThreshold float64 `mapstructure:"warning_threshold"` // 警告阈值（次要告警）
}

// TargetConfig 表示监控目标配置
type TargetConfig struct {
	Name               string            `mapstructure:"name"`                 // 目标名称
	URL                string            `mapstructure:"url"`                  // 请求URL
	Method             string            `mapstructure:"method"`               // HTTP方法（GET/POST等）
	Timeout            string            `mapstructure:"timeout"`              // 超时时间（如 5s）
	Interval           int               `mapstructure:"interval"`             // 采集间隔（秒），默认10秒
	Enabled            bool              `mapstructure:"enabled"`              // 是否启用
	Headers            map[string]string `mapstructure:"headers"`              // 请求头
	Body               string            `mapstructure:"body"`                 // 请求体（POST时使用）
	InsecureSkipVerify bool              `mapstructure:"insecure_skip_verify"` // 是否跳过TLS验证
	Proxy              string            `mapstructure:"proxy"`                // 代理地址
	Metrics            []MetricConfig    `mapstructure:"metrics"`              // 指标配置列表
}

// MetricResult 表示指标采集结果
type MetricResult struct {
	Name             string  `json:"name"`
	Alias            string  `json:"alias"`
	Value            float64 `json:"value"`
	Unit             string  `json:"unit"`
	Path             string  `json:"path"`
	Success          bool    `json:"success"`
	Error            string  `json:"error,omitempty"`
	Threshold        float64 `json:"threshold,omitempty"`
	WarningThreshold float64 `json:"warning_threshold,omitempty"`
	Alert            bool    `json:"alert"`
	AlertLevel       string  `json:"alert_level,omitempty"`
	OverThreshold    bool    `json:"over_threshold"`
	IsWarning        bool    `json:"is_warning"`
}

// ScrapeResult 表示一次采集结果
type ScrapeResult struct {
	TargetName string         `json:"target_name"`
	TargetURL  string         `json:"target_url"`
	Timestamp  time.Time      `json:"timestamp"`
	Duration   time.Duration  `json:"duration"`
	StatusCode int            `json:"status_code"`
	Success    bool           `json:"success"`
	Error      string         `json:"error,omitempty"`
	Metrics    []MetricResult `json:"metrics"`
}

// Scrape 采集单个目标的指标
func Scrape(target TargetConfig) (ScrapeResult, error) {
	result := ScrapeResult{
		TargetName: target.Name,
		TargetURL:  target.URL,
		Timestamp:  timeutil.Now(),
		Success:    false,
	}

	// 设置默认值
	if target.Method == "" {
		target.Method = "GET"
	}
	if target.Interval == 0 {
		target.Interval = 10
	}

	// 解析超时时间
	timeout := 5 * time.Second
	if target.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(target.Timeout)
		if err != nil {
			logger.Warn("解析超时时间失败，使用默认值", zap.String("timeout", target.Timeout), zap.Error(err))
			timeout = 5 * time.Second
		}
	}

	// 创建 HTTP 客户端
	client := resty.New()
	client.SetTimeout(timeout)

	if target.InsecureSkipVerify {
		client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	}

	if target.Proxy != "" {
		client.SetProxy(target.Proxy)
	}

	startTime := time.Now()

	// 发送请求
	req := client.R().SetHeaders(target.Headers)
	if target.Body != "" {
		req.SetBody(target.Body)
	}

	resp, err := req.Execute(target.Method, target.URL)
	result.Duration = time.Since(startTime)

	if err != nil {
		result.Error = fmt.Sprintf("请求失败: %v", err)
		logger.Error(result.Error, zap.String("target", target.Name))
		return result, fmt.Errorf(result.Error)
	}

	result.StatusCode = resp.StatusCode()

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		result.Error = fmt.Sprintf("HTTP 状态码异常: %d", resp.StatusCode())
		logger.Error(result.Error, zap.String("target", target.Name))
		return result, fmt.Errorf(result.Error)
	}

	// 使用 gjson 提取指标（直接使用字符串，无需先解析为 interface{}）
	result.Metrics = extractMetrics(string(resp.Body()), target.Metrics)
	result.Success = true

	applyConsecutiveAlerts(target.Name, target.Metrics, result.Metrics)

	logger.Info("采集完成",
		zap.String("target", target.Name),
		zap.Int("metrics", len(result.Metrics)),
		zap.Duration("duration", result.Duration))

	return result, nil
}

// extractMetrics 使用 gjson 提取指标
func extractMetrics(jsonStr string, metrics []MetricConfig) []MetricResult {
	var results []MetricResult

	for _, metric := range metrics {
		result := MetricResult{
			Name:      metric.Name,
			Alias:     metric.Alias,
			Unit:      metric.Unit,
			Path:      metric.Path,
			Threshold: metric.Threshold,
			Alert:     metric.Alert,
			Success:   false,
		}

		gjsonPath := normalizePath(metric.Path)
		gjsonResult := gjson.Get(jsonStr, gjsonPath)
		if !gjsonResult.Exists() {
			// 如果是可选指标，跳过不记录
			if metric.Optional {
				logger.Debug("可选指标不存在，跳过",
					zap.String("metric", metric.Name),
					zap.String("path", metric.Path))
				continue
			}
			result.Error = fmt.Sprintf("路径不存在: %s", metric.Path)
			logger.Warn("指标提取失败",
				zap.String("metric", metric.Name),
				zap.String("path", metric.Path),
				zap.String("error", result.Error))
			results = append(results, result)
			continue
		}

		// 获取值
		val := gjsonResult.Value()

		// 转换为 float64
		floatVal, err := toFloat(val)
		if err != nil {
			result.Error = fmt.Sprintf("类型转换失败: %v", err)
			logger.Warn("指标转换失败",
				zap.String("metric", metric.Name),
				zap.String("error", result.Error))
			results = append(results, result)
			continue
		}

		// 自动百分比处理：如果值小于1且单位是%，自动乘以100
		if metric.AutoPercent || (metric.Unit == "%" && floatVal < 1 && floatVal >= 0) {
			floatVal = floatVal * 100
			logger.Debug("自动百分比转换",
				zap.String("metric", metric.Name),
				zap.Float64("before", floatVal/100),
				zap.Float64("after", floatVal))
		}

		// 应用缩放因子
		if metric.Scale != 0 && metric.Scale != 1 {
			floatVal = floatVal * metric.Scale
			logger.Debug("应用缩放因子",
				zap.String("metric", metric.Name),
				zap.Float64("scale", metric.Scale),
				zap.Float64("value", floatVal))
		}

		result.Value = floatVal
		result.Success = true
		result.Threshold = metric.Threshold
		result.WarningThreshold = metric.WarningThreshold

		// 检查告警条件
		checkAlertConditions(metric, floatVal, &result)

		results = append(results, result)
	}

	return results
}

// checkAlertConditions 检查告警条件
func checkAlertConditions(metric MetricConfig, value float64, result *MetricResult) {
	// 如果没有配置阈值或未启用告警，直接返回
	if (metric.Threshold <= 0 && metric.WarningThreshold <= 0) || !metric.Alert {
		return
	}

	// 默认比较操作符为大于
	compareOp := metric.CompareOp
	if compareOp == "" {
		compareOp = "gt"
	}

	// 默认告警级别为 warn
	alertLevel := metric.AlertLevel
	if alertLevel == "" {
		alertLevel = "warn"
	}

	result.Alert = true
	result.AlertLevel = alertLevel

	// 检查主阈值（严重告警）
	if metric.Threshold > 0 {
		if compare(compareOp, value, metric.Threshold) {
			result.OverThreshold = true
			logAlert("error", metric, value, metric.Threshold, "严重")
			return
		}
	}

	// 检查警告阈值
	if metric.WarningThreshold > 0 {
		if compare(compareOp, value, metric.WarningThreshold) {
			result.IsWarning = true
			logAlert("warn", metric, value, metric.WarningThreshold, "警告")
		}
	}
}

// compare 执行比较操作
func compare(op string, value float64, threshold float64) bool {
	switch op {
	case "gt": // 大于
		return value > threshold
	case "lt": // 小于
		return value < threshold
	case "eq": // 等于
		return value == threshold
	case "ge": // 大于等于
		return value >= threshold
	case "le": // 小于等于
		return value <= threshold
	default:
		return value > threshold // 默认大于
	}
}

// logAlert 记录告警日志
func logAlert(level string, metric MetricConfig, value float64, threshold float64, levelDesc string) {
	msg := fmt.Sprintf("[%s] %s (%s): %.2f %s %s 阈值 %.2f",
		levelDesc, metric.Name, metric.Alias, value, metric.Unit, getOpDesc(metric.CompareOp), threshold)

	switch level {
	case "error":
		logger.Error(msg)
	case "warn":
		logger.Warn(msg)
	case "info":
		logger.Info(msg)
	}
}

var (
	consecutiveAlertCounts = make(map[string]int)
	consecutiveMu          sync.Mutex
)

// applyConsecutiveAlerts 根据配置的连续次数要求，过滤尚未达到连续次数的告警。
func applyConsecutiveAlerts(targetName string, metricConfigs []MetricConfig, results []MetricResult) {
	configMap := make(map[string]MetricConfig)
	for _, mc := range metricConfigs {
		configMap[mc.Name] = mc
	}

	for i := range results {
		mc, ok := configMap[results[i].Name]
		if !ok || mc.Consecutive <= 1 {
			continue
		}

		key := targetName + "::" + mc.Name

		if results[i].OverThreshold || results[i].IsWarning {
			consecutiveMu.Lock()
			consecutiveAlertCounts[key]++
			count := consecutiveAlertCounts[key]
			consecutiveMu.Unlock()

			if count < mc.Consecutive {
				results[i].OverThreshold = false
				results[i].IsWarning = false
				results[i].Alert = false
			}
		} else {
			consecutiveMu.Lock()
			delete(consecutiveAlertCounts, key)
			consecutiveMu.Unlock()
		}
	}
}

// getOpDesc 获取比较操作符描述
func getOpDesc(op string) string {
	switch op {
	case "gt":
		return ">"
	case "lt":
		return "<"
	case "eq":
		return "=="
	case "ge":
		return ">="
	case "le":
		return "<="
	default:
		return ">"
	}
}

// toFloat 将任意类型转换为 float64
func toFloat(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("不支持的类型: %T", val)
	}
}

// PrintJSONTree 打印 JSON 树形结构，方便用户获取路径
func PrintJSONTree(jsonData interface{}) string {
	return formatJSONTree(jsonData, "")
}

// formatJSONTree 递归遍历 JSON 数据，生成带 JSONPath 的树形结构字符串。
func formatJSONTree(data interface{}, prefix string) string {
	var result string

	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			result += fmt.Sprintf("%s$.%s\n", prefix, key)
			result += formatJSONTree(val, prefix+"  ")
		}
	case []interface{}:
		for i, val := range v {
			result += fmt.Sprintf("%s$[%d]\n", prefix, i)
			result += formatJSONTree(val, prefix+"  ")
		}
	default:
		result += fmt.Sprintf("%s= %v\n", prefix, v)
	}

	return result
}

// normalizePath 将 JSONPath 风格路径转换为 gjson 兼容路径。
// 支持的转换：
//   - 去除 "$." 或 "$" 前缀
//   - 数组索引 "[0]" 转为 ".0"
//   - 括号引号 '["key"]' 转为 ".key"
func normalizePath(path string) string {
	if path == "" {
		return path
	}

	p := path

	// 去除 "$." 前缀
	if len(p) > 2 && p[0] == '$' && p[1] == '.' {
		p = p[2:]
	} else if len(p) > 1 && p[0] == '$' {
		p = p[1:]
	}

	// 转换 [0] → .0 和 ["key"] → .key
	var result []byte
	i := 0
	for i < len(p) {
		if p[i] == '[' {
			i++
			// 跳过引号
			for i < len(p) && (p[i] == '"' || p[i] == '\'') {
				i++
			}
			// 收集 key
			keyStart := i
			for i < len(p) && p[i] != ']' && p[i] != '"' && p[i] != '\'' {
				i++
			}
			key := p[keyStart:i]
			// 跳过引号和 ]
			for i < len(p) && (p[i] == '"' || p[i] == '\'') {
				i++
			}
			if i < len(p) && p[i] == ']' {
				i++
			}
			result = append(result, '.')
			result = append(result, key...)
		} else {
			result = append(result, p[i])
			i++
		}
	}

	return string(result)
}
