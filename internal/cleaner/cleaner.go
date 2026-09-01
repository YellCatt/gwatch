// Package cleaner 提供日志和测试报告的自动清理功能
package cleaner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

// Config 清理配置（与 config.go 中的 CleanupConfig 保持一致）
type Config struct {
	Enabled         bool     `mapstructure:"enabled"`          // 是否启用自动清理
	RetentionDays   int      `mapstructure:"retention_days"`   // 文件保留天数
	LogDir          string   `mapstructure:"log_dir"`          // 日志目录
	ReportDir       string   `mapstructure:"report_dir"`       // 测试报告目录
	DataDir         string   `mapstructure:"data_dir"`         // 已废弃：数据存储目录不再参与清理，保留字段仅为兼容旧配置
	IncludePatterns []string `mapstructure:"include_patterns"` // 要清理的文件模式列表
	ExcludePatterns []string `mapstructure:"exclude_patterns"` // 排除的文件模式列表
	IntervalHours   int      `mapstructure:"interval_hours"`   // 定时清理间隔（小时）
}

// protectedPatterns 内置强制排除的文件模式。
//
// 这些文件承载系统指标与调度状态，属于持续使用的业务数据，删除后无法再生，
// 且部分文件（如年级指标 CSV）写入频率低于保留天数，按 mtime 判断必然被误删。
// 因此无论用户如何配置 include_patterns，这些文件都不会被清理器删除。
var protectedPatterns = []string{
	"system_metrics_*.csv", // 系统指标小时/日/月/年聚合数据
	"csv_index.csv",        // 存储层索引文件
	"last_report_sent.txt", // 报告调度去重状态，删除会导致重复发送报告
}

// Cleaner 清理器
type Cleaner struct {
	config    Config
	stopChan  chan struct{}
	running   bool
}

// NewCleaner 创建清理器实例
func NewCleaner(cfg Config) *Cleaner {
	return &Cleaner{
		config:   cfg,
		stopChan: make(chan struct{}),
	}
}

// Start 启动定时清理任务
func (c *Cleaner) Start() error {
	if !c.config.Enabled {
		logger.Info("清理器已禁用，跳过启动")
		return nil
	}

	if c.running {
		return fmt.Errorf("清理器已在运行")
	}

	c.setDefaults()

	if c.config.DataDir != "" {
		logger.Warn("cleaner.data_dir 已废弃：数据存储目录包含系统指标与告警历史等业务数据，不再参与清理",
			zap.String("data_dir", c.config.DataDir))
	}

	c.running = true
	interval := time.Duration(c.config.IntervalHours) * time.Hour
	logger.Info("启动清理器",
		zap.Int("retention_days", c.config.RetentionDays),
		zap.String("log_dir", c.config.LogDir),
		zap.String("report_dir", c.config.ReportDir),
		zap.Int("interval_hours", c.config.IntervalHours))

	// 立即执行一次清理
	go c.cleanup()

	// 启动定时任务
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.cleanup()
			case <-c.stopChan:
				logger.Info("清理器已停止")
				return
			}
		}
	}()

	return nil
}

// Stop 停止清理任务
func (c *Cleaner) Stop() {
	if !c.running {
		return
	}
	c.running = false
	close(c.stopChan)
}

// Cleanup 执行一次清理（手动调用）
func (c *Cleaner) Cleanup() error {
	if !c.config.Enabled {
		return fmt.Errorf("清理器已禁用")
	}
	c.setDefaults()
	return c.cleanup()
}

// setDefaults 设置默认值并合并内置保护模式。
//
// 默认包含模式只覆盖日志与报告产物（*.log / *.json / *.txt），不再包含 *.csv：
// CSV 是本项目的业务数据载体（指标、告警、汇总），不是可再生的日志。
func (c *Cleaner) setDefaults() {
	if c.config.RetentionDays <= 0 {
		c.config.RetentionDays = 30
	}
	if c.config.IntervalHours <= 0 {
		c.config.IntervalHours = 24
	}
	if len(c.config.IncludePatterns) == 0 {
		c.config.IncludePatterns = []string{"*.log", "*.json", "*.txt"}
	}
	c.mergeProtectedPatterns()
}

// mergeProtectedPatterns 将内置保护模式追加到排除列表末尾。
// 排除优先级高于包含，因此即使这些模式同时出现在 include_patterns 中也不会被删除。
func (c *Cleaner) mergeProtectedPatterns() {
	for _, p := range protectedPatterns {
		if !containsString(c.config.ExcludePatterns, p) {
			c.config.ExcludePatterns = append(c.config.ExcludePatterns, p)
		}
	}
}

// containsString 判断字符串切片中是否存在目标值（精确匹配）。
func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// isProtectedPath 判断文件是否位于数据存储目录（app.data_dir）之内。
//
// 数据存储目录承载系统指标、告警历史、监控汇总等业务数据，不能按修改时间清理。
// 即使使用者把 log_dir / report_dir 误配成数据目录，该目录下的文件也不会被删除。
func isProtectedPath(path string) bool {
	dataDir := strings.TrimSpace(config.GlobalConfig.App.DataDir)
	if dataDir == "" {
		return false
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	// rel 为 "." 表示路径就是数据目录本身；
	// 不以 ".." 开头且非绝对路径，表示位于数据目录之内。
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

// cleanup 执行实际的清理操作
func (c *Cleaner) cleanup() error {
	logger.Info("开始清理任务")
	threshold := timeutil.Now().Add(-time.Duration(c.config.RetentionDays) * 24 * time.Hour)

	totalDeleted := 0

	if c.config.LogDir != "" {
		count, err := c.cleanupDirectory(c.config.LogDir, threshold)
		if err != nil {
			logger.Warn("清理日志目录失败", zap.String("dir", c.config.LogDir), zap.Error(err))
		} else {
			totalDeleted += count
		}
	}

	if c.config.ReportDir != "" {
		count, err := c.cleanupDirectory(c.config.ReportDir, threshold)
		if err != nil {
			logger.Warn("清理报告目录失败", zap.String("dir", c.config.ReportDir), zap.Error(err))
		} else {
			totalDeleted += count
		}
	}

	// 数据存储目录（app.data_dir）不参与清理：
	// 其中的系统指标 CSV、告警历史、汇总记录均属于业务数据，
	// 且年级/月级指标写入频率低于保留天数，按 mtime 判断必然被误删。

	logger.Info("清理任务完成", zap.Int("files_deleted", totalDeleted))
	return nil
}

// cleanupDirectory 清理指定目录中超过阈值时间的文件
func (c *Cleaner) cleanupDirectory(dir string, threshold time.Time) (int, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logger.Debug("目录不存在，跳过", zap.String("dir", dir))
		return 0, nil
	}

	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// 数据存储目录整体豁免，优先于包含模式判断
		if isProtectedPath(path) {
			return nil
		}

		if !c.matchesIncludePatterns(path) {
			return nil
		}

		if c.matchesExcludePatterns(path) {
			return nil
		}

		if info.ModTime().Before(threshold) {
			if err := os.Remove(path); err != nil {
				logger.Warn("删除文件失败", zap.String("path", path), zap.Error(err))
				return err
			}
			count++
			logger.Info("已删除旧文件", zap.String("path", path), zap.Time("mod_time", info.ModTime()))
		}

		return nil
	})

	return count, err
}

// matchesIncludePatterns 检查文件是否匹配包含模式
func (c *Cleaner) matchesIncludePatterns(path string) bool {
	if len(c.config.IncludePatterns) == 0 {
		return true
	}

	baseName := filepath.Base(path)
	for _, pattern := range c.config.IncludePatterns {
		matched, err := filepath.Match(pattern, baseName)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// matchesExcludePatterns 检查文件是否匹配排除模式
func (c *Cleaner) matchesExcludePatterns(path string) bool {
	if len(c.config.ExcludePatterns) == 0 {
		return false
	}

	baseName := filepath.Base(path)
	for _, pattern := range c.config.ExcludePatterns {
		matched, err := filepath.Match(pattern, baseName)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// GetConfig 获取当前配置
func (c *Cleaner) GetConfig() Config {
	return c.config
}

// IsRunning 检查清理器是否正在运行
func (c *Cleaner) IsRunning() bool {
	return c.running
}

// ExtractLogDir 从日志输出路径中提取目录路径
// 例如："./logs/pipet.log" -> "./logs"
func ExtractLogDir(logOutputPath string) string {
	if logOutputPath == "" || logOutputPath == "stdout" || logOutputPath == "stderr" {
		return ""
	}
	return filepath.Dir(logOutputPath)
}