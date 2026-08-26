package report

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
	"time"

	"gwatch/config"
	"gwatch/internal/timeutil"
	"gwatch/internal/util"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var tmpl *template.Template

// init 初始化模板系统，注册自定义模板函数并解析嵌入式模板文件。
func init() {
	funcMap := template.FuncMap{
		"formatDuration": formatDuration,
		"formatDateTime": timeutil.FormatDateTime,
		"formatBytes":    util.FormatBytes,
		"formatSpeed":    util.FormatSpeed,
		"boolEnabled":    boolToEnabled,
		"join":           strings.Join,
		"deviceName":     util.GetDeviceName,
		"periodName":     func(p ReportPeriod) string { return PeriodNames[p] },
		"now":            timeutil.Now,
	}
	tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.tmpl"))
}

// executeTemplate 使用指定名称渲染模板，返回渲染后的字符串。
func executeTemplate(name string, data any) string {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return fmt.Sprintf("模板渲染错误 [%s]: %v", name, err)
	}
	return buf.String()
}

type baseReportData struct {
	Period               string
	StartDate            string
	EndDate              string
	GeneratedAt          time.Time
	TotalTasks           int
	SuccessTasks         int
	FailedTasks          int
	SuccessRate          float64
	AvgDuration          string
	InterfaceStats       []interfaceStatRow
	AggregatedErrors     []aggregatedErrorRow
	SystemAlerts         []systemAlertRow
	ScraperAlerts        []scraperAlertRow
	CriticalAlertCount   int
	WarningAlertCount    int
	TotalAlertCount      int
	SystemCriticalCount  int
	SystemWarningCount   int
	SystemAlertTotal     int
	ScraperCriticalCount int
	ScraperWarningCount  int
	ScraperAlertTotal    int
}

type interfaceStatRow struct {
	TaskID       string
	TotalCount   int
	SuccessCount int
	FailedCount  int
	AlertCount   int
	AvgDuration  string
	MaxDuration  string
	HasFailure   bool
}

type aggregatedErrorRow struct {
	Icon            string
	Level           string
	TaskID          string
	TaskDesc        string
	Method          string
	URL             string
	ExpectedStatus  int
	AlertCount      int
	FirstOccurrence string
	LastOccurrence  string
	ErrorMsg        string
}

type systemAlertRow struct {
	Icon            string
	Level           string
	Metric          string
	MetricAlias     string
	Value           string
	Threshold       string
	Unit            string
	AlertCount      int64
	FirstOccurrence string
	LastOccurrence  string
	Message         string
}

type scraperAlertRow struct {
	Icon            string
	Level           string
	TargetName      string
	TargetURL       string
	MetricName      string
	MetricAlias     string
	Value           string
	Threshold       string
	Unit            string
	AlertCount      int64
	FirstOccurrence string
	LastOccurrence  string
	Message         string
}

type startupReportData struct {
	Version            string
	GeneratedAt        string
	DeviceName         string
	Now                string
	DataDir            string
	ReportDir          string
	CaseDir            string
	MonitorEnabled     string
	DefaultInterval    int
	MaxWorkers         int
	AlertOnFailure     string
	AlertOnSlow        string
	AlertInterval      int
	DailyReport        string
	WeeklyReport       string
	MonthlyReport      string
	YearlyReport       string
	DailyAllReports    string
	ReportTime         string
	HasDailyReport     bool
	EmailEnabled       string
	HasEmail           bool
	EmailFrom          string
	EmailTo            string
	SMTPServer         string
	SMTPPort           int
	ScraperEnabled     string
	HasScraper         bool
	ScraperTargetCount int
	ScraperTargets     []scraperTargetRow
	SystemMonEnabled   string
	HasSystemMon       bool
	SystemMonInterval  int
	CPUThreshold       string
	MemoryThreshold    string
	DiskThreshold      string
	TaskCount          int
	HasTasks           bool
	TaskList           []startupTaskRow
	ActualMaxWorkers   int
}

type startupTaskRow struct {
	ID       string
	Desc     string
	Method   string
	URL      string
	Interval int
}

type scraperTargetRow struct {
	Name string
	URL  string
}

type hourlyResourceData struct {
	StartDate string
	EndDate   string
	Metrics   []hourlyMetricRow
	Charts    []string
}

type hourlyMetricRow struct {
	TargetName  string
	MetricAlias string
	Unit        string
	Values      []string
}

type dailyResourceData struct {
	StartDate string
	EndDate   string
	Title     string
	Metrics   []dailyMetricRow
	Charts    []string
}

type dailyMetricRow struct {
	TargetName  string
	MetricAlias string
	Unit        string
	Labels      []string
	Values      []string
}

type monthlyResourceData struct {
	StartDate string
	EndDate   string
	Metrics   []monthlyMetricRow
	Charts    []string
}

type monthlyMetricRow struct {
	TargetName  string
	MetricAlias string
	Unit        string
	Labels      []string
	Values      []string
}

// buildBaseData 从 Report 对象构建基础报告数据结构，包含接口统计、告警汇总等。
// 计算成功率、聚合告警级别分布（严重/警告/总次数），将内部数据结构转换为模板友好的扁平结构。
// 该数据供 base/daily/weekly/monthly/yearly 等所有周期报告模板共用。
func buildBaseData(r *Report) baseReportData {
	successRate := 0.0
	if r.TotalTasks > 0 {
		successRate = float64(r.SuccessTasks) / float64(r.TotalTasks) * 100
	}

	stats := make([]interfaceStatRow, 0, len(r.InterfaceStats))
	for _, s := range r.InterfaceStats {
		stats = append(stats, interfaceStatRow{
			TaskID:       s.TaskID,
			TotalCount:   s.TotalCount,
			SuccessCount: s.SuccessCount,
			FailedCount:  s.FailedCount,
			AlertCount:   s.AlertCount,
			AvgDuration:  formatDuration(s.AvgDurationMS),
			MaxDuration:  formatDuration(s.MaxDurationMS),
			HasFailure:   s.FailedCount > 0 || s.AlertCount > 0,
		})
	}

	criticalCount := 0
	warningCount := 0
	totalAlertCount := 0
	errs := make([]aggregatedErrorRow, 0, len(r.AggregatedErrors))
	for _, e := range r.AggregatedErrors {
		icon, label := alertLevelDisplay(e.AlertLevel)
		level := strings.ToUpper(strings.TrimSpace(e.AlertLevel))
		if level == "CRITICAL" {
			criticalCount += e.AlertCount
		} else if level == "WARNING" {
			warningCount += e.AlertCount
		}
		totalAlertCount += e.AlertCount
		errs = append(errs, aggregatedErrorRow{
			Icon:            icon,
			Level:           label,
			TaskID:          e.TaskID,
			TaskDesc:        e.TaskDesc,
			Method:          e.Method,
			URL:             e.URL,
			ExpectedStatus:  e.ExpectedStatus,
			AlertCount:      e.AlertCount,
			FirstOccurrence: timeutil.FormatDateTime(e.FirstOccurrence),
			LastOccurrence:  timeutil.FormatDateTime(e.LastOccurrence),
			ErrorMsg:        e.ErrorMsg,
		})
	}

	sysCritical := 0
	sysWarning := 0
	sysTotal := 0
	sysAlerts := make([]systemAlertRow, 0, len(r.SystemAlerts))
	for _, a := range r.SystemAlerts {
		icon, label := alertLevelDisplay(a.AlertLevel)
		level := strings.ToUpper(strings.TrimSpace(a.AlertLevel))
		if level == "CRITICAL" {
			sysCritical += int(a.AlertCount)
		} else if level == "WARNING" {
			sysWarning += int(a.AlertCount)
		}
		sysTotal += int(a.AlertCount)
		sysAlerts = append(sysAlerts, systemAlertRow{
			Icon:            icon,
			Level:           label,
			Metric:          a.Metric,
			MetricAlias:     a.MetricAlias,
			Value:           fmt.Sprintf("%.1f", a.Value),
			Threshold:       fmt.Sprintf("%.1f", a.Threshold),
			Unit:            a.Unit,
			AlertCount:      a.AlertCount,
			FirstOccurrence: a.FirstOccurrence,
			LastOccurrence:  a.LastOccurrence,
			Message:         a.Message,
		})
	}

	scraperCritical := 0
	scraperWarning := 0
	scraperTotal := 0
	scraperAlerts := make([]scraperAlertRow, 0, len(r.ScraperAlerts))
	for _, a := range r.ScraperAlerts {
		icon, label := alertLevelDisplay(a.AlertLevel)
		level := strings.ToUpper(strings.TrimSpace(a.AlertLevel))
		if level == "CRITICAL" {
			scraperCritical += int(a.AlertCount)
		} else if level == "WARNING" {
			scraperWarning += int(a.AlertCount)
		}
		scraperTotal += int(a.AlertCount)
		scraperAlerts = append(scraperAlerts, scraperAlertRow{
			Icon:            icon,
			Level:           label,
			TargetName:      a.TargetName,
			TargetURL:       a.TargetURL,
			MetricName:      a.MetricName,
			MetricAlias:     a.MetricAlias,
			Value:           fmt.Sprintf("%.2f", a.Value),
			Threshold:       fmt.Sprintf("%.2f", a.Threshold),
			Unit:            a.Unit,
			AlertCount:      a.AlertCount,
			FirstOccurrence: a.FirstOccurrence,
			LastOccurrence:  a.LastOccurrence,
			Message:         a.Message,
		})
	}

	return baseReportData{
		Period:               PeriodNames[r.Period],
		StartDate:            r.StartDate,
		EndDate:              r.EndDate,
		GeneratedAt:          r.GeneratedAt,
		TotalTasks:           r.TotalTasks,
		SuccessTasks:         r.SuccessTasks,
		FailedTasks:          r.FailedTasks,
		SuccessRate:          successRate,
		AvgDuration:          formatAvgDuration(r.InterfaceStats),
		InterfaceStats:       stats,
		AggregatedErrors:     errs,
		SystemAlerts:         sysAlerts,
		ScraperAlerts:        scraperAlerts,
		CriticalAlertCount:   criticalCount,
		WarningAlertCount:    warningCount,
		TotalAlertCount:      totalAlertCount,
		SystemCriticalCount:  sysCritical,
		SystemWarningCount:   sysWarning,
		SystemAlertTotal:     sysTotal,
		ScraperCriticalCount: scraperCritical,
		ScraperWarningCount:  scraperWarning,
		ScraperAlertTotal:    scraperTotal,
	}
}

// buildStartupData 从 Report 和 StartupInfo 构建启动报告数据结构，包含配置信息和任务列表。
// 汇总应用版本、设备信息、各模块开关状态、阈值配置以及任务清单，供启动报告模板渲染。
// 当 StartupInfo 不为空时，附加任务列表及实际并发参数。
func buildStartupData(r *Report, info *StartupInfo) startupReportData {
	cfg := config.GlobalConfig
	data := startupReportData{
		Version:            config.Version,
		GeneratedAt:        timeutil.FormatDateTime(r.GeneratedAt),
		DeviceName:         util.GetDeviceName(),
		Now:                timeutil.FormatDateTime(timeutil.Now()),
		DataDir:            cfg.App.DataDir,
		ReportDir:          cfg.App.ReportDir,
		CaseDir:            cfg.App.CaseDir,
		MonitorEnabled:     boolToEnabled(cfg.Monitor.Enabled),
		DefaultInterval:    cfg.Monitor.DefaultInterval,
		MaxWorkers:         cfg.Monitor.MaxWorkers,
		AlertOnFailure:     boolToEnabled(cfg.Monitor.AlertOnFailure),
		AlertOnSlow:        boolToEnabled(cfg.Monitor.AlertOnSlow),
		AlertInterval:      cfg.Monitor.AlertInterval,
		DailyReport:        boolToEnabled(cfg.Monitor.DailyReport),
		WeeklyReport:       boolToEnabled(cfg.Monitor.WeeklyReport),
		MonthlyReport:      boolToEnabled(cfg.Monitor.MonthlyReport),
		YearlyReport:       boolToEnabled(cfg.Monitor.YearlyReport),
		DailyAllReports:    boolToEnabled(cfg.Monitor.DailyAllReports),
		ReportTime:         cfg.Monitor.ReportTime,
		HasDailyReport:     cfg.Monitor.DailyReport,
		EmailEnabled:       boolToEnabled(cfg.Email.Enabled),
		HasEmail:           cfg.Email.Enabled,
		EmailFrom:          cfg.Email.From,
		EmailTo:            strings.Join(cfg.Email.To, ", "),
		SMTPServer:         cfg.Email.SMTPServer,
		SMTPPort:           cfg.Email.SMTPPort,
		ScraperEnabled:     boolToEnabled(cfg.Scraper.Enabled),
		HasScraper:         cfg.Scraper.Enabled,
		ScraperTargetCount: len(cfg.Scraper.Targets),
		ScraperTargets:     buildScraperTargets(cfg.Scraper.Targets),
		SystemMonEnabled:   boolToEnabled(cfg.SystemMon.Enabled),
		HasSystemMon:       cfg.SystemMon.Enabled,
		SystemMonInterval:  cfg.SystemMon.Interval,
		CPUThreshold:       fmt.Sprintf("%.0f", cfg.SystemMon.CPUThreshold),
		MemoryThreshold:    fmt.Sprintf("%.0f", cfg.SystemMon.MemoryThreshold),
		DiskThreshold:      fmt.Sprintf("%.0f", cfg.SystemMon.DiskUsageThreshold),
	}

	if info != nil && len(info.Tasks) > 0 {
		data.TaskCount = len(info.Tasks)
		data.HasTasks = true
		data.ActualMaxWorkers = info.MaxWorkers
		data.TaskList = make([]startupTaskRow, 0, len(info.Tasks))
		for _, t := range info.Tasks {
			data.TaskList = append(data.TaskList, startupTaskRow{
				ID:       t.ID,
				Desc:     t.Desc,
				Method:   t.Method,
				URL:      t.URL,
				Interval: t.Interval,
			})
		}
	}

	return data
}

// buildScraperTargets 将采集器目标配置列表转换为模板行数据。
// 从配置的 ScraperTargetConfig 切片提取名称和 URL，生成模板渲染所需的扁平结构。
func buildScraperTargets(targets []config.ScraperTargetConfig) []scraperTargetRow {
	rows := make([]scraperTargetRow, 0, len(targets))
	for _, t := range targets {
		rows = append(rows, scraperTargetRow{Name: t.Name, URL: t.URL})
	}
	return rows
}

// buildHourlyResourceData 从 Report 构建每小时资源指标的模板数据。
// 将小时级指标转换为模板友好的行数据，速度类单位使用格式化显示，缺失值显示为 "-"。
// 同时构建对应的 ASCII 图表列表。
func buildHourlyResourceData(r *Report) hourlyResourceData {
	metrics := make([]hourlyMetricRow, 0, len(r.HourlyMetrics))
	for _, m := range r.HourlyMetrics {
		isSpeed := util.IsSpeedUnit(m.Unit)
		values := make([]string, 24)
		for i, d := range m.HourlyData {
			if d.AvgValue >= 0 {
				if isSpeed {
					values[i] = fmt.Sprintf("%11s", util.FormatSpeed(d.AvgValue))
				} else {
					values[i] = fmt.Sprintf("%7.1f", d.AvgValue)
				}
			} else {
				values[i] = fmt.Sprintf("%7s", "-")
			}
		}
		metrics = append(metrics, hourlyMetricRow{
			TargetName:  m.TargetName,
			MetricAlias: m.MetricAlias,
			Unit:        m.Unit,
			Values:      values,
		})
	}
	return hourlyResourceData{
		StartDate: r.StartDate,
		EndDate:   r.EndDate,
		Metrics:   metrics,
		Charts:    buildHourlyChartData(r),
	}
}

// buildDailyResourceData 从 Report 构建每日资源指标的模板数据。
// 将日级指标转换为模板友好的行数据，速度类单位使用格式化显示，缺失值显示为 "-"。
// title 参数用于自定义资源指标区块的标题（如 "每日资源监控"）。
// 同时构建对应的 ASCII 图表列表。
func buildDailyResourceData(r *Report, title string) dailyResourceData {
	metrics := make([]dailyMetricRow, 0, len(r.DailyMetrics))
	for _, m := range r.DailyMetrics {
		isSpeed := util.IsSpeedUnit(m.Unit)
		labels := make([]string, len(m.DailyData))
		values := make([]string, len(m.DailyData))
		for i, d := range m.DailyData {
			labels[i] = d.DayLabel
			if d.AvgValue >= 0 {
				if isSpeed {
					values[i] = fmt.Sprintf("%-14s", util.FormatSpeed(d.AvgValue))
				} else {
					values[i] = fmt.Sprintf("%-10.1f", d.AvgValue)
				}
			} else {
				values[i] = fmt.Sprintf("%-10s", "-")
			}
		}
		metrics = append(metrics, dailyMetricRow{
			TargetName:  m.TargetName,
			MetricAlias: m.MetricAlias,
			Unit:        m.Unit,
			Labels:      labels,
			Values:      values,
		})
	}
	return dailyResourceData{
		StartDate: r.StartDate,
		EndDate:   r.EndDate,
		Title:     title,
		Metrics:   metrics,
		Charts:    buildDailyChartData(r),
	}
}

// buildMonthlyResourceData 从 Report 构建每月资源指标的模板数据。
// 将月级指标转换为模板友好的行数据，速度类单位使用格式化显示，缺失值显示为 "-"。
// 同时构建对应的 ASCII 图表列表。
func buildMonthlyResourceData(r *Report) monthlyResourceData {
	metrics := make([]monthlyMetricRow, 0, len(r.MonthlyMetrics))
	for _, m := range r.MonthlyMetrics {
		isSpeed := util.IsSpeedUnit(m.Unit)
		labels := make([]string, len(m.MonthlyData))
		values := make([]string, len(m.MonthlyData))
		for i, d := range m.MonthlyData {
			labels[i] = d.MonthLabel
			if d.AvgValue >= 0 {
				if isSpeed {
					values[i] = fmt.Sprintf("%-12s", util.FormatSpeed(d.AvgValue))
				} else {
					values[i] = fmt.Sprintf("%-8.1f", d.AvgValue)
				}
			} else {
				values[i] = fmt.Sprintf("%-8s", "-")
			}
		}
		metrics = append(metrics, monthlyMetricRow{
			TargetName:  m.TargetName,
			MetricAlias: m.MetricAlias,
			Unit:        m.Unit,
			Labels:      labels,
			Values:      values,
		})
	}
	return monthlyResourceData{
		StartDate: r.StartDate,
		EndDate:   r.EndDate,
		Metrics:   metrics,
		Charts:    buildMonthlyChartData(r),
	}
}
