package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"gwatch/internal/logger"
)

var CfgFile string

var GlobalConfig Config

// InitConfig 初始化应用配置：读取 YAML 配置文件、解析到 GlobalConfig 结构体、设置默认值和加载变量。
func InitConfig() {
	if CfgFile != "" {
		viper.SetConfigFile(CfgFile)
	} else {
		viper.AddConfigPath("./config")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Fprintln(os.Stderr, "Error reading config file:", err)
		os.Exit(1)
	}

	if err := viper.Unmarshal(&GlobalConfig); err != nil {
		fmt.Fprintln(os.Stderr, "Unable to decode config into struct:", err)
		os.Exit(1)
	}

	setCleanerDefaults()
	setMonitorDefaults()
	setScraperDefaults()
	setSystemMonitorDefaults()

	GlobalConfig.Vars = loadRawVars()
}

// ReloadConfig 重新加载配置文件并返回日志级别是否发生了变化。
func ReloadConfig() bool {
	oldLogLevel := GlobalConfig.Log.Level

	if err := viper.ReadInConfig(); err != nil {
		logger.Warn("Failed to reload config file", zap.Error(err))
		return false
	}

	if err := viper.Unmarshal(&GlobalConfig); err != nil {
		logger.Warn("Failed to unmarshal config", zap.Error(err))
		return false
	}

	setCleanerDefaults()
	setMonitorDefaults()
	setScraperDefaults()
	setSystemMonitorDefaults()

	GlobalConfig.Vars = loadRawVars()

	return oldLogLevel != GlobalConfig.Log.Level
}

// loadRawVars 从配置文件直接读取 vars 变量映射，优先使用原生 YAML 解析以保留变量名大小写。
func loadRawVars() map[string]string {
	result := make(map[string]string)

	configFile := CfgFile
	if configFile == "" {
		configFile = "./config/config.yaml"
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		zap.L().Warn("loadRawVars: read config file failed, falling back to viper", zap.Error(err))
	} else {
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err != nil {
			zap.L().Warn("loadRawVars: YAML unmarshal failed, falling back to viper", zap.Error(err))
		} else if varsMap, ok := raw["vars"].(map[string]any); ok {
			for k, v := range varsMap {
				switch val := v.(type) {
				case string:
					result[k] = val
				default:
					result[k] = fmt.Sprintf("%v", val)
				}
			}
			zap.L().Info("loadRawVars done (direct read)", zap.Int("count", len(result)))
			return result
		} else {
			zap.L().Warn("loadRawVars: vars section missing or invalid, falling back to viper")
		}
	}

	zap.L().Warn("loadRawVars: using viper fallback (keys will be lowercased)")
	viperVars := viper.GetStringMapString("vars")
	for k, v := range viperVars {
		result[k] = v
	}
	zap.L().Info("loadRawVars done (viper fallback)", zap.Int("count", len(result)))
	return result
}
