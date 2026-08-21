package email

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"gwatch/internal/util"
)

//go:embed templates/*.tmpl
var alertTemplateFS embed.FS

var alertTmpl *template.Template

func formatValue(v float64, unit string) string {
	if unit == "KB/s" {
		return util.FormatSpeed(v)
	}
	return fmt.Sprintf("%.2f %s", v, unit)
}

func levelDisplay(level string) string {
	if strings.EqualFold(level, "CRITICAL") {
		return "严重"
	}
	return "警告"
}

func init() {
	funcMap := template.FuncMap{
		"formatSpeed":  util.FormatSpeed,
		"formatValue":  formatValue,
		"printf":       fmt.Sprintf,
		"levelDisplay": levelDisplay,
	}
	alertTmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(alertTemplateFS, "templates/*.tmpl"))
}

type UnifiedAlertEmailData struct {
	Timestamp     string
	DeviceName    string
	TotalCount    int
	CriticalCount int
	WarningCount  int
	Groups        []AlertGroupData
}

type AlertGroupData struct {
	SourceName string
	Alerts     []AlertRowData
}

type AlertRowData struct {
	TargetName        string
	MetricAlias       string
	Level             string
	Value             float64
	Threshold         float64
	Unit              string
	Message           string
	TopProcesses      []string
	TopProcessesLabel string
}

func renderAlertTemplate(data UnifiedAlertEmailData) string {
	var buf bytes.Buffer
	if err := alertTmpl.ExecuteTemplate(&buf, "unified_alert", data); err != nil {
		return fmt.Sprintf("模板渲染错误: %v", err)
	}
	return buf.String()
}

func RenderUnifiedAlertBody(data UnifiedAlertEmailData) string {
	return renderAlertTemplate(data)
}

func BuildUnifiedAlertSubject(alerts []AlertRowData, criticalCount, warningCount int) string {
	icon := "⚠️"
	if criticalCount > 0 {
		icon = "🚨"
	}

	var alertNames []string
	seen := make(map[string]bool)
	for _, a := range alerts {
		name := a.MetricAlias
		if name == "" {
			name = a.TargetName
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

	subject := fmt.Sprintf("%s %s | 告警(%d)·严重(%d)·警告(%d)",
		icon, strings.Join(alertNames, ", "), len(alerts), criticalCount, warningCount)

	if len([]rune(subject)) > 40 {
		runes := []rune(subject)
		if len(runes) > 39 {
			subject = string(runes[:38]) + "…"
		}
	}

	return subject
}