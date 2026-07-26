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
	Scraper ScraperConfig     `mapstructure:"scraper"` // 通用采集器配置
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

// ScraperMetricConfig 表示单个指标配置
type ScraperMetricConfig struct {
	Name             string  `mapstructure:"name"`              // 指标名称
	Path             string  `mapstructure:"path"`              // JSONPath 路径
	Alias            string  `mapstructure:"alias"`             // 指标别名（可选）
	Unit             string  `mapstructure:"unit"`              // 单位（可选）
	Threshold        float64 `mapstructure:"threshold"`         // 严重阈值（超过此值触发严重告警）
	Alert            bool    `mapstructure:"alert"`             // 是否启用告警
	Optional         bool    `mapstructure:"optional"`          // 是否为可选指标，不存在时不报错
	Scale            float64 `mapstructure:"scale"`             // 缩放因子（如 100 表示乘以100）
	AutoPercent      bool    `mapstructure:"auto_percent"`      // 自动处理百分比（值<1时乘以100）
	CompareOp        string  `mapstructure:"compare_op"`        // 比较操作符：gt(大于), lt(小于), eq(等于), ge(大于等于), le(小于等于)
	AlertLevel       string  `mapstructure:"alert_level"`       // 告警级别：info, warn, error
	Consecutive      int     `mapstructure:"consecutive"`       // 连续超过阈值多少次才告警
	WarningThreshold float64 `mapstructure:"warning_threshold"` // 警告阈值（超过此值触发警告告警）
}

// ScraperTargetConfig 表示监控目标配置
type ScraperTargetConfig struct {
	Name               string                `mapstructure:"name"`                 // 目标名称
	URL                string                `mapstructure:"url"`                  // 请求URL
	Method             string                `mapstructure:"method"`               // HTTP方法（GET/POST等）
	Timeout            string                `mapstructure:"timeout"`              // 超时时间（如 5s）
	Interval           int                   `mapstructure:"interval"`             // 采集间隔（秒），默认10秒
	Enabled            bool                  `mapstructure:"enabled"`              // 是否启用
	Headers            map[string]string     `mapstructure:"headers"`              // 请求头
	Body               string                `mapstructure:"body"`                 // 请求体（POST时使用）
	InsecureSkipVerify bool                  `mapstructure:"insecure_skip_verify"` // 是否跳过TLS验证
	Proxy              string                `mapstructure:"proxy"`                // 代理地址
	Metrics            []ScraperMetricConfig `mapstructure:"metrics"`              // 指标配置列表
}

// ScraperConfig 表示通用采集器配置
type ScraperConfig struct {
	Enabled bool                  `mapstructure:"enabled"` // 是否启用通用采集器
	Targets []ScraperTargetConfig `mapstructure:"targets"` // 监控目标列表
}

// LogConfig 表示日志系统的配置
type LogConfig struct {
	Level    string `mapstructure:"level"`    // 日志级别 (debug/info/warn/error)
	Encoding string `mapstructure:"encoding"` // 日志格式 (json/console)
	Output   string `mapstructure:"output"`   // 输出位置 (stdout 或文件路径)
}

// AppConfig 表示应用相关的配置
type AppConfig struct {
	ReportDir string `mapstructure:"report_dir"` // 报告输出目录（包括告警记录）
	CaseDir   string `mapstructure:"case_dir"`   // 默认测试用例/监控配置目录
	DataDir   string `mapstructure:"data_dir"`   // 数据存储目录（用于 CSV 文件）

	SevereStatus []int    `mapstructure:"severe_status"` // 严重错误状态码列表，这些状态码的测试用例失败时优先于其他失败用例
	GlobalPre    []string `mapstructure:"global_pre"`    // 全局前置条件测试用例ID列表（所有测试执行前运行）
	GlobalPost   []string `mapstructure:"global_post"`   // 全局后置条件测试用例ID列表（所有测试执行后运行）
	HostName     string   `mapstructure:"host_name"`     // 主机名称（未配置时自动使用主机名）
}

