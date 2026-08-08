package email

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
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
		return
	}

	if err := sendUnifiedAlertEmail(filtered); err != nil {
		logger.Error("Failed to send unified alert email", zap.Error(err))
	}
}

func filterByCooldown(alerts []UnifiedAlert) []UnifiedAlert {
	lastAlertMu.Lock()
	defer lastAlertMu.Unlock()

	var filtered []UnifiedAlert
	now := time.Now()

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
		return 6 * time.Hour
	case SourceSystem:
		return 2 * time.Hour
	case SourceScraper:
		return 2 * time.Hour
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

	subject := buildAlertSubject(alerts)

	var body strings.Builder
	body.WriteString("╔══════════════════════════════════════════════════════════╗\n")
	body.WriteString("║              gwatch 统一告警通知                         ║\n")
	body.WriteString("╠══════════════════════════════════════════════════════════╣\n")
	body.WriteString(fmt.Sprintf("║ 告警时间: %-44s ║\n", timeutil.FormatDateTime(timeutil.Now())))
	body.WriteString(fmt.Sprintf("║ 监控设备: %-44s ║\n", getDeviceName()))
	body.WriteString(fmt.Sprintf("║ 告警数量: %d (严重:%d 警告:%d)%-25s ║\n", len(alerts), criticalCount, warningCount, ""))
	body.WriteString("╚══════════════════════════════════════════════════════════╝\n\n")

	grouped := groupBySource(alerts)
	for _, group := range grouped {
		body.WriteString(fmt.Sprintf("━━━ %s ━━━\n\n", group.sourceName))
		for _, a := range group.alerts {
			icon := "⚠️"
			if strings.EqualFold(a.AlertLevel, "CRITICAL") {
				icon = "🚨"
			}
			body.WriteString(fmt.Sprintf("%s [%s] %s\n", icon, a.AlertLevel, a.MetricAlias))
			body.WriteString(fmt.Sprintf("   目标:     %s\n", a.TargetName))
			if a.Value != 0 {
				if a.Unit == "KB/s" {
					body.WriteString(fmt.Sprintf("   当前值:   %s\n", formatSpeedValue(a.Value)))
				} else {
					body.WriteString(fmt.Sprintf("   当前值:   %.2f %s\n", a.Value, a.Unit))
				}
			}
			if a.Threshold > 0 {
				if a.Unit == "KB/s" {
					body.WriteString(fmt.Sprintf("   阈值:     %s\n", formatSpeedValue(a.Threshold)))
				} else {
					body.WriteString(fmt.Sprintf("   阈值:     %.2f %s\n", a.Threshold, a.Unit))
				}
			}
			if a.Message != "" {
				body.WriteString(fmt.Sprintf("   消息:     %s\n", a.Message))
			}
			body.WriteString("\n")
		}
	}

	body.WriteString("────── 来自 gwatch 统一监控系统 ──────\n")

	logger.Info("Sending unified alert email", zap.Int("alerts", len(alerts)))
	return SendEmail(subject, body.String())
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

func buildAlertSubject(alerts []UnifiedAlert) string {
	sourceCounts := make(map[AlertSource]int)
	for _, a := range alerts {
		sourceCounts[a.Source]++
	}

	var alertNames []string
	seen := make(map[string]bool)
	for _, a := range alerts {
		name := a.MetricAlias
		if name == "" {
			name = a.MetricName
		}
		if !seen[name] {
			seen[name] = true
			alertNames = append(alertNames, fmt.Sprintf("%s告警", name))
		}
		if len(alertNames) >= 3 {
			break
		}
	}

	if len(alerts) > 3 {
		alertNames = append(alertNames, fmt.Sprintf("等%d项", len(alerts)))
	}

	var sourceParts []string
	sourceOrder := []AlertSource{SourceAPI, SourceScraper, SourceSystem}
	sourceNames := map[AlertSource]string{
		SourceAPI:     "接口",
		SourceScraper: "采集",
		SourceSystem:  "系统",
	}
	for _, src := range sourceOrder {
		if cnt, ok := sourceCounts[src]; ok {
			sourceParts = append(sourceParts, fmt.Sprintf("%s(%d)", sourceNames[src], cnt))
		}
	}

	alertsText := strings.Join(alertNames, ", ")
	sources := strings.Join(sourceParts, "·")

	subject := fmt.Sprintf("%s | %s", alertsText, sources)

	if len([]rune(subject)) > 40 {
		runes := []rune(subject)
		if len(runes) > 39 {
			subject = string(runes[:38]) + "…"
		}
	}

	return subject
}

func formatSpeedValue(kbps float64) string {
	if kbps >= 1024 {
		return fmt.Sprintf("%.2f MB/s", kbps/1024)
	}
	return fmt.Sprintf("%.2f KB/s", kbps)
}

func CloseDispatcher() {
	dispatcherMu.Lock()
	defer dispatcherMu.Unlock()

	if alertChan != nil {
		close(alertChan)
	}
	dispatcherRunning = false
}
