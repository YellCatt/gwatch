// Package bootstrap 负责 gwatch 应用启动阶段的初始化工作：
// 创建必要目录、加载配置、初始化日志、加载变量、初始化邮件模块等。
package bootstrap

import (
	"fmt"
	"os"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/email"
	"gwatch/internal/logger"
	"gwatch/internal/vars"
)

// InitApp 应用入口初始化函数（在 cobra.OnInitialize 中被调用）。
// 初始化顺序：目录 -> 配置 -> 日志 -> 变量 -> 邮件。
func InitApp() {
	InitDirectories()

	config.InitConfig()

	logger.InitLogger(logger.LogConfig{
		Level:     config.GlobalConfig.Log.Level,
		Encoding:  config.GlobalConfig.Log.Encoding,
		Output:    config.GlobalConfig.Log.Output,
		MaxSizeMB: config.GlobalConfig.Log.MaxSizeMB,
	})

	logger.Info("gwatch 已启动", zap.String("版本", config.Version))

	// 注入基础变量 base_url，供后续模板替换使用
	vars.Set("base_url", config.GlobalConfig.Target.BaseURL)

	if len(config.GlobalConfig.Vars) > 0 {
		vars.InitFromConfig(config.GlobalConfig.Vars)
		logger.Info("用户自定义变量加载完成", zap.Int("count", len(config.GlobalConfig.Vars)), zap.Any("vars", maskVars(config.GlobalConfig.Vars)))
	} else {
		logger.Info("未配置用户自定义变量")
	}
	logger.Info("当前可用变量", zap.Any("vars", vars.GetAll()))

	email.InitEmail(email.EmailConfig{
		Enabled:         config.GlobalConfig.Email.Enabled,
		FromEmail:       config.GlobalConfig.Email.From,
		ToEmail:         config.GlobalConfig.Email.To,
		AuthCode:        config.GlobalConfig.Email.AuthCode,
		SMTPServer:      config.GlobalConfig.Email.SMTPServer,
		SMTPPort:        config.GlobalConfig.Email.SMTPPort,
		DeviceName:      config.GlobalConfig.App.HostName,
		ErrorSubject:    config.GlobalConfig.Email.ErrorSubject,
		ScraperCooldown: config.GlobalConfig.Email.ScraperCooldown,
		APICooldown:     config.GlobalConfig.Email.APICooldown,
		SystemCooldown:  config.GlobalConfig.Email.SystemCooldown,
	})
}

// maskVars 对敏感变量进行脱敏处理后返回，仅用于日志打印。
func maskVars(vars map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range vars {
		result[k] = maskString(v)
	}
	return result
}

// maskString 将任意字符串脱敏：短字符串直接替换为 ***，长字符串保留前后 4 位。
func maskString(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

// InitDirectories 创建项目运行所需的基础目录（配置、日志、报告、数据等），
// 若目录不存在则自动创建。同时尝试生成默认配置文件，便于首次运行。
func InitDirectories() {
	directories := []string{
		"./config",
		"./logs",
		"./reports",
		"./sql",
		"./cases",
	}

	for _, dir := range directories {
		if dir == "." || dir == "/" {
			continue
		}

		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("警告: 创建目录失败 '%s': %v\n", dir, err)
		}
	}

	createDefaultConfigFile()
}

// createDefaultConfigFile 若 config/config.yaml 不存在则写入一份默认模板，
// 方便首次运行的用户快速上手。若已存在则直接返回。
func createDefaultConfigFile() {
	configPath := "./config/config.yaml"

	if _, err := os.Stat(configPath); err == nil {
		return
	}

	defaultConfig := `target:
  base_url: "https://localhost:8080"
  timeout: 30

log:
  level: "info"
  encoding: "json"
  output: "./logs/gwatch.log"
  max_size_mb: 20

app:
  report_dir: "./reports"
  case_dir: "./demo"
  data_dir: "./sql"
  severe_status:
    - 500
  global_pre: []
  global_post: []
  host_name: ""

http:
  insecure_skip_verify: false

vars: {}

monitor:
  enabled: true
  default_interval: 60
  alert_on_failure: true
  alert_on_slow: true
  max_workers: 1

email:
  enabled: false
  from: ""
  to: []
  auth_code: ""
  smtp_server: "smtp.example.com"
  smtp_port: 465
  scraper_cooldown: 21600
  api_cooldown: 21600
  system_cooldown: 7200

sys_monitor:
  enabled: true
  interval: 10
  chart_enabled: true
  email_enabled: true
  cpu_threshold: 85
  memory_threshold: 90
  disk_usage_threshold: 90
  network_down_threshold: 1.0
  network_up_threshold: 1.0
  alert_cooldown: 300

cleaner:
  enabled: true
  retention_days: 30
  log_dir: "./logs"
  report_dir: "./reports"
  data_dir: "./sql"
  include_patterns:
    - "*.log"
    - "*.json"
    - "*.csv"
    - "*.txt"
  exclude_patterns: []
  interval_hours: 24
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		fmt.Printf("警告: 创建默认配置文件失败 '%s': %v\n", configPath, err)
	} else {
		fmt.Printf("已创建默认配置文件: %s\n", configPath)
	}
}
