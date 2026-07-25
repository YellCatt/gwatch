// Package cmd 提供命令行接口功能
package cmd

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/scraper"
)

var scraperCmd = &cobra.Command{
	Use:   "scraper",
	Short: "运行通用指标采集器",
	Long:  `从配置文件读取监控目标，使用 JSONPath 提取指标并输出`,
	Run: func(cmd *cobra.Command, args []string) {
		runScraper()
	},
}

var probeCmd = &cobra.Command{
	Use:   "probe <url>",
	Short: "探测目标 URL 并打印 JSON 结构",
	Long:  `发送 HTTP 请求到目标 URL，解析返回的 JSON 并打印树形结构，方便获取 JSONPath`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		probeURL(args[0])
	},
}

func init() {
	rootCmd.AddCommand(scraperCmd)
	rootCmd.AddCommand(probeCmd)
}

func runScraper() {
	cfg := config.GlobalConfig.Scraper

	if !cfg.Enabled {
		logger.Info("通用采集器未启用")
		return
	}

	if len(cfg.Targets) == 0 {
		logger.Info("未配置监控目标")
		return
	}

	logger.Info("启动通用采集器", zap.Int("targets", len(cfg.Targets)))

	// 循环采集
	for {
		// 收集本次采集的所有告警
		var alerts []email.AlertInfo

		for _, target := range cfg.Targets {
			if !target.Enabled {
				continue
			}

			// 转换配置
			scraperTarget := scraper.TargetConfig{
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
				scraperTarget.Metrics = append(scraperTarget.Metrics, scraper.MetricConfig{
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

			// 采集指标
			result, err := scraper.Scrape(scraperTarget)
			if err != nil {
				logger.Error("采集失败", zap.String("target", target.Name), zap.Error(err))
				continue
			}

			// 输出结果
			fmt.Printf("\n【%s】%s\n", result.Timestamp.Format("2006-01-02 15:04:05"), result.TargetName)
			fmt.Printf("URL: %s\n", result.TargetURL)
			fmt.Printf("状态码: %d\n", result.StatusCode)
			fmt.Printf("耗时: %.2fms\n", float64(result.Duration.Microseconds())/1000)

			for _, metric := range result.Metrics {
				status := "✅"
				alertLevel := ""
				if !metric.Success {
					status = "❌"
				} else if metric.OverThreshold {
					status = "🔴"
					alertLevel = "严重"
				} else if metric.IsWarning {
					status = "🟡"
					alertLevel = "警告"
				}

				name := metric.Name
				if metric.Alias != "" {
					name = metric.Alias
				}

				if metric.Success {
					fmt.Printf("  %s %s: %.2f %s", status, name, metric.Value, metric.Unit)
					if metric.WarningThreshold > 0 {
						fmt.Printf(" (警告阈值: %.2f)", metric.WarningThreshold)
					}
					if metric.Threshold > 0 {
						fmt.Printf(" (严重阈值: %.2f)", metric.Threshold)
					}
					if metric.OverThreshold {
						fmt.Printf(" 🔴 严重告警")
					} else if metric.IsWarning {
						fmt.Printf(" 🟡 警告")
					}
					fmt.Println()

					// 收集告警信息
					if alertLevel != "" {
						alerts = append(alerts, email.AlertInfo{
							TargetName:       target.Name,
							MetricName:       metric.Name,
							MetricAlias:      metric.Alias,
							Value:            metric.Value,
							Unit:             metric.Unit,
							Threshold:        metric.Threshold,
							WarningThreshold: metric.WarningThreshold,
							AlertLevel:       alertLevel,
						})
					}
				} else {
					fmt.Printf("  %s %s: 提取失败 - %s\n", status, name, metric.Error)
				}
			}
		}

		// 如果有告警，发送邮件通知
		if len(alerts) > 0 {
			logger.Warn("检测到告警", zap.Int("count", len(alerts)))
			if err := email.SendAlertEmail(alerts); err != nil {
				logger.Error("发送告警邮件失败", zap.Error(err))
			}
		}

		// 等待下次采集
		time.Sleep(10 * time.Second)
	}
}

func probeURL(targetURL string) {
	// 解析 URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		fmt.Printf("URL 解析失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("正在探测: %s\n", targetURL)
	fmt.Println("----------------------------------------")

	// 创建 HTTP 客户端
	client := resty.New()
	client.SetTimeout(10 * time.Second)

	// 判断是否需要跳过 TLS 验证
	if parsedURL.Scheme == "https" {
		client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	}

	// 发送请求
	resp, err := client.R().Get(targetURL)
	if err != nil {
		fmt.Printf("请求失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("HTTP 状态码: %d\n", resp.StatusCode())
	fmt.Printf("响应长度: %d 字节\n", len(resp.Body()))
	fmt.Println("----------------------------------------")

	// 尝试解析 JSON
	var jsonData interface{}
	if err := json.Unmarshal(resp.Body(), &jsonData); err != nil {
		fmt.Println("响应不是有效的 JSON 格式")
		fmt.Println("----------------------------------------")
		fmt.Println(string(resp.Body()))
		os.Exit(0)
	}

	// 打印 JSON 树形结构
	fmt.Println("JSON 结构路径:")
	fmt.Println("----------------------------------------")
	fmt.Println(scraper.PrintJSONTree(jsonData))

	// 输出示例配置
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

	// 尝试自动提取简单的叶子节点
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
