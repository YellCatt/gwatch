package monitor

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
	"gwatch/internal/psv"
)

// startHotReload 启动热加载协程：按 hotReloadInterval 周期扫描配置和测试用例变更，
// 直到收到停止信号。
func startHotReload() {
	ticker := time.NewTicker(hotReloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hotReload()
		case <-stopChan:
			logger.Info("Hot reload stopped")
			return
		}
	}
}

// hotReload 执行一次热加载流程：重载配置、重新解析 PSV 文件、添加新用例、
// 重启已修改用例、移除已删除用例。
func hotReload() {
	logger.Debug("Checking for hot reload changes")

	if config.ReloadConfig() {
		newLevel := config.GlobalConfig.Log.Level
		logger.SetLogLevel(newLevel)
		logger.Info("Log level updated via config file", zap.String("new_level", newLevel))
		fmt.Printf("\n[热加载] 配置文件已更新，日志级别已切换为: %s\n", newLevel)
	}

	caseDir := config.GlobalConfig.App.CaseDir
	if caseDir == "" {
		caseDir = "./demo"
	}

	newCases, err := psv.ParseFiles([]string{caseDir})
	if err != nil {
		logger.Warn("Failed to parse files during hot reload", zap.Error(err))
		return
	}

	newMonitorCases := filterMonitorCases(newCases)

	tasksMu.Lock()
	currentTaskIDs := make(map[string]bool)
	for id := range tasks {
		currentTaskIDs[id] = true
	}
	tasksMu.Unlock()

	newCount := 0
	modifiedCount := 0
	for _, tc := range newMonitorCases {
		tasksMu.Lock()
		task, exists := tasks[tc.ID]
		tasksMu.Unlock()

		if !exists {
			startTask(tc)
			newCount++
		} else {
			if isTestCaseModified(task.TestCase, tc) {
				removeTask(tc.ID)
				startTask(tc)
				modifiedCount++
				fmt.Printf("[热加载] 测试用例 %s 已修改并重启\n", tc.ID)
			}
		}
	}

	removeDeletedTestCases(newMonitorCases)

	if newCount > 0 {
		logger.Info("Hot reload completed", zap.Int("new_tasks", newCount))
		fmt.Printf("\n[热加载] 发现 %d 个新测试用例，已自动添加到监控\n", newCount)
	}
	_ = modifiedCount
}

// isTestCaseModified 比较两个测试用例的关键属性，判断其是否发生变化（用于热加载时重启任务）。
func isTestCaseModified(old, new psv.TestCase) bool {
	if old.URL != new.URL {
		return true
	}
	if old.Method != new.Method {
		return true
	}
	if old.MonitorInterval != new.MonitorInterval {
		return true
	}
	if old.ResponseThreshold != new.ResponseThreshold {
		return true
	}
	if old.AlertOnFailure != new.AlertOnFailure {
		return true
	}
	if old.AlertOnSlow != new.AlertOnSlow {
		return true
	}
	if old.ExpectedStatus != new.ExpectedStatus {
		return true
	}
	if old.ExpectedBody != new.ExpectedBody {
		return true
	}
	if old.JSON != new.JSON {
		return true
	}
	if old.Body != new.Body {
		return true
	}
	if old.Form != nil || new.Form != nil {
		if len(old.Form) != len(new.Form) {
			return true
		}
		for k, v := range old.Form {
			if new.Form[k] != v {
				return true
			}
		}
	}
	if old.Params != nil || new.Params != nil {
		if len(old.Params) != len(new.Params) {
			return true
		}
		for k, v := range old.Params {
			if new.Params[k] != v {
				return true
			}
		}
	}
	if old.Headers != nil || new.Headers != nil {
		if len(old.Headers) != len(new.Headers) {
			return true
		}
		for k, v := range old.Headers {
			if new.Headers[k] != v {
				return true
			}
		}
	}
	return false
}

// removeDeletedTestCases 移除已从活跃用例列表中删除的监控任务。
func removeDeletedTestCases(activeCases []psv.TestCase) {
	activeIDs := make(map[string]bool)
	for _, tc := range activeCases {
		activeIDs[tc.ID] = true
	}

	tasksMu.Lock()
	var toRemove []string
	for id := range tasks {
		if !activeIDs[id] {
			toRemove = append(toRemove, id)
		}
	}
	tasksMu.Unlock()

	for _, id := range toRemove {
		removeTask(id)
		fmt.Printf("[热加载] 测试用例 %s 已移除\n", id)
	}
}
