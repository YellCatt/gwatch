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

type AlertSource string

const (
	SourceAPI     AlertSource = "api_monitor"
	SourceScraper AlertSource = "scraper"
	SourceSystem  AlertSource = "system_monitor"
)

type UnifiedAlert struct {
	Source      AlertSource
	SourceName  string
	TargetName  string
	MetricName  string
	MetricAlias string
	Value       float64
	Unit        string
	Threshold   float64
	AlertLevel  string
	Message     string
	Timestamp   time.Time
}

var (
	alertChan         = make(chan UnifiedAlert, 200)
	collectedAlerts   []UnifiedAlert
	alertsMu          sync.Mutex
	lastAlertKeys     = make(map[string]time.Time)
	lastAlertMu       sync.Mutex
	dispatcherRunning bool
	dispatcherMu      sync.Mutex
)

func DispatchAlert(alert UnifiedAlert) {
	logger.Debug("收到告警通知",
		zap.String("target", alert.TargetName),
		zap.String("metric", alert.MetricName),
		zap.String("alert_level", alert.AlertLevel),
		zap.String("message", alert.Message))

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
		logger.Warn("Failed to send unified alert email", zap.Error(err))
	} else {
		logger.Debug("告警邮件发送成功", zap.Int("count", len(filtered)))
	}
}

func filterByCooldown(alerts []UnifiedAlert) []UnifiedAlert {
	lastAlertMu.Lock()
	defer lastAlertMu.Unlock()

	var filtered []UnifiedAlert
	now := timeutil.Now()

	for _, a := range alerts {
		key := fmt.Sprintf("%s:%s:%s", a.Source, a.TargetName, a.MetricName)
		cooldown := getCooldownForSource(a.Source)

		if last, ok := lastAlertKeys[key]; ok && now.Sub(last) < cooldown {
			logger.Debug("Alert suppressed by cooldown",
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

func sendUnifiedAlertEmail(alerts []UnifiedAlert) error {
	if !Config.Enabled {
		logger.Info("Email disabled, skipping unified alert email")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		logger.Info("Email not configured, skipping unified alert email")
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
				TargetName:  a.TargetName,
				MetricAlias: a.MetricAlias,
				Level:       a.AlertLevel,
				Value:       a.Value,
				Threshold:   a.Threshold,
				Unit:        a.Unit,
				Message:     a.Message,
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
	subject := BuildUnifiedAlertSubject(allRows, criticalCount, warningCount)

	logger.Info("Sending unified alert email", zap.Int("alerts", len(alerts)))
	return SendEmail(subject, body)
}

type alertGroup struct {
	sourceName string
	alerts     []UnifiedAlert
}

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
	for src, g := range groups {
		result = append(result, *g)
		_ = src
	}

	return result
}

func CloseDispatcher() {
	dispatcherMu.Lock()
	defer dispatcherMu.Unlock()

	if alertChan != nil {
		close(alertChan)
	}
	dispatcherRunning = false
}