// Package config 提供配置管理功能
// 使用 viper 读取 YAML 配置文件并解析到结构体中
package config

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// CfgFile 存储命令行指定的配置文件路径
var CfgFile string

// Config 表示应用程序的完整配置
type Config struct {
	Target  TargetConfig      `mapstructure:"target"`  // 目标 API 配置
	Log     LogConfig         `mapstructure:"log"`     // 日志配置
	App     AppConfig         `mapstructure:"app"`     // 应用配置
	HTTP    HTTPConfig        `mapstructure:"http"`    // HTTP 客户端配置
	Email   EmailConfig       `mapstructure:"email"`   // 邮件配置
	Cleaner CleanupConfig     `mapstructure:"cleaner"` // 自动清理配置
	Monitor MonitorConfig     `mapstructure:"monitor"` // 监控配置
	Vars    map[string]string `mapstructure:"vars"`    // 用户自定义变量（用于替换测试用例中的 {{var}}）
}

// HTTPConfig 表示 HTTP 客户端的配置
type HTTPConfig struct {
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"` // 是否跳过 TLS 证书验证
}

// TargetConfig 表示目标 API 的配置
type TargetConfig struct {
	BaseURL       string `mapstructure:"base_url"`      // API 基础地址
	Timeout       int    `mapstructure:"timeout"`       // 请求超时时间（秒）
	Authorization string `mapstructure:"authorization"` // API 授权令牌
	UserId        string `mapstructure:"user_id"`       // 用户 ID
}

// LogConfig 表示日志系统的配置
type LogConfig struct {
	Level    string `mapstructure:"level"`    // 日志级别 (debug/info/warn/error)
	Encoding string `mapstructure:"encoding"` // 日志格式 (json/console)
	Output   string `mapstructure:"output"`   // 输出位置 (stdout 或文件路径)
}

// AppConfig 表示应用相关的配置
type AppConfig struct {
	ReportDir     string   `mapstructure:"report_dir"`     // 报告输出目录（包括告警记录）
	CaseDir       string   `mapstructure:"case_dir"`       // 默认测试用例/监控配置目录
	DataDir       string   `mapstructure:"data_dir"`       // 数据存储目录（用于 CSV 文件）

	SevereStatus  []int    `mapstructure:"severe_status"`  // 严重错误状态码列表，这些状态码的测试用例失败时优先于其他失败用例
	GlobalPre     []string `mapstructure:"global_pre"`     // 全局前置条件测试用例ID列表（所有测试执行前运行）
	GlobalPost    []string `mapstructure:"global_post"`    // 全局后置条件测试用例ID列表（所有测试执行后运行）
	HostName      string   `mapstructure:"host_name"`      // 主机名称（未配置时自动使用主机名）
}

// EmailConfig 表示邮件发送相关的配置
type EmailConfig struct {
	Enabled         bool     `mapstructure:"enabled"`          // 是否启用邮件发送
	From            string   `mapstructure:"from"`             // 发件人邮箱
	To              []string `mapstructure:"to"`               // 收件人邮箱列表
	AuthCode        string   `mapstructure:"auth_code"`        // 邮箱授权码
	SMTPServer      string   `mapstructure:"smtp_server"`     // SMTP 服务器地址
	SMTPPort        int      `mapstructure:"smtp_port"`        // SMTP 端口
	ErrorSubject    string   `mapstructure:"error_subject"`    // 异常报告邮件标题模板（支持 {{device}} 和 {{time}} 占位符）
}

// CleanupConfig 表示自动清理相关的配置
type CleanupConfig struct {
	Enabled         bool     `mapstructure:"enabled"`          // 是否启用自动清理
	RetentionDays   int      `mapstructure:"retention_days"`   // 文件保留天数
	LogDir          string   `mapstructure:"log_dir"`          // 日志目录（自动从 log.output 提取）
	ReportDir       string   `mapstructure:"report_dir"`       // 测试报告目录（自动从 test.report_dir 提取）
	DataDir         string   `mapstructure:"data_dir"`         // 数据目录（自动从 test.data_dir 提取）
	IncludePatterns []string `mapstructure:"include_patterns"` // 要清理的文件模式列表（如 *.log, *.json）
	ExcludePatterns []string `mapstructure:"exclude_patterns"` // 排除的文件模式列表
	IntervalHours   int      `mapstructure:"interval_hours"`   // 定时清理间隔（小时）
}