// EmailConfig 表示邮件发送相关的配置
type EmailConfig struct {
	Enabled      bool     `mapstructure:"enabled"`       // 是否启用邮件发送
	From         string   `mapstructure:"from"`          // 发件人邮箱
	To           []string `mapstructure:"to"`            // 收件人邮箱列表
	AuthCode     string   `mapstructure:"auth_code"`     // 邮箱授权码
	SMTPServer   string   `mapstructure:"smtp_server"`   // SMTP 服务器地址
	SMTPPort     int      `mapstructure:"smtp_port"`     // SMTP 端口
	ErrorSubject string   `mapstructure:"error_subject"` // 异常报告邮件标题模板（支持 {{device}} 和 {{time}} 占位符）
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
	Enabled         bool   `mapstructure:"enabled"`          // 是否启用监控模式（全局开关）
	DefaultInterval int    `mapstructure:"default_interval"` // 默认监控周期（秒）
	AlertOnFailure  bool   `mapstructure:"alert_on_failure"` // 默认失败时告警
	AlertOnSlow     bool   `mapstructure:"alert_on_slow"`    // 默认响应慢时告警
	MaxWorkers      int    `mapstructure:"max_workers"`      // 最大并发goroutine数，0表示不限制，默认1适合资源受限设备
	AlertInterval   int    `mapstructure:"alert_interval"`   // 告警间隔（秒），相同接口异常后需要等待此时间才能再次告警，默认6小时
	DailyReport     bool   `mapstructure:"daily_report"`     // 是否启用每日报告
	WeeklyReport    bool   `mapstructure:"weekly_report"`    // 是否启用每周报告
	MonthlyReport   bool   `mapstructure:"monthly_report"`   // 是否启用每月报告
	YearlyReport    bool   `mapstructure:"yearly_report"`    // 是否启用年度报告
	ReportTime      string `mapstructure:"report_time"`      // 报告生成时间（HH:MM），适用于所有周期报告
}

// GlobalConfig 存储全局配置实例
var GlobalConfig Config

// InitConfig 初始化配置
// 从配置文件读取配置并解析到 GlobalConfig 中
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
	if err := viper.Unmarshal(&GlobalConfig); err != nil {
		log.Fatalf("Unable to decode config into struct: %v", err)
	}

	// 设置 cleaner 的默认配置
	setCleanerDefaults()

	// 设置 monitor 的默认配置
	setMonitorDefaults()

	// 设置 scraper 的默认配置
	setScraperDefaults()

	// 单独读取 vars 配置，保留原始键名（避免 viper 自动转换小写）
	GlobalConfig.Vars = loadRawVars()
}

// setCleanerDefaults 设置 cleaner 配置的默认值
// 如果用户完全没有配置 cleaner，则启用默认配置
// 如果用户配置了 cleaner 的某些字段，则只为空字段设置默认值
func setCleanerDefaults() {
	// 检查配置文件中是否存在 cleaner 配置
	hasCleanerConfig := viper.IsSet("cleaner")

	// 如果用户完全没有配置 cleaner，启用默认配置（包括 enabled: true）
	if !hasCleanerConfig {
		GlobalConfig.Cleaner.Enabled = true
		GlobalConfig.Cleaner.RetentionDays = 30
		GlobalConfig.Cleaner.LogDir = "./logs"
		GlobalConfig.Cleaner.ReportDir = "./reports"
		GlobalConfig.Cleaner.DataDir = "./sql"
		GlobalConfig.Cleaner.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
		GlobalConfig.Cleaner.IntervalHours = 24
		return
	}

	// 如果用户配置了 cleaner，但某些字段为空，则只为空字段设置默认值
	if GlobalConfig.Cleaner.RetentionDays <= 0 {
		GlobalConfig.Cleaner.RetentionDays = 30
	}
	if GlobalConfig.Cleaner.LogDir == "" {
		GlobalConfig.Cleaner.LogDir = "./logs"
	}
	if GlobalConfig.Cleaner.ReportDir == "" {
		GlobalConfig.Cleaner.ReportDir = "./reports"
	}
	if GlobalConfig.Cleaner.DataDir == "" {
		GlobalConfig.Cleaner.DataDir = "./sql"
	}
	if len(GlobalConfig.Cleaner.IncludePatterns) == 0 {
		GlobalConfig.Cleaner.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
	}
	if GlobalConfig.Cleaner.IntervalHours <= 0 {
		GlobalConfig.Cleaner.IntervalHours = 24
	}
}

// setScraperDefaults 设置 scraper 配置的默认值
// 如果用户完全没有配置 scraper，则禁用（enabled: false）
// 如果用户配置了 scraper，但未明确设置 enabled: true，则默认禁用（安全原则）
func setScraperDefaults() {
	// 检查配置文件中是否存在 scraper 配置
	hasScraperConfig := viper.IsSet("scraper")

	// 如果用户完全没有配置 scraper，禁用
	if !hasScraperConfig {
		GlobalConfig.Scraper.Enabled = false
		return
	}

	// 如果用户配置了 scraper，但未明确设置 enabled，默认禁用（安全原则：必须明确开启）
	// 只有明确设置 enabled: true 才会启用
	if !viper.IsSet("scraper.enabled") {
		GlobalConfig.Scraper.Enabled = false
	}

	// 为每个目标设置默认值
	for i := range GlobalConfig.Scraper.Targets {
		target := &GlobalConfig.Scraper.Targets[i]

		// 默认方法为 GET
		if target.Method == "" {
			target.Method = "GET"
		}

		// 默认超时时间为 5s
		if target.Timeout == "" {
			target.Timeout = "5s"
		}

		// 默认采集间隔为 10 秒
		if target.Interval <= 0 {
			target.Interval = 10
		}

		// 如果未设置 enabled，默认启用（目标级别，与全局不同）
		if !viper.IsSet(fmt.Sprintf("scraper.targets.%d.enabled", i)) {
			target.Enabled = true
		}
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

// setMonitorDefaults 设置 monitor 配置的默认值
// 如果未配置告警间隔，则默认为 6 小时（21600 秒）
// 如果未配置报告相关选项，默认启用所有报告并设置时间为早上七点
func setMonitorDefaults() {
	if GlobalConfig.Monitor.AlertInterval <= 0 {
		GlobalConfig.Monitor.AlertInterval = 6 * 60 * 60 // 6 小时（秒）
	}

	// 如果用户完全没有配置 daily_report，默认启用
	if !viper.IsSet("monitor.daily_report") {
		GlobalConfig.Monitor.DailyReport = true
	}

	// 如果用户完全没有配置 weekly_report，默认启用
	if !viper.IsSet("monitor.weekly_report") {
		GlobalConfig.Monitor.WeeklyReport = true
	}

	// 如果用户完全没有配置 monthly_report，默认启用
	if !viper.IsSet("monitor.monthly_report") {
		GlobalConfig.Monitor.MonthlyReport = true
	}

	// 如果用户完全没有配置 yearly_report，默认启用
	if !viper.IsSet("monitor.yearly_report") {
		GlobalConfig.Monitor.YearlyReport = true
	}

	// 如果用户配置了任何报告为 true，但未配置 report_time，默认设置为早上七点
	if (GlobalConfig.Monitor.DailyReport ||
		GlobalConfig.Monitor.WeeklyReport ||
		GlobalConfig.Monitor.MonthlyReport ||
		GlobalConfig.Monitor.YearlyReport) &&
		GlobalConfig.Monitor.ReportTime == "" {
		GlobalConfig.Monitor.ReportTime = "07:00"
	}

	// 如果未配置 AlertOnFailure，默认开启（确保报告能发送邮件）
	if !viper.IsSet("monitor.alert_on_failure") {
		GlobalConfig.Monitor.AlertOnFailure = true
	}
}
