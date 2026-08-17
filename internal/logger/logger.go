package logger

import (
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"gwatch/internal/timeutil"
)

var log *zap.Logger

var atomicLevel zap.AtomicLevel

type LogConfig struct {
	Level     string
	Encoding  string
	Output    string
	MaxSizeMB int
}

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
		writeSyncer = zapcore.AddSync(os.Stdout)
	} else {
		rotationWriter, err := NewDailyRotationWriter(cfg.Output, cfg.MaxSizeMB)
		if err != nil {
			zap.L().Fatal("Failed to create log rotation writer", zap.Error(err))
			os.Exit(1)
		}
		writeSyncer = zapcore.AddSync(rotationWriter)
	}

	core := zapcore.NewCore(encoder, writeSyncer, atomicLevel)

	if cfg.Encoding == "console" {
		log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.WarnLevel))
	} else {
		log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	}

	zap.ReplaceGlobals(log)
}

func ensureDir(path string) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		os.MkdirAll(dir, 0755)
	}
}

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

func Debug(msg string, fields ...zap.Field) {
	log.Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	log.Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	log.Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	log.Error(msg, fields...)
}

func DPanic(msg string, fields ...zap.Field) {
	log.DPanic(msg, fields...)
}

func Panic(msg string, fields ...zap.Field) {
	log.Panic(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	log.Fatal(msg, fields...)
}

func Sync() error {
	return log.Sync()
}

func SetLogLevel(level string) {
	atomicLevel.SetLevel(getLogLevel(level))
}

func GetLogLevel() string {
	return atomicLevel.Level().String()
}