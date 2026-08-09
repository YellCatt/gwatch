package scraper

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/storage"
	"gwatch/internal/util"
)

var (
	stopChan chan struct{}
	running  bool
)

func StartLoop() {
	cfg := config.GlobalConfig.Scraper

	if !cfg.Enabled {
		logger.Info("通用采集器未启用")
		return
	}

	if len(cfg.Targets) == 0 {
		logger.Info("未配置监控目标")
		return
	}

	stopChan = make(chan struct{})
	running = true
	defer func() {
		running = false
	}()

	logger.Info("启动通用采集器", zap.Int("targets", len(cfg.Targets)))

	for {
		select {
		case <-stopChan:
			logger.Info("通用采集器已停止")
			return
		default:
		}

		for _, target := range cfg.Targets {
			if !target.Enabled {
				continue
			}

			scraperTarget := TargetConfig{
				Name:               target.Name,
				URL:                target.URL,
				Method:             target.Method,
				Timeout:            target.Timeout,
				Interval:           target.Interval,
				Enabled:            target.Enabled,
				Headers:            target.Headers,
				Body:               target.Body,
				InsecureSkipVerify: target.InsecureSkipVerify,
				Proxy:              target.Proxy,
			}

			for _, metric := range target.Metrics {
				scraperTarget.Metrics = append(scraperTarget.Metrics, MetricConfig{
					Name:             metric.Name,
					Path:             metric.Path,
					Alias:            metric.Alias,
					Unit:             metric.Unit,
					Threshold:        metric.Threshold,
					Alert:            metric.Alert,
					Optional:         metric.Optional,
					Scale:            metric.Scale,
					AutoPercent:      metric.AutoPercent,
					CompareOp:        metric.CompareOp,
					AlertLevel:       metric.AlertLevel,
					Consecutive:      metric.Consecutive,
					WarningThreshold: metric.WarningThreshold,
				})
			}

			result, err := Scrape(scraperTarget)
			if err != nil {
				logger.Error("采集失败", zap.String("target", target.Name), zap.Error(err))
				continue
			}

			fmt.Printf("\n【%s】%s\n", result.Timestamp.Format("2006-01-02 15:04:05"), result.TargetName)
			fmt.Printf("URL: %s\n", result.TargetURL)
			fmt.Printf("状态码: %d\n", result.StatusCode)
			fmt.Printf("耗时: %.2fms\n", float64(result.Duration.Microseconds())/1000)

			for _, metric := range result.Metrics {
				status := "OK"
				alertLevel := ""
				if !metric.Success {
					status = "FAIL"
				} else if metric.OverThreshold {
					status = "CRITICAL"
					alertLevel = "严重"
				} else if metric.IsWarning {
					status = "WARNING"
					alertLevel = "警告"
				}

				name := metric.Name
				if metric.Alias != "" {
					name = metric.Alias
				}

				if metric.Success {
					isSpeed := util.IsSpeedUnit(metric.Unit)
					valueStr := fmt.Sprintf("%.2f %s", metric.Value, metric.Unit)
					if isSpeed {
						valueStr = util.FormatSpeed(metric.Value)
					}
					fmt.Printf("  [%s] %s: %s", status, name, valueStr)
					if metric.WarningThreshold > 0 {
						if isSpeed {
							fmt.Printf(" (警告阈值: %s)", util.FormatSpeed(metric.WarningThreshold))
						} else {
							fmt.Printf(" (警告阈值: %.2f %s)", metric.WarningThreshold, metric.Unit)
						}
					}
					if metric.Threshold > 0 {
						if isSpeed {
							fmt.Printf(" (严重阈值: %s)", util.FormatSpeed(metric.Threshold))
						} else {
							fmt.Printf(" (严重阈值: %.2f %s)", metric.Threshold, metric.Unit)
						}
					}
					if metric.OverThreshold {
						fmt.Printf(" [严重告警]")
					} else if metric.IsWarning {
						fmt.Printf(" [警告]")
					}
					fmt.Println()

					if alertLevel != "" {
						isSpeed := util.IsSpeedUnit(metric.Unit)
						email.DispatchAlert(email.UnifiedAlert{
							Source:      email.SourceScraper,
							SourceName:  "远程资源采集",
							TargetName:  target.Name,
							MetricName:  metric.Name,
							MetricAlias: metric.Alias,
							Value:       metric.Value,
							Unit:        metric.Unit,
							Threshold:   metric.Threshold,
							AlertLevel:  alertLevel,
							Message:     buildAlertMessage(name, metric, isSpeed),
							Timestamp:   result.Timestamp,
						})

						dateStr := result.Timestamp.Format("2006-01-02")
						storage.UpdateScraperAlertSummary(storage.ScraperAlertRecord{
							Date:        dateStr,
							TargetName:  target.Name,
							TargetURL:   target.URL,
							MetricName:  metric.Name,
							MetricAlias: metric.Alias,
							Value:       metric.Value,
							Threshold:   metric.Threshold,
							Unit:        metric.Unit,
							AlertLevel:  alertLevel,
							Message:     buildAlertMessage(name, metric, isSpeed),
						})
					}
				} else {
					fmt.Printf("  [%s] %s: 提取失败 - %s\n", status, name, metric.Error)
				}

				record := storage.ScraperMetricRecord{
					TargetName:  result.TargetName,
					TargetURL:   result.TargetURL,
					MetricName:  metric.Name,
					MetricAlias: metric.Alias,
					Value:       metric.Value,
					Unit:        metric.Unit,
					Success:     metric.Success,
					Timestamp:   result.Timestamp,
				}
				if err := storage.RecordScraperMetric(record); err != nil {
					logger.Warn("Failed to record scraper metric", zap.Error(err))
				}
			}
		}

		time.Sleep(10 * time.Second)
	}
}

