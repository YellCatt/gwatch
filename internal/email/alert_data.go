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

// alertTmpl 预编译的告警邮件模板集合，从 templates/*.tmpl 加载。
var alertTmpl *template.Template

// formatValue 根据单位对阈值/数值做格式化：速度类单位走 FormatSpeed，其他保留两位小数。
func formatValue(v float64, unit string) string {
	if unit == "KB/s" {
		return util.FormatSpeed(v)
	}
	return fmt.Sprintf("%.2f %s", v, unit)
}

// levelDisplay 将英文告警级别转换为中文展示文本。
func levelDisplay(level string) string {
	if strings.EqualFold(level, "CRITICAL") {
		return "严重"
	}
	return "警告"
}

// init 在包初始化时加载模板并注册自定义模板函数。
func init() {
	funcMap := template.FuncMap{
		"formatSpeed":  util.FormatSpeed,
		"formatValue":  formatValue,
		"printf":       fmt.Sprintf,
		"levelDisplay": levelDisplay,
	}
	alertTmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(alertTemplateFS, "templates/*.tmpl"))
}

// UnifiedAlertEmailData 统一告警邮件模板的顶层数据结构。
type UnifiedAlertEmailData struct {
	Timestamp     string           // 邮件生成时间
	DeviceName    string           // 当前设备名
	TotalCount    int              // 告警总数
	CriticalCount int              // CRITICAL 级别数量
	WarningCount  int              // WARNING 级别数量
	Groups        []AlertGroupData // 按来源分组的告警列表
}

// AlertGroupData 按告警来源分组的数据。
type AlertGroupData struct {
	SourceName string         // 来源中文名（如"接口监控告警"）
	Alerts     []AlertRowData // 该来源下的告警行
}

// AlertRowData 单条告警行的展示字段。
type AlertRowData struct {
	TargetName        string   // 告警目标名
	MetricAlias       string   // 指标别名
	Level             string   // CRITICAL / WARNING
	Value             float64  // 当前值
	Threshold         float64  // 阈值
	Unit              string   // 单位
	Message           string   // 告警描述信息
	TopProcesses      []string // 系统监控场景下的 Top 进程列表
	TopProcessesLabel string   // Top 进程对应标签
	StatusCode        int      // HTTP 响应状态码
	Assertion         string   // 断言内容或错误详情
}

// renderAlertTemplate 使用模板引擎渲染统一告警邮件正文。
func renderAlertTemplate(data UnifiedAlertEmailData) string {
	var buf bytes.Buffer
	if err := alertTmpl.ExecuteTemplate(&buf, "unified_alert", data); err != nil {
		return fmt.Sprintf("模板渲染错误: %v", err)
	}
	return buf.String()
}

// RenderUnifiedAlertBody 对外暴露的统一告警邮件正文渲染接口。
func RenderUnifiedAlertBody(data UnifiedAlertEmailData) string {
	return renderAlertTemplate(data)
}

// BuildUnifiedAlertSubject 构造统一告警邮件标题。
// 会根据严重级别选择图标，并压缩过长的告警名列表。
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

	// 过长时使用省略号截断
	if len([]rune(subject)) > 40 {
		runes := []rune(subject)
		if len(runes) > 39 {
			subject = string(runes[:38]) + "…"
		}
	}

	return subject
}