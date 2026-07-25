// Package scraper 提供通用监控指标采集功能
// 支持通过配置文件定义监控目标，使用 JSONPath 提取指标
package scraper

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/oliveagle/jsonpath"
	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

// MetricConfig 表示单个指标配置
type MetricConfig struct {
	Name        string  `mapstructure:"name"`         // 指标名称
	Path        string  `mapstructure:"path"`         // JSONPath 路径
	Alias       string  `mapstructure:"alias"`        // 指标别名（可选）
	Unit        string  `mapstructure:"unit"`         // 单位（可选）
	Threshold   float64 `mapstructure:"threshold"`    // 阈值（可选）
	Alert       bool    `mapstructure:"alert"`        // 超过阈值是否告警
	Optional    bool    `mapstructure:"optional"`     // 是否为可选指标，不存在时不报错
	Scale       float64 `mapstructure:"scale"`        // 缩放因子（如 100 表示乘以100）
	AutoPercent bool    `mapstructure:"auto_percent"` // 自动处理百分比（值<1时乘以100）
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
	Name          string  `json:"name"`
	Alias         string  `json:"alias"`
	Value         float64 `json:"value"`
	Unit          string  `json:"unit"`
	Path          string  `json:"path"`
	Success       bool    `json:"success"`
	Error         string  `json:"error,omitempty"`
	Threshold     float64 `json:"threshold,omitempty"`
	Alert         bool    `json:"alert"`
	OverThreshold bool    `json:"over_threshold"`
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

	// 解析 JSON
	var jsonData interface{}
	if err := json.Unmarshal(resp.Body(), &jsonData); err != nil {
		result.Error = fmt.Sprintf("JSON 解析失败: %v", err)
		logger.Error(result.Error, zap.String("target", target.Name))
		return result, fmt.Errorf(result.Error)
	}

	// 使用 JSONPath 提取指标
	result.Metrics = extractMetrics(jsonData, target.Metrics)
	result.Success = true

	logger.Info("采集完成",
		zap.String("target", target.Name),
		zap.Int("metrics", len(result.Metrics)),
		zap.Duration("duration", result.Duration))

	return result, nil
}

// extractMetrics 使用 JSONPath 提取指标
func extractMetrics(jsonData interface{}, metrics []MetricConfig) []MetricResult {
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

		// 使用 JSONPath 提取值
		val, err := jsonpath.JsonPathLookup(jsonData, metric.Path)
		if err != nil {
			// 如果是可选指标，跳过不记录
			if metric.Optional {
				logger.Debug("可选指标不存在，跳过",
					zap.String("metric", metric.Name),
					zap.String("path", metric.Path))
				continue
			}
			result.Error = fmt.Sprintf("JSONPath 提取失败: %v", err)
			logger.Warn("指标提取失败",
				zap.String("metric", metric.Name),
				zap.String("path", metric.Path),
				zap.String("error", result.Error))
			results = append(results, result)
			continue
		}

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

		// 检查是否超过阈值
		if metric.Threshold > 0 && metric.Alert {
			result.OverThreshold = floatVal > metric.Threshold
			if result.OverThreshold {
				logger.Warn("指标超过阈值",
					zap.String("metric", metric.Name),
					zap.Float64("value", floatVal),
					zap.Float64("threshold", metric.Threshold))
			}
		}

		results = append(results, result)
	}

	return results
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
