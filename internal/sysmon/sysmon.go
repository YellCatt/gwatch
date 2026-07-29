package sysmon

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
)

var (
	history     []SystemMetric
	historyMu   sync.RWMutex
	maxHistory  = 600
	stopSysMon  chan struct{}
	running     bool
	runningMu   sync.Mutex
)

func StartSystemMonitor() {
	cfg := config.GlobalConfig.SystemMon
	if !cfg.Enabled {
		logger.Info("System monitor is disabled")
		return
	}

	if err := EnsureStorage(); err != nil {
		logger.Error("Failed to initialize system monitor storage", zap.Error(err))
		return
	}

	runningMu.Lock()
	if running {
		runningMu.Unlock()
		return
	}
	running = true
	runningMu.Unlock()

	stopSysMon = make(chan struct{})

	interval := time.Duration(cfg.Interval) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}

	history = make([]SystemMetric, 0, maxHistory)

	go collectLoop(interval)
	go dailyReportLoop()
	go cleanupLoop()

	printSystemMonitorInfo(interval)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-stopSysMon

	runningMu.Lock()
	running = false
	runningMu.Unlock()

	logger.Info("System monitor stopped")
}

func StopSystemMonitor() {
	runningMu.Lock()
	defer runningMu.Unlock()
	if running && stopSysMon != nil {
		close(stopSysMon)
	}
}

func collectLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("System monitor collection started", zap.Duration("interval", interval))

	for {
		select {
		case <-stopSysMon:
			return
		case <-ticker.C:
			metric, err := CollectMetrics()
			if err != nil {
				logger.Error("Failed to collect system metrics", zap.Error(err))
				continue
			}

			addHistory(metric)

			if err := RecordMetric(metric); err != nil {
				logger.Warn("Failed to record system metric", zap.Error(err))
			}

			alerts := CheckAlerts(metric)
			if len(alerts) > 0 {
				for _, a := range alerts {
					logger.Warn("System threshold exceeded",
						zap.String("metric", a.Metric),
						zap.Float64("value", a.Value),
						zap.Float64("threshold", a.Threshold))
				}

				if err := SendAlertEmail(alerts); err != nil {
					logger.Warn("Failed to send system alert email", zap.Error(err))
				}
			}
		}
	}
}

func dailyReportLoop() {
	now := time.Now()
	nextReport := time.Date(now.Year(), now.Month(), now.Day(), 7, 0, 0, 0, now.Location())
	if now.After(nextReport) {
		nextReport = nextReport.Add(24 * time.Hour)
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-stopSysMon:
			return
		case <-ticker.C:
			now := time.Now()
			if now.After(nextReport) {
				if err := generateAndSendSystemReport(); err != nil {
					logger.Warn("Failed to generate system report", zap.Error(err))
				}
				nextReport = nextReport.Add(24 * time.Hour)
			}
		}
	}
}

func cleanupLoop() {
	interval := time.Duration(config.GlobalConfig.SystemMon.RetentionHours) * time.Hour
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopSysMon:
			return
		case <-ticker.C:
			if err := CleanupOldRecords(); err != nil {
				logger.Warn("Failed to cleanup old system metrics", zap.Error(err))
			}
		}
	}
}

func addHistory(metric SystemMetric) {
	historyMu.Lock()
	defer historyMu.Unlock()

	history = append(history, metric)
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
}

func GetHistory() []SystemMetric {
	historyMu.RLock()
	defer historyMu.RUnlock()
	result := make([]SystemMetric, len(history))
	copy(result, history)
	return result
}

func generateAndSendSystemReport() error {
	metrics, err := LoadRecentMetrics(24)
	if err != nil {
		return err
	}

	if len(metrics) == 0 {
		logger.Info("No system metrics data for report")
		return nil
	}

	latest := metrics[len(metrics)-1]
	alerts := CheckAlerts(latest)

	if config.GlobalConfig.SystemMon.ChartEnabled {
		reportPath, err := SaveSystemReport(metrics, alerts)
		if err != nil {
			logger.Warn("Failed to save system report", zap.Error(err))
		} else {
			logger.Info("System report saved", zap.String("path", reportPath))
		}
	}

	if config.GlobalConfig.SystemMon.EmailEnabled {
		if err := SendSystemStatusEmail(metrics); err != nil {
			logger.Warn("Failed to send system status email", zap.Error(err))
		}
	}

	return nil
}

func printSystemMonitorInfo(interval time.Duration) {
	fmt.Printf("\n╔══════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║           系统资源监控已启动                            ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 采集间隔:   %-43v ║\n", interval)
	fmt.Printf("║ CPU阈值:    %-43s ║\n", fmt.Sprintf("%.0f%%", config.GlobalConfig.SystemMon.CPUThreshold))
	fmt.Printf("║ 内存阈值:   %-43s ║\n", fmt.Sprintf("%.0f%%", config.GlobalConfig.SystemMon.MemoryThreshold))
	fmt.Printf("║ 磁盘阈值:   %-43s ║\n", fmt.Sprintf("%.0f%%", config.GlobalConfig.SystemMon.DiskUsageThreshold))
	fmt.Printf("║ 图表生成:   %-43v ║\n", config.GlobalConfig.SystemMon.ChartEnabled)
	fmt.Printf("║ 邮件告警:   %-43v ║\n", config.GlobalConfig.SystemMon.EmailEnabled)
	fmt.Printf("╚══════════════════════════════════════════════════════════╝\n\n")
}

func PrintCurrentStatus() {
	metric, err := CollectMetrics()
	if err != nil {
		fmt.Printf("采集失败: %v\n", err)
		return
	}

	fmt.Printf("\n╔══ 当前系统状态 ══╗\n")
	fmt.Printf("║ CPU:    %6.2f%%  ║\n", metric.CPUPercent)
	fmt.Printf("║ MEM:    %6.2f%%  ║", metric.MemoryPercent)
	fmt.Printf("║ DISK:   %6.2f%%  ║", metric.DiskPercent)
	fmt.Printf("║ NET↓:   %6.2f KB/s  ║", metric.NetDownKBps)
	fmt.Printf("║ NET↑:   %6.2f KB/s  ║", metric.NetUpKBps)
	fmt.Printf("╚══════════════════╝\n")
}

func GenerateAndSaveReport() (string, error) {
	metrics, err := LoadRecentMetrics(24)
	if err != nil {
		return "", err
	}

	if len(metrics) == 0 {
		return "", fmt.Errorf("no data")
	}

	latest := metrics[len(metrics)-1]
	alerts := CheckAlerts(latest)
	return SaveSystemReport(metrics, alerts)
}