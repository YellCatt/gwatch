package config

import (
	"fmt"
	"gwatch/internal/logger"
	"log"
	"os"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var CfgFile string

var GlobalConfig Config

func InitConfig() {
	if CfgFile != "" {
		viper.SetConfigFile(CfgFile)
	} else {
		viper.AddConfigPath("./config")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	if err := viper.Unmarshal(&GlobalConfig); err != nil {
		log.Fatalf("Unable to decode config into struct: %v", err)
	}

	setCleanerDefaults()
	setMonitorDefaults()
	setScraperDefaults()
	setSystemMonitorDefaults()

	GlobalConfig.Vars = loadRawVars()
}

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

func loadRawVars() map[string]string {
	result := make(map[string]string)

	configFile := CfgFile
	if configFile == "" {
		configFile = "./config/config.yaml"
	}

	log.Printf("[DEBUG] loadRawVars 开始，配置文件: %s", configFile)

	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Printf("[WARN] loadRawVars: 直接读取配置文件失败，将回退到 viper: %v", err)
	} else {
		log.Printf("[DEBUG] loadRawVars: 成功读取配置文件，大小: %d bytes", len(data))
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err != nil {
			log.Printf("[WARN] loadRawVars: YAML 解析失败，将回退到 viper: %v", err)
		} else {
			log.Printf("[DEBUG] loadRawVars: YAML 解析成功")
			if varsMap, ok := raw["vars"].(map[string]any); ok {
				log.Printf("[DEBUG] loadRawVars: 找到 vars 配置，数量: %d", len(varsMap))
				for k, v := range varsMap {
					switch val := v.(type) {
					case string:
						result[k] = val
						log.Printf("[DEBUG] loadRawVars: 加载变量 key=%s, value=%s", k, val)
					default:
						result[k] = fmt.Sprintf("%v", val)
						log.Printf("[DEBUG] loadRawVars: 加载变量（非字符串类型）key=%s, value=%v", k, v)
					}
				}
				log.Printf("[INFO] loadRawVars 完成（直接读取），变量数量: %d", len(result))
				return result
			} else {
				log.Printf("[WARN] loadRawVars: vars 配置不存在或格式不正确，将回退到 viper")
			}
		}
	}

	log.Printf("[WARN] loadRawVars: 使用 viper 回退模式（键名会转为小写）")
	viperVars := viper.GetStringMapString("vars")
	for k, v := range viperVars {
		result[k] = v
		log.Printf("[DEBUG] loadRawVars: viper 模式加载变量 key=%s, value=%s", k, v)
	}
	log.Printf("[INFO] loadRawVars 完成（viper 回退），变量数量: %d", len(result))
	return result
}