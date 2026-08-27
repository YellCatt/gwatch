package email

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
	"gwatch/internal/util"
)

// AlertSource 告警来源类型（API 监控 / 采集器 / 系统监控）。
type AlertSource string

const (
	// SourceAPI 接口监控告警
	SourceAPI AlertSource = "api_monitor"
	// SourceScraper 远程采集器告警
	SourceScraper AlertSource = "scraper"
	// SourceSystem 系统资源监控告警
	SourceSystem AlertSource = "system_monitor"
)

// UnifiedAlert 统一告警结构，各模块产生的告警都会被包装成此结构，
// 由 dispatcher 合并、去重、冷却后统一通过邮件发送。
type UnifiedAlert struct {
	Source            AlertSource // 告警来源
	SourceName        string      // 来源中文名
	TargetName        string      // 目标（接口/指标/资源）名
	MetricName        string      // 内部指标名
	MetricAlias       string      // 友好别名
	Value             float64     // 当前值
	Unit              string      // 单位
	Threshold         float64     // 阈值
	AlertLevel        string      // CRITICAL / WARNING
	Message           string      // 告警描述
	Timestamp         time.Time   // 发生时间
	TopProcesses      []string    // 系统监控场景下的 Top 进程
	TopProcessesLabel string      // Top 进程标签
	StatusCode        int         // HTTP 响应状态码（接口监控场景）
	Assertion         string      // 断言内容或错误详情（接口监控场景）
	StatusCodeOk      bool        // 状态码断言是否通过（接口监控场景）
	AssertionOk       bool        // 响应体断言是否通过（接口监控场景）
}

var (
	// alertChan 告警缓冲通道，用于各模块异步提交告警
	alertChan = make(chan UnifiedAlert, 200)
	// collectedAlerts 聚合周期内已收集的告警列表
	collectedAlerts []UnifiedAlert
	// alertsMu 保护 collectedAlerts 的互斥锁
	alertsMu sync.Mutex
	// lastAlertKeys 记录每个告警键（来源+目标+指标）最近一次发送时间，用于冷却去重
	lastAlertKeys = make(map[string]time.Time)
	// lastAlertMu 保护 lastAlertKeys 的互斥锁
	lastAlertMu sync.Mutex
	// dispatcherRunning 告警调度协程是否已启动
	dispatcherRunning bool
	// dispatcherMu 保护 dispatcherRunning 的互斥锁
	dispatcherMu sync.Mutex
)

// DispatchAlert 提交一条告警到统一调度器；若调度协程未启动则自动启动。
func DispatchAlert(alert UnifiedAlert) {
	logger.Debug("收到告警通知",
		zap.String("target", alert.TargetName),
		zap.String("metric", alert.MetricName),
		zap.String("alert_level", alert.AlertLevel),
		zap.String("message", alert.Message))

	// 首次调用时启动后台调度协程
	dispatcherMu.Lock()
	if !dispatcherRunning {
		dispatcherRunning = true
		dispatcherMu.Unlock()
		go alertDispatcherLoop()
	} else {
		dispatcherMu.Unlock()
	}

	alertChan <- alert
}

// alertDispatcherLoop 告警调度主循环：每 30 秒批量 flush 一次告警并发送邮件。
func alertDispatcherLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case alert, ok := <-alertChan:
			if !ok {
				flushAndSend()
				return
			}
			alertsMu.Lock()
			collectedAlerts = append(collectedAlerts, alert)
			alertsMu.Unlock()

		case <-ticker.C:
			flushAndSend()
		}
	}
}

// flushAndSend 将当前缓存的告警列表取出、按冷却时间过滤并发送邮件。
func flushAndSend() {
	alertsMu.Lock()
	if len(collectedAlerts) == 0 {
		alertsMu.Unlock()
		return
	}
	alerts := make([]UnifiedAlert, len(collectedAlerts))
	copy(alerts, collectedAlerts)
	collectedAlerts = nil
	alertsMu.Unlock()

	filtered := filterByCooldown(alerts)
	if len(filtered) == 0 {
		logger.Debug("所有告警被冷却抑制，跳过发送", zap.Int("original_count", len(alerts)))
		return
	}

	logger.Debug("开始发送告警邮件",
		zap.Int("original_count", len(alerts)),
		zap.Int("filtered_count", len(filtered)))

	if err := sendUnifiedAlertEmail(filtered); err != nil {
		logger.Warn("发送统一告警邮件失败", zap.Error(err))
	} else {
		logger.Debug("告警邮件发送成功", zap.Int("count", len(filtered)))
	}
}