func StopLoop() {
	if stopChan != nil && running {
		close(stopChan)
	}
}

func ProbeURL(targetURL string) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		fmt.Printf("URL 解析失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("正在探测: %s\n", targetURL)
	fmt.Println("----------------------------------------")

	client := resty.New()
	client.SetTimeout(10 * time.Second)

	if parsedURL.Scheme == "https" {
		client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	}

	resp, err := client.R().Get(targetURL)
	if err != nil {
		fmt.Printf("请求失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("HTTP 状态码: %d\n", resp.StatusCode())
	fmt.Printf("响应长度: %d 字节\n", len(resp.Body()))
	fmt.Println("----------------------------------------")

	var jsonData interface{}
	if err := json.Unmarshal(resp.Body(), &jsonData); err != nil {
		fmt.Println("响应不是有效的 JSON 格式")
		fmt.Println("----------------------------------------")
		fmt.Println(string(resp.Body()))
		os.Exit(0)
	}

	fmt.Println("JSON 结构路径:")
	fmt.Println("----------------------------------------")
	fmt.Println(PrintJSONTree(jsonData))

	fmt.Println("----------------------------------------")
	fmt.Println("建议配置:")
	fmt.Println("scraper:")
	fmt.Println("  targets:")
	fmt.Println("    - name: \"自定义名称\"")
	fmt.Printf("      url: \"%s\"\n", targetURL)
	fmt.Println("      method: GET")
	fmt.Println("      timeout: 5s")
	fmt.Println("      interval: 10")
	fmt.Println("      enabled: true")
	fmt.Println("      metrics:")

	printMetricSuggestions(jsonData, "$")
}

func printMetricSuggestions(data interface{}, path string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			newPath := fmt.Sprintf("%s.%s", path, key)
			switch val.(type) {
			case float64, int, string, bool:
				fmt.Printf("        - name: %s\n", key)
				fmt.Printf("          path: \"%s\"\n", newPath)
				fmt.Printf("          alias: \"%s\"\n", key)
				fmt.Println("          unit: \"\"")
			default:
				printMetricSuggestions(val, newPath)
			}
		}
	case []interface{}:
		for i, val := range v {
			newPath := fmt.Sprintf("%s[%d]", path, i)
			printMetricSuggestions(val, newPath)
		}
	}
}

func buildAlertMessage(name string, metric MetricResult, isSpeed bool) string {
	if isSpeed {
		return fmt.Sprintf("%s: %s 超过阈值 %s", name, util.FormatSpeed(metric.Value), util.FormatSpeed(metric.Threshold))
	}
	return fmt.Sprintf("%s: %.2f %s 超过阈值 %.2f %s", name, metric.Value, metric.Unit, metric.Threshold, metric.Unit)
}
