// Package httpclient 提供全局 HTTP 客户端封装。
// 基于 resty 库实现，统一配置基础 URL、超时、重试与 TLS 选项，供各模块复用。
package httpclient

import (
	"crypto/tls"
	"time"

	"github.com/go-resty/resty/v2"

	"gwatch/config"
)

// Client 全局 HTTP 客户端实例，由 InitClient 初始化。
var Client *resty.Client

// InitClient 根据全局配置初始化 HTTP 客户端。
// 配置项包括：基础 URL、请求超时、重试策略以及是否跳过 TLS 校验。
func InitClient() {
	client := resty.New().
		SetBaseURL(config.GlobalConfig.Target.BaseURL).                              // 设置 API 基础地址
		SetTimeout(time.Duration(config.GlobalConfig.Target.Timeout) * time.Second). // 设置请求超时
		SetRetryCount(3).                                                            // 设置重试次数
		SetRetryWaitTime(1 * time.Second).                                           // 设置重试等待时间
		SetRetryMaxWaitTime(5 * time.Second)                                         // 设置最大重试等待时间

	if config.GlobalConfig.HTTP.InsecureSkipVerify {
		client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	}

	Client = client
}
