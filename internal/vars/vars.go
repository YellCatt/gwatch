// Package vars 提供全局变量管理功能
// 支持变量的设置、获取和字符串替换
package vars

import (
	"regexp"
	"strings"
	"sync"

	"go.uber.org/zap"

	"gwatch/internal/logger"
)

// vars 存储全局变量，使用 map 实现
var (
	vars                 = make(map[string]string)
	varsMu               sync.Mutex
	globalPreVariables   = make(map[string]bool) // 记录全局前置脚本提取的变量
	globalPreVariablesMu sync.Mutex

	// reservedKeys 断言专用指令，不作为变量替换
	reservedKeys = map[string]bool{
		"skip":       true,
		"not_exists": true,
	}
)

// Set 设置全局变量
// key: 变量名
// value: 变量值
func Set(key, value string) {
	varsMu.Lock()
	defer varsMu.Unlock()
	vars[key] = value
}

// Get 获取全局变量
// key: 变量名
// 返回: 变量值（如果不存在返回空字符串）
func Get(key string) string {
	varsMu.Lock()
	defer varsMu.Unlock()
	return vars[key]
}

// GetAll 获取所有全局变量
// 返回: 包含所有变量的 map 副本
func GetAll() map[string]string {
	varsMu.Lock()
	defer varsMu.Unlock()
	result := make(map[string]string)
	for k, v := range vars {
		result[k] = v
	}
	return result
}

// Delete 删除指定变量
// key: 变量名
func Delete(key string) {
	varsMu.Lock()
	defer varsMu.Unlock()
	delete(vars, key)
}

// Replace 替换字符串中的变量引用
// text: 包含变量引用的字符串（格式: {{var}}）
// 返回: 替换后的字符串
// 注意：变量名必须完全匹配（大小写敏感）
func Replace(text string) string {
	if text == "" {
		logger.Debug("变量替换：输入为空字符串")
		return text
	}

	logger.Debug("变量替换：输入文本", zap.String("text", text))
	logger.Debug("变量替换：当前变量池", zap.Any("vars", GetAll()))

	re := regexp.MustCompile(`\{\{([a-zA-Z][a-zA-Z0-9_]*)\}\}`)
	result := re.ReplaceAllStringFunc(text, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")
		if reservedKeys[key] {
			logger.Debug("变量替换：跳过断言指令", zap.String("match", match))
			return match
		}
		logger.Debug("变量替换：发现变量引用", zap.String("match", match), zap.String("key", key))
		if value, ok := vars[key]; ok {
			logger.Info("变量替换成功", zap.String("key", key), zap.String("value", maskValue(value)))
			return value
		}
		logger.Info("变量替换失败：变量未找到", zap.String("key", key), zap.String("match", match))
		return match
	})

	if result != text {
		logger.Info("变量替换完成", zap.String("before", text), zap.String("after", maskValue(result)))
	} else {
		logger.Debug("变量替换：无变量被替换", zap.String("text", text))
	}
	return result
}

// maskValue 对长度较长的值做掩码，避免日志泄露完整 token
func maskValue(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:6] + "***" + s[len(s)-6:]
}

// InitFromConfig 从配置初始化变量
// config: 配置 map
func InitFromConfig(config map[string]string) {
	varsMu.Lock()
	defer varsMu.Unlock()
	for k, v := range config {
		vars[k] = v
	}
}

// MarkAsGlobalPre 标记变量为全局前置脚本提取的变量
func MarkAsGlobalPre(key string) {
	globalPreVariablesMu.Lock()
	defer globalPreVariablesMu.Unlock()
	globalPreVariables[key] = true
}

// CleanupGlobalPreVariables 清理所有全局前置脚本提取的变量
func CleanupGlobalPreVariables() {
	globalPreVariablesMu.Lock()
	defer globalPreVariablesMu.Unlock()

	varsMu.Lock()
	defer varsMu.Unlock()

	for key := range globalPreVariables {
		delete(vars, key)
		logger.Debug("清理全局前置变量", zap.String("name", key))
	}

	// 清空记录
	globalPreVariables = make(map[string]bool)
}