// MonitorConfig 表示监控相关的配置
type MonitorConfig struct {
	Enabled         bool `mapstructure:"enabled"`          // 是否启用监控模式（全局开关）
	DefaultInterval int  `mapstructure:"default_interval"` // 默认监控周期（秒）
	AlertOnFailure  bool `mapstructure:"alert_on_failure"` // 默认失败时告警
	AlertOnSlow     bool `mapstructure:"alert_on_slow"`    // 默认响应慢时告警
}

// AppConfig 存储全局配置实例
var AppConfig Config

// InitConfig 初始化配置
// 从配置文件读取配置并解析到 AppConfig 中
func InitConfig() {
	// 如果指定了配置文件路径，使用指定的文件
	if CfgFile != "" {
		viper.SetConfigFile(CfgFile)
	} else {
		// 默认从 ./config/config.yaml 读取配置
		viper.AddConfigPath("./config")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	// 将配置解析到结构体（vars 字段会被 viper 转换为小写，后续会修复）
	if err := viper.Unmarshal(&AppConfig); err != nil {
		log.Fatalf("Unable to decode config into struct: %v", err)
	}

	// 设置 cleaner 的默认配置
	setCleanerDefaults()

	// 单独读取 vars 配置，保留原始键名（避免 viper 自动转换小写）
	AppConfig.Vars = loadRawVars()
}

// setCleanerDefaults 设置 cleaner 配置的默认值
// 如果用户完全没有配置 cleaner，则启用默认配置
// 如果用户配置了 cleaner 的某些字段，则只为空字段设置默认值
func setCleanerDefaults() {
	// 检查配置文件中是否存在 cleaner 配置
	hasCleanerConfig := viper.IsSet("cleaner")

	// 如果用户完全没有配置 cleaner，启用默认配置（包括 enabled: true）
	if !hasCleanerConfig {
		AppConfig.Cleaner.Enabled = true
		AppConfig.Cleaner.RetentionDays = 30
		AppConfig.Cleaner.LogDir = "./logs"
		AppConfig.Cleaner.ReportDir = "./reports"
		AppConfig.Cleaner.DataDir = "./sql"
		AppConfig.Cleaner.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
		AppConfig.Cleaner.IntervalHours = 24
		return
	}

	// 如果用户配置了 cleaner，但某些字段为空，则只为空字段设置默认值
	if AppConfig.Cleaner.RetentionDays <= 0 {
		AppConfig.Cleaner.RetentionDays = 30
	}
	if AppConfig.Cleaner.LogDir == "" {
		AppConfig.Cleaner.LogDir = "./logs"
	}
	if AppConfig.Cleaner.ReportDir == "" {
		AppConfig.Cleaner.ReportDir = "./reports"
	}
	if AppConfig.Cleaner.DataDir == "" {
		AppConfig.Cleaner.DataDir = "./sql"
	}
	if len(AppConfig.Cleaner.IncludePatterns) == 0 {
		AppConfig.Cleaner.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
	}
	if AppConfig.Cleaner.IntervalHours <= 0 {
		AppConfig.Cleaner.IntervalHours = 24
	}
}

// loadRawVars 从配置文件读取原始 vars，保留键名大小写。
// viper 默认会把所有配置键转为小写，因此需要直接解析 YAML 文件来保留原始键名。
func loadRawVars() map[string]string {
	result := make(map[string]string)

	// 确定配置文件路径（不依赖 viper.ConfigFileUsed，避免某些场景返回空）
	configFile := CfgFile
	if configFile == "" {
		configFile = "./config/config.yaml"
	}

	// 直接从文件读取原始 YAML，保留 vars 键名大小写
	data, err := os.ReadFile(configFile)
	if err == nil {
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err == nil {
			if varsMap, ok := raw["vars"].(map[string]any); ok {
				for k, v := range varsMap {
					switch val := v.(type) {
					case string:
						result[k] = val
					default:
						result[k] = fmt.Sprintf("%v", val)
					}
				}
				return result
			}
		}
	}

	// 回退：使用 viper 读取（键名会被转小写）
	for k, v := range viper.GetStringMapString("vars") {
		result[k] = v
	}
	return result
}