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

func InitApp() {
	InitDirectories()

	config.InitConfig()

	logger.InitLogger(logger.LogConfig{
		Level:    config.GlobalConfig.Log.Level,
		Encoding: config.GlobalConfig.Log.Encoding,
		Output:   config.GlobalConfig.Log.Output,
	})

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

func maskVars(vars map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range vars {
		result[k] = maskString(v)
	}
	return result
}

func maskString(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

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
