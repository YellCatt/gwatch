package monitor

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/psv"
)

// startTask 启动一个监控任务：创建 MonitorTask 并启动定时调度协程。
// 若任务已存在则跳过。
func startTask(tc psv.TestCase) {
	tasksMu.Lock()
	defer tasksMu.Unlock()

	if _, exists := tasks[tc.ID]; exists {
		logger.Info("监控任务已存在", zap.String("id", tc.ID))
		return
	}

	task := &MonitorTask{
		TestCase: tc,
		Ticker:   time.NewTicker(time.Duration(tc.MonitorInterval) * time.Second),
		StopChan: make(chan struct{}),
		Running:  true,
	}
	tasks[tc.ID] = task

	go scheduleTask(task)

	fmt.Printf("启动监控任务: [%s] %s (周期: %ds)\n", tc.ID, tc.Desc, tc.MonitorInterval)
}

// scheduleTask 调度单个监控任务：立即执行一次，之后按 Ticker 周期重复执行，
// 直到 StopChan 收到停止信号。
func scheduleTask(task *MonitorTask) {
	taskChan <- task.TestCase

	for {
		select {
		case <-task.Ticker.C:
			taskChan <- task.TestCase
		case <-task.StopChan:
			task.Running = false
			return
		}
	}
}

// removeTask 移除指定 ID 的监控任务：停止 Ticker、关闭 StopChan 并从任务表中删除。
func removeTask(id string) {
	tasksMu.Lock()
	task, exists := tasks[id]
	if exists {
		task.Ticker.Stop()
		close(task.StopChan)
		delete(tasks, id)
	}
	tasksMu.Unlock()

	if exists {
		logger.Info("已移除监控任务", zap.String("id", id))
	}
}