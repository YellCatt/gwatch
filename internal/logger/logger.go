// Package logger 基于 zap 实现的日志模块。
// 支持多级别日志、JSON/Console 编码、按天/按大小轮转、热切换日志级别。
package logger

import (
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"gwatch/internal/timeutil"
)

// log 全局 zap 日志实例，通过 zap.ReplaceGlobals 设置为全局。
var log *zap.Logger

// atomicLevel 原子日志级别，支持运行时动态切换。
var atomicLevel zap.AtomicLevel

// LogConfig 日志模块初始化配置。
type LogConfig struct {
	// Level 日志级别（debug/info/warn/error/...）
	Level string
	// Encoding 输出编码：json / console
	Encoding string
	// Output 日志输出路径；为 "stdout" 时输出到标准输出
	Output string
	// MaxSizeMB 单个日志文件最大体积（MB）
	MaxSizeMB int
}

// InitLogger 初始化日志模块，写入路径、编码、级别、轮转等配置。
// 初始化失败会直接 Fatal 并退出进程。
func InitLogger(cfg LogConfig) {
	atomicLevel = zap.NewAtomicLevelAt(getLogLevel(cfg.Level))

	var encoderConfig zapcore.EncoderConfig
	var encoder zapcore.Encoder

	switch cfg.Encoding {
	case "console":
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(timeutil.FormatDateTimeMs(t))
		}
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	default:
		encoderConfig = zap.NewProductionEncoderConfig()
		encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(timeutil.FormatDateTimeMs(t))
		}
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	var writeSyncer zapcore.WriteSyncer
	if cfg.Output == "stdout" {
		// 直接输出到标准输出，常用于本地调试
		writeSyncer = zapcore.AddSync(os.Stdout)
	} else {
		// 按天 + 按大小轮转的文件写入器
		rotationWriter, err := NewDailyRotationWriter(cfg.Output, cfg.MaxSizeMB)
		if err != nil {
			zap.L().Fatal("Failed to create log rotation writer", zap.Error(err))
			os.Exit(1)
		}
		writeSyncer = zapcore.AddSync(rotationWriter)
	}

	core := zapcore.NewCore(encoder, writeSyncer, atomicLevel)

	// console 编码下在 Warn 级别触发堆栈；json 编码仅在 Error 级别触发
	if cfg.Encoding == "console" {
		log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.WarnLevel))
	} else {
		log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	}

	zap.ReplaceGlobals(log)
}

// ensureDir 确保日志文件所在目录存在。
func ensureDir(path string) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		os.MkdirAll(dir, 0755)
	}
}

// getLogLevel 将字符串级别转换为 zapcore.Level；未识别时默认 Info。
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

// Debug 输出 debug 级别日志
func Debug(msg string, fields ...zap.Field) {
	log.Debug(msg, fields...)
}

// Info 输出 info 级别日志
func Info(msg string, fields ...zap.Field) {
	log.Info(msg, fields...)
}

// Warn 输出 warn 级别日志
func Warn(msg string, fields ...zap.Field) {
	log.Warn(msg, fields...)
}

// Error 输出 error 级别日志
func Error(msg string, fields ...zap.Field) {
	log.Error(msg, fields...)
}

// DPanic 输出 dpanic 级别日志
func DPanic(msg string, fields ...zap.Field) {
	log.DPanic(msg, fields...)
}

// Panic 输出 panic 级别日志
func Panic(msg string, fields ...zap.Field) {
	log.Panic(msg, fields...)
}

// Fatal 输出 fatal 级别日志
func Fatal(msg string, fields ...zap.Field) {
	log.Fatal(msg, fields...)
}

// Sync 刷新日志缓冲区到磁盘（进程退出前应调用）。
func Sync() error {
	return log.Sync()
}

// SetLogLevel 在运行时动态切换日志级别（热加载配置时使用）。
func SetLogLevel(level string) {
	atomicLevel.SetLevel(getLogLevel(level))
}

// GetLogLevel 返回当前日志级别字符串表示。
func GetLogLevel() string {
	return atomicLevel.Level().String()
}
