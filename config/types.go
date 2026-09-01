// Package config 定义全局配置结构体及各子模块的配置项。
// 所有结构体均通过 Viper 从 config.yaml 反序列化填充，并可通过 defaults.go 补齐默认值。
package config

// Config 全局配置根结构体，聚合 gwatch 各子模块的配置。
// 每个子结构体对应 YAML 中的一个顶级配置段。
type Config struct {
	// Target 目标服务配置（基础 URL、默认超时等）
	Target TargetConfig `mapstructure:"target"`
	// Log 日志模块配置（级别、编码、输出路径、轮转大小）
	Log LogConfig `mapstructure:"log"`
	// App 应用级通用配置（目录、全局前置/后置脚本、主机名）
	App AppConfig `mapstructure:"app"`
	// HTTP HTTP 客户端配置（TLS 跳过校验等）
	HTTP HTTPConfig `mapstructure:"http"`
	// Email 邮件模块配置（SMTP 服务器、收件人、各类告警冷却时间）
	Email EmailConfig `mapstructure:"email"`
	// Cleaner 清理器配置（日志保留天数、清理间隔、路径模式匹配）
	Cleaner CleanupConfig `mapstructure:"cleaner"`
	// Monitor 接口监控配置（开关、默认间隔、并发数、各类定期报告开关）
	Monitor MonitorConfig `mapstructure:"monitor"`
	// Scraper 远程指标采集器配置（目标列表、采集周期等）
	Scraper ScraperConfig `mapstructure:"scraper"`
	// SystemMon 本机系统资源监控配置（CPU/内存/磁盘/网络阈值等）
	SystemMon SystemMonitorConfig `mapstructure:"sys_monitor"`
	// Vars 用户自定义变量表，用于模板变量替换（如 {{base_url}}）
	Vars map[string]string `mapstructure:"vars"`
}

// HTTPConfig HTTP 相关配置。
type HTTPConfig struct {
	// InsecureSkipVerify 是否跳过 TLS 证书校验（仅用于测试环境）
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
}

// TargetConfig 被测目标服务的基础配置。
type TargetConfig struct {
	// BaseURL 目标服务的基础 URL（作为 HTTP 请求的默认前缀）
	BaseURL string `mapstructure:"base_url"`
	// Timeout 默认请求超时时间（秒）
	Timeout int `mapstructure:"timeout"`
}

// ScraperMetricConfig 采集器中单个指标的配置。
// 每个目标可以包含多个指标，按名称分别判定阈值、触发告警。
type ScraperMetricConfig struct {
	// Name 指标内部名称（唯一标识）
	Name string `mapstructure:"name"`
	// Path 从响应 JSON 中提取该指标值的 JSONPath 表达式
	Path string `mapstructure:"path"`
	// Alias 指标的中文/友好名称，用于邮件展示
	Alias string `mapstructure:"alias"`
	// Unit 指标单位（如 KB/s、%、次等）
	Unit string `mapstructure:"unit"`
	// Threshold 指标告警阈值；当值 >= 阈值时触发告警
	Threshold float64 `mapstructure:"threshold"`
	// Alert 该指标是否开启告警开关
	Alert bool `mapstructure:"alert"`
	// Optional 是否为可选指标：为 true 时 JSONPath 提取失败不会整体判定为失败
	Optional bool `mapstructure:"optional"`
	// Scale 缩放因子，提取值会乘以该系数
	Scale float64 `mapstructure:"scale"`
	// AutoPercent 是否将值自动转换为百分比显示
	AutoPercent bool `mapstructure:"auto_percent"`
	// CompareOp 比较运算符：支持 >= / <= / == 等，默认 >=
	CompareOp string `mapstructure:"compare_op"`
	// AlertLevel 告警级别：CRITICAL 严重 / WARNING 警告
	AlertLevel string `mapstructure:"alert_level"`
	// Consecutive 连续 N 次超阈值才触发告警，用于防抖
	Consecutive int `mapstructure:"consecutive"`
	// WarningThreshold 预警阈值（低于告警阈值时作为 WARNING 级别）
	WarningThreshold float64 `mapstructure:"warning_threshold"`
}

// ScraperTargetConfig 单个采集目标的完整配置。
type ScraperTargetConfig struct {
	// Name 目标名称（用于日志与邮件展示）
	Name string `mapstructure:"name"`
	// URL 目标 HTTP(S) 地址
	URL string `mapstructure:"url"`
	// Method HTTP 方法（GET/POST 等）
	Method string `mapstructure:"method"`
	// Timeout 单次请求超时，字符串格式（如 "5s"、"1000ms"）
	Timeout string `mapstructure:"timeout"`
	// Interval 采集间隔（秒）
	Interval int `mapstructure:"interval"`
	// Enabled 目标是否启用采集
	Enabled bool `mapstructure:"enabled"`
	// Headers 附加的 HTTP 请求头
	Headers map[string]string `mapstructure:"headers"`
	// Body POST/PUT 请求体
	Body string `mapstructure:"body"`
	// InsecureSkipVerify 针对此目标是否跳过 TLS 证书校验
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
	// Proxy 代理地址（如 http://proxy:8080）
	Proxy string `mapstructure:"proxy"`
	// Metrics 该目标下的指标列表
	Metrics []ScraperMetricConfig `mapstructure:"metrics"`
}

// ScraperConfig 采集器模块的总配置。
type ScraperConfig struct {
	// Enabled 采集器总开关
	Enabled bool `mapstructure:"enabled"`
	// Targets 采集目标列表
	Targets []ScraperTargetConfig `mapstructure:"targets"`
}

// LogConfig 日志模块配置。
type LogConfig struct {
	// Level 日志级别：debug / info / warn / error
	Level string `mapstructure:"level"`
	// Encoding 输出格式：json / console
	Encoding string `mapstructure:"encoding"`
	// Output 日志文件路径（或 stdout 输出到控制台）
	Output string `mapstructure:"output"`
	// MaxSizeMB 单个日志文件最大体积（MB），超出后自动轮转
	MaxSizeMB int `mapstructure:"max_size_mb"`
}

// AppConfig 应用级通用配置。
type AppConfig struct {
	// ReportDir 报告存放目录
	ReportDir string `mapstructure:"report_dir"`
	// CaseDir 测试用例（PSV 文件）存放目录
	CaseDir string `mapstructure:"case_dir"`
	// DataDir 结构化数据（CSV/SQLite）存放目录
	DataDir string `mapstructure:"data_dir"`

	// GlobalPre 全局前置脚本列表（在所有测试用例前执行）
	GlobalPre []string `mapstructure:"global_pre"`
	// GlobalPost 全局后置脚本列表（在所有测试用例后执行）
	GlobalPost []string `mapstructure:"global_post"`
	// HostName 当前主机名，用于告警邮件标识
	HostName string `mapstructure:"host_name"`
}

// EmailConfig 邮件模块配置。
type EmailConfig struct {
	// Enabled 邮件发送总开关
	Enabled bool `mapstructure:"enabled"`
	// From 发件人邮箱地址
	From string `mapstructure:"from"`
	// To 收件人邮箱地址列表
	To []string `mapstructure:"to"`
	// AuthCode 邮箱授权码（用于 SMTP 登录）
	AuthCode string `mapstructure:"auth_code"`
	// SMTPServer SMTP 服务器地址
	SMTPServer string `mapstructure:"smtp_server"`
	// SMTPPort SMTP 服务器端口
	SMTPPort int `mapstructure:"smtp_port"`
	// ErrorSubject 异常报告邮件标题模板，支持 {{device}}、{{time}} 占位符
	ErrorSubject string `mapstructure:"error_subject"`
	// ScraperCooldown 采集器类告警冷却时间（秒）
	ScraperCooldown int `mapstructure:"scraper_cooldown"`
	// APICooldown API 监控类告警冷却时间（秒）
	APICooldown int `mapstructure:"api_cooldown"`
	// SystemCooldown 系统监控类告警冷却时间（秒）
	SystemCooldown int `mapstructure:"system_cooldown"`
}

// CleanupConfig 清理器配置，用于定期清理过期日志/报告/数据文件。
type CleanupConfig struct {
	// Enabled 清理任务开关
	Enabled bool `mapstructure:"enabled"`
	// RetentionDays 保留天数，超过此天数的文件将被删除
	RetentionDays int `mapstructure:"retention_days"`
	// LogDir 日志目录
	LogDir string `mapstructure:"log_dir"`
	// ReportDir 报告目录
	ReportDir string `mapstructure:"report_dir"`
	// DataDir 已废弃：数据存储目录不再参与清理（其中 CSV 为业务数据，
	// 且月/年级指标写入频率低于保留天数，按 mtime 清理会误删）。
	// 保留字段仅用于兼容旧配置文件中的 cleaner.data_dir，运行时会被忽略。
	DataDir string `mapstructure:"data_dir"`
	// IncludePatterns 参与清理的文件 glob 模式
	IncludePatterns []string `mapstructure:"include_patterns"`
	// ExcludePatterns 排除的文件 glob 模式
	ExcludePatterns []string `mapstructure:"exclude_patterns"`
	// IntervalHours 清理任务执行间隔（小时）
	IntervalHours int `mapstructure:"interval_hours"`
}

// MonitorConfig API 接口监控模块配置。
type MonitorConfig struct {
	// Enabled 接口监控总开关
	Enabled bool `mapstructure:"enabled"`
	// DefaultInterval 未单独配置 interval 的用例使用的默认监控间隔（秒）
	DefaultInterval int `mapstructure:"default_interval"`
	// AlertOnFailure 接口执行失败是否触发告警
	AlertOnFailure bool `mapstructure:"alert_on_failure"`
	// AlertOnSlow 接口响应慢是否触发告警
	AlertOnSlow bool `mapstructure:"alert_on_slow"`
	// MaxWorkers 最大并发 worker 数量
	MaxWorkers int `mapstructure:"max_workers"`
	// AlertInterval 同一告警的最小发送间隔（秒），用于告警去重
	AlertInterval int `mapstructure:"alert_interval"`
	// DailyReport 启用日报
	DailyReport bool `mapstructure:"daily_report"`
	// WeeklyReport 启用周报
	WeeklyReport bool `mapstructure:"weekly_report"`
	// MonthlyReport 启用月报
	MonthlyReport bool `mapstructure:"monthly_report"`
	// YearlyReport 启用年报
	YearlyReport bool `mapstructure:"yearly_report"`
	// DailyAllReports 测试模式：每天都生成周/月/年报告，忽略周期日期限制
	DailyAllReports bool `mapstructure:"daily_all_reports"`
	// ReportTime 定期报告生成时间（HH:MM）
	ReportTime string `mapstructure:"report_time"`
}

// SystemMonitorConfig 本机系统资源监控模块配置。
type SystemMonitorConfig struct {
	// Enabled 系统监控总开关
	Enabled bool `mapstructure:"enabled"`
	// Interval 指标采集间隔（秒）
	Interval int `mapstructure:"interval"`
	// ChartEnabled 是否在系统报告中生成 ASCII 图表
	ChartEnabled bool `mapstructure:"chart_enabled"`
	// EmailEnabled 是否开启系统告警邮件
	EmailEnabled bool `mapstructure:"email_enabled"`
	// CPUThreshold CPU 使用率告警阈值（百分比）
	CPUThreshold float64 `mapstructure:"cpu_threshold"`
	// MemoryThreshold 内存使用率告警阈值（百分比）
	MemoryThreshold float64 `mapstructure:"memory_threshold"`
	// DiskUsageThreshold 磁盘使用率告警阈值（百分比）
	DiskUsageThreshold float64 `mapstructure:"disk_usage_threshold"`
	// NetworkDownThreshold 下行网络流量告警阈值（KB/s）
	NetworkDownThreshold float64 `mapstructure:"network_down_threshold"`
	// NetworkUpThreshold 上行网络流量告警阈值（KB/s）
	NetworkUpThreshold float64 `mapstructure:"network_up_threshold"`
	// NetworkDownWarnThreshold 下行网络预警阈值（KB/s，用于 WARNING 级别）
	NetworkDownWarnThreshold float64 `mapstructure:"network_down_warn_threshold"`
	// NetworkUpWarnThreshold 上行网络预警阈值（KB/s，用于 WARNING 级别）
	NetworkUpWarnThreshold float64 `mapstructure:"network_up_warn_threshold"`
	// AlertCooldown 系统告警冷却时间（秒）
	AlertCooldown int `mapstructure:"alert_cooldown"`
}
