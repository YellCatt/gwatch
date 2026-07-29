package monitor

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/psv"
)

func startTask(tc psv.TestCase) {
	tasksMu.Lock()
	defer tasksMu.Unlock()

	if _, exists := tasks[tc.ID]; exists {
		logger.Warn("Task already exists", zap.String("id", tc.ID))
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
		logger.Info("Removed monitor task", zap.String("id", id))
	}
}