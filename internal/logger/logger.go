// Package logger 提供日志记录功能
// 使用 zap 日志库实现结构化日志，支持控制台和文件输出
package logger

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"pipetGo/internal/timeutil"
)

// log 是全局日志实例

var log *zap.Logger

// LogConfig 表示日志配置
type LogConfig struct {
	Level    string // 日志级别 (debug/info/warn/error)
	Encoding string // 日志格式 (json/console)
	Output   string // 输出位置 (stdout 或文件路径)
}

// InitLogger 初始化日志系统
// cfg: 日志配置
func InitLogger(cfg LogConfig) {
	var zapConfig zap.Config

	// 根据编码格式选择配置
	switch cfg.Encoding {
	case "console":
		zapConfig = zap.NewDevelopmentConfig()
	default:
		zapConfig = zap.NewProductionConfig()
	}

	// 设置日志级别
	zapConfig.Level = zap.NewAtomicLevelAt(getLogLevel(cfg.Level))
	zapConfig.Encoding = cfg.Encoding

	// 设置输出路径
	var outputPaths []string
	if cfg.Output == "stdout" {
		outputPaths = []string{"stdout"}
	} else {
		logPath := addTimestampToFilename(cfg.Output)
		ensureDir(logPath)
		outputPaths = []string{logPath}
	}
	zapConfig.OutputPaths = outputPaths

	// 设置时间格式（东八区）
	zapConfig.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(timeutil.FormatDateTimeMs(t))
	}


	// 构建日志实例
	var err error
	log, err = zapConfig.Build()
	if err != nil {
		zap.L().Fatal("Failed to initialize logger", zap.Error(err))
		os.Exit(1)
	}

	// 设置全局日志实例
	zap.ReplaceGlobals(log)
}

// addTimestampToFilename 为日志文件名添加时间戳（东八区）
// path: 原始文件路径
// 返回: 添加时间戳后的文件路径
func addTimestampToFilename(path string) string {
	dir := filepath.Dir(path)
	filename := filepath.Base(path)

	ext := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, ext)

	// 使用东八区时间
	timestamp := timeutil.FormatCompact(timeutil.Now())

	return filepath.Join(dir, nameWithoutExt+"_"+timestamp+ext)

}

// ensureDir 确保日志目录存在
// path: 文件路径
func ensureDir(path string) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		os.MkdirAll(dir, 0755)
	}
}

// getLogLevel 将字符串日志级别转换为 zapcore.Level
// level: 日志级别字符串
// 返回: zapcore.Level 枚举值
func getLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "dpanic":
		return zapcore.DPanicLevel
	case "panic":
		return zapcore.PanicLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// Debug 记录调试级别日志
func Debug(msg string, fields ...zap.Field) {
	log.Debug(msg, fields...)
}

// Info 记录信息级别日志
func Info(msg string, fields ...zap.Field) {
	log.Info(msg, fields...)
}

// Warn 记录警告级别日志
func Warn(msg string, fields ...zap.Field) {
	log.Warn(msg, fields...)
}

// Error 记录错误级别日志
func Error(msg string, fields ...zap.Field) {
	log.Error(msg, fields...)
}

// DPanic 记录调试panic级别日志（仅在开发模式下panic）
func DPanic(msg string, fields ...zap.Field) {
	log.DPanic(msg, fields...)
}

// Panic 记录panic级别日志（会触发panic）
func Panic(msg string, fields ...zap.Field) {
	log.Panic(msg, fields...)
}

// Fatal 记录致命级别日志（会调用os.Exit(1)）
func Fatal(msg string, fields ...zap.Field) {
	log.Fatal(msg, fields...)
}

// Sync 刷新日志缓冲区
func Sync() error {
	return log.Sync()
}