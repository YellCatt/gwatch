// Package httpclient 提供 HTTP 客户端功能
// 使用 resty 库封装 HTTP 请求，支持重试和超时配置
package httpclient

import (
	"crypto/tls"
	"time"

	"github.com/go-resty/resty/v2"

	"pipetGo/config"
)

// Client 是全局 HTTP 客户端实例
var Client *resty.Client

// InitClient 初始化 HTTP 客户端
// 配置基础URL、超时时间和重试策略
func InitClient() {
	client := resty.New().
		SetBaseURL(config.AppConfig.Target.BaseURL).                              // 设置 API 基础地址
		SetTimeout(time.Duration(config.AppConfig.Target.Timeout) * time.Second). // 设置请求超时
		SetRetryCount(3).                                                         // 设置重试次数
		SetRetryWaitTime(1 * time.Second).                                        // 设置重试等待时间
		SetRetryMaxWaitTime(5 * time.Second)                                      // 设置最大重试等待时间

	// 如果配置了跳过 TLS 验证
	if config.AppConfig.HTTP.InsecureSkipVerify {
		client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	}

	Client = client
}