// filterByCooldown 基于冷却时间过滤告警，防止短时间内重复发送同一告警。
// 冷却时间按告警来源区分（API / 采集器 / 系统）。
func filterByCooldown(alerts []UnifiedAlert) []UnifiedAlert {
	lastAlertMu.Lock()
	defer lastAlertMu.Unlock()

	var filtered []UnifiedAlert
	now := timeutil.Now()

	for _, a := range alerts {
		key := fmt.Sprintf("%s:%s:%s", a.Source, a.TargetName, a.MetricName)
		cooldown := getCooldownForSource(a.Source)

		if last, ok := lastAlertKeys[key]; ok && now.Sub(last) < cooldown {
			logger.Debug("告警被冷却抑制",
				zap.String("key", key),
				zap.Duration("since_last", now.Sub(last)),
				zap.Duration("cooldown", cooldown))
			continue
		}
		lastAlertKeys[key] = now
		filtered = append(filtered, a)
	}

	return filtered
}

// getCooldownForSource 根据告警来源返回对应的冷却时间。
func getCooldownForSource(source AlertSource) time.Duration {
	switch source {
	case SourceAPI:
		if Config.APICooldown > 0 {
			return time.Duration(Config.APICooldown) * time.Second
		}
		return 6 * time.Hour
	case SourceSystem:
		if Config.SystemCooldown > 0 {
			return time.Duration(Config.SystemCooldown) * time.Second
		}
		return 2 * time.Hour
	case SourceScraper:
		if Config.ScraperCooldown > 0 {
			return time.Duration(Config.ScraperCooldown) * time.Second
		}
		return 6 * time.Hour
	default:
		return 5 * time.Minute
	}
}

// sendUnifiedAlertEmail 将告警列表组装为邮件正文和标题并调用 SendEmail 发送。
func sendUnifiedAlertEmail(alerts []UnifiedAlert) error {
	if !Config.Enabled {
		logger.Info("邮件功能已禁用，跳过统一告警邮件")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		logger.Info("邮件未配置，跳过统一告警邮件")
		return nil
	}
	if len(alerts) == 0 {
		return nil
	}

	var criticalCount, warningCount int
	for _, a := range alerts {
		if strings.EqualFold(a.AlertLevel, "CRITICAL") {
			criticalCount++
		} else {
			warningCount++
		}
	}

	grouped := groupBySource(alerts)

	var groups []AlertGroupData
	var allRows []AlertRowData
	for _, g := range grouped {
		var groupRows []AlertRowData
		for _, a := range g.alerts {
			row := AlertRowData{
				TargetName:        a.TargetName,
				MetricAlias:       a.MetricAlias,
				Level:             a.AlertLevel,
				Value:             a.Value,
				Threshold:         a.Threshold,
				Unit:              a.Unit,
				Message:           a.Message,
				TopProcesses:      a.TopProcesses,
				TopProcessesLabel: a.TopProcessesLabel,
				StatusCode:        a.StatusCode,
				Assertion:         a.Assertion,
				StatusCodeOk:      a.StatusCodeOk,
				AssertionOk:       a.AssertionOk,
			}
			groupRows = append(groupRows, row)
			allRows = append(allRows, row)
		}
		groups = append(groups, AlertGroupData{
			SourceName: g.sourceName,
			Alerts:     groupRows,
		})
	}

	data := UnifiedAlertEmailData{
		Timestamp:     timeutil.FormatDateTime(timeutil.Now()),
		DeviceName:    util.GetDeviceName(),
		TotalCount:    len(alerts),
		CriticalCount: criticalCount,
		WarningCount:  warningCount,
		Groups:        groups,
	}

	body := RenderUnifiedAlertBody(data)
	subject := BuildUnifiedAlertSubject(allRows, criticalCount, warningCount, util.GetDeviceName())

	logger.Info("发送统一告警邮件", zap.Int("alerts", len(alerts)))
	return SendEmail(subject, body)
}

// alertGroup 告警按来源分组的中间结构。
type alertGroup struct {
	sourceName string
	alerts     []UnifiedAlert
}

// groupBySource 将告警列表按来源分组，并按固定顺序（API/Scraper/System）输出。
func groupBySource(alerts []UnifiedAlert) []alertGroup {
	sourceOrder := []AlertSource{SourceAPI, SourceScraper, SourceSystem}
	sourceNames := map[AlertSource]string{
		SourceAPI:     "接口监控告警",
		SourceScraper: "远程资源采集告警",
		SourceSystem:  "系统资源监控告警",
	}

	groups := make(map[AlertSource]*alertGroup)
	for _, a := range alerts {
		if groups[a.Source] == nil {
			groups[a.Source] = &alertGroup{sourceName: sourceNames[a.Source]}
		}
		groups[a.Source].alerts = append(groups[a.Source].alerts, a)
	}

	var result []alertGroup
	for _, src := range sourceOrder {
		if g, ok := groups[src]; ok {
			result = append(result, *g)
			delete(groups, src)
		}
	}
	// 剩余未在预定义顺序中的来源追加到末尾
	for _, g := range groups {
		result = append(result, *g)
	}

	return result
}

// CloseDispatcher 关闭告警调度器，会先关闭通道以便后台协程 flush 后退出。
func CloseDispatcher() {
	dispatcherMu.Lock()
	defer dispatcherMu.Unlock()

	if alertChan != nil {
		close(alertChan)
	}
	dispatcherRunning = false
}