package config

type Config struct {
	Target    TargetConfig        `mapstructure:"target"`
	Log       LogConfig           `mapstructure:"log"`
	App       AppConfig           `mapstructure:"app"`
	HTTP      HTTPConfig          `mapstructure:"http"`
	Email     EmailConfig         `mapstructure:"email"`
	Cleaner   CleanupConfig       `mapstructure:"cleaner"`
	Monitor   MonitorConfig       `mapstructure:"monitor"`
	Scraper   ScraperConfig       `mapstructure:"scraper"`
	SystemMon SystemMonitorConfig `mapstructure:"sys_monitor"`
	Vars      map[string]string   `mapstructure:"vars"`
}

type HTTPConfig struct {
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
}

type TargetConfig struct {
	BaseURL string `mapstructure:"base_url"`
	Timeout int    `mapstructure:"timeout"`
}

type ScraperMetricConfig struct {
	Name             string  `mapstructure:"name"`
	Path             string  `mapstructure:"path"`
	Alias            string  `mapstructure:"alias"`
	Unit             string  `mapstructure:"unit"`
	Threshold        float64 `mapstructure:"threshold"`
	Alert            bool    `mapstructure:"alert"`
	Optional         bool    `mapstructure:"optional"`
	Scale            float64 `mapstructure:"scale"`
	AutoPercent      bool    `mapstructure:"auto_percent"`
	CompareOp        string  `mapstructure:"compare_op"`
	AlertLevel       string  `mapstructure:"alert_level"`
	Consecutive      int     `mapstructure:"consecutive"`
	WarningThreshold float64 `mapstructure:"warning_threshold"`
}

type ScraperTargetConfig struct {
	Name               string                `mapstructure:"name"`
	URL                string                `mapstructure:"url"`
	Method             string                `mapstructure:"method"`
	Timeout            string                `mapstructure:"timeout"`
	Interval           int                   `mapstructure:"interval"`
	Enabled            bool                  `mapstructure:"enabled"`
	Headers            map[string]string     `mapstructure:"headers"`
	Body               string                `mapstructure:"body"`
	InsecureSkipVerify bool                  `mapstructure:"insecure_skip_verify"`
	Proxy              string                `mapstructure:"proxy"`
	Metrics            []ScraperMetricConfig `mapstructure:"metrics"`
}

type ScraperConfig struct {
	Enabled bool                  `mapstructure:"enabled"`
	Targets []ScraperTargetConfig `mapstructure:"targets"`
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Encoding   string `mapstructure:"encoding"`
	Output     string `mapstructure:"output"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
}

type AppConfig struct {
	ReportDir string `mapstructure:"report_dir"`
	CaseDir   string `mapstructure:"case_dir"`
	DataDir   string `mapstructure:"data_dir"`

	GlobalPre  []string `mapstructure:"global_pre"`
	GlobalPost []string `mapstructure:"global_post"`
	HostName   string   `mapstructure:"host_name"`
}

type EmailConfig struct {
	Enabled         bool     `mapstructure:"enabled"`
	From            string   `mapstructure:"from"`
	To              []string `mapstructure:"to"`
	AuthCode        string   `mapstructure:"auth_code"`
	SMTPServer      string   `mapstructure:"smtp_server"`
	SMTPPort        int      `mapstructure:"smtp_port"`
	ErrorSubject    string   `mapstructure:"error_subject"`
	ScraperCooldown int      `mapstructure:"scraper_cooldown"`
	APICooldown     int      `mapstructure:"api_cooldown"`
	SystemCooldown  int      `mapstructure:"system_cooldown"`
}

type CleanupConfig struct {
	Enabled         bool     `mapstructure:"enabled"`
	RetentionDays   int      `mapstructure:"retention_days"`
	LogDir          string   `mapstructure:"log_dir"`
	ReportDir       string   `mapstructure:"report_dir"`
	DataDir         string   `mapstructure:"data_dir"`
	IncludePatterns []string `mapstructure:"include_patterns"`
	ExcludePatterns []string `mapstructure:"exclude_patterns"`
	IntervalHours   int      `mapstructure:"interval_hours"`
}

type MonitorConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	DefaultInterval int    `mapstructure:"default_interval"`
	AlertOnFailure  bool   `mapstructure:"alert_on_failure"`
	AlertOnSlow     bool   `mapstructure:"alert_on_slow"`
	MaxWorkers      int    `mapstructure:"max_workers"`
	AlertInterval   int    `mapstructure:"alert_interval"`
	DailyReport     bool   `mapstructure:"daily_report"`
	WeeklyReport    bool   `mapstructure:"weekly_report"`
	MonthlyReport   bool   `mapstructure:"monthly_report"`
	YearlyReport    bool   `mapstructure:"yearly_report"`
	ReportTime      string `mapstructure:"report_time"`
}

type SystemMonitorConfig struct {
	Enabled                  bool    `mapstructure:"enabled"`
	Interval                 int     `mapstructure:"interval"`
	ChartEnabled             bool    `mapstructure:"chart_enabled"`
	EmailEnabled             bool    `mapstructure:"email_enabled"`
	CPUThreshold             float64 `mapstructure:"cpu_threshold"`
	MemoryThreshold          float64 `mapstructure:"memory_threshold"`
	DiskUsageThreshold       float64 `mapstructure:"disk_usage_threshold"`
	NetworkDownThreshold     float64 `mapstructure:"network_down_threshold"`
	NetworkUpThreshold       float64 `mapstructure:"network_up_threshold"`
	NetworkDownWarnThreshold float64 `mapstructure:"network_down_warn_threshold"`
	NetworkUpWarnThreshold   float64 `mapstructure:"network_up_warn_threshold"`
	AlertCooldown            int     `mapstructure:"alert_cooldown"`
}