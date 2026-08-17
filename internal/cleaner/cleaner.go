// Package cleaner 提供日志和测试报告的自动清理功能
package cleaner

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

// Config 清理配置（与 config.go 中的 CleanupConfig 保持一致）
type Config struct {
	Enabled         bool     `mapstructure:"enabled"`          // 是否启用自动清理
	RetentionDays   int      `mapstructure:"retention_days"`   // 文件保留天数
	LogDir          string   `mapstructure:"log_dir"`          // 日志目录
	ReportDir       string   `mapstructure:"report_dir"`       // 测试报告目录
	DataDir         string   `mapstructure:"data_dir"`         // 数据目录
	IncludePatterns []string `mapstructure:"include_patterns"` // 要清理的文件模式列表
	ExcludePatterns []string `mapstructure:"exclude_patterns"` // 排除的文件模式列表
	IntervalHours   int      `mapstructure:"interval_hours"`   // 定时清理间隔（小时）
}

// Cleaner 清理器
type Cleaner struct {
	config    Config
	stopChan  chan struct{}
	running   bool
}

// NewCleaner 创建清理器实例
func NewCleaner(config Config) *Cleaner {
	return &Cleaner{
		config:   config,
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

	c.running = true
	interval := time.Duration(c.config.IntervalHours) * time.Hour
	logger.Info("启动清理器",
		zap.Int("retention_days", c.config.RetentionDays),
		zap.String("log_dir", c.config.LogDir),
		zap.String("report_dir", c.config.ReportDir),
		zap.String("data_dir", c.config.DataDir),
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

// setDefaults 设置默认值
func (c *Cleaner) setDefaults() {
	if c.config.RetentionDays <= 0 {
		c.config.RetentionDays = 30
	}
	if c.config.IntervalHours <= 0 {
		c.config.IntervalHours = 24
	}
	if len(c.config.IncludePatterns) == 0 {
		c.config.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
	}
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

	if c.config.DataDir != "" {
		count, err := c.cleanupDirectory(c.config.DataDir, threshold)
		if err != nil {
			logger.Warn("清理数据目录失败", zap.String("dir", c.config.DataDir), zap.Error(err))
		} else {
			totalDeleted += count
		}
	}

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