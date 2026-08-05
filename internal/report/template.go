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
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var tmpl *template.Template

func init() {
	funcMap := template.FuncMap{
		"formatDuration": formatDuration,
		"formatDateTime": timeutil.FormatDateTime,
		"boolEnabled":    boolToEnabled,
		"join":           strings.Join,
		"deviceName":     getDeviceName,
		"periodName":     func(p ReportPeriod) string { return PeriodNames[p] },
		"now":            timeutil.Now,
	}
	tmpl = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.tmpl"))
}

func executeTemplate(name string, data any) string {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return fmt.Sprintf("模板渲染错误 [%s]: %v", name, err)
	}
	return buf.String()
}

type baseReportData struct {
	Period             string
	StartDate          string
	EndDate            string
	GeneratedAt        time.Time
	TotalTasks         int
	SuccessTasks       int
	FailedTasks        int
	SuccessRate        float64
	AvgDuration        string
	InterfaceStats     []interfaceStatRow
	AggregatedErrors   []aggregatedErrorRow
	CriticalAlertCount int
	WarningAlertCount  int
	TotalAlertCount    int
}

type interfaceStatRow struct {
	TaskID       string
	TotalCount   int
	SuccessCount int
	FailedCount  int
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

type startupReportData struct {
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
}

type monthlyMetricRow struct {
	TargetName  string
	MetricAlias string
	Unit        string
	Labels      []string
	Values      []string
}

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
			AvgDuration:  formatDuration(s.AvgDurationMS),
			MaxDuration:  formatDuration(s.MaxDurationMS),
			HasFailure:   s.FailedCount > 0,
		})
	}

	criticalCount := 0
	warningCount := 0
	errs := make([]aggregatedErrorRow, 0, len(r.AggregatedErrors))
	for _, e := range r.AggregatedErrors {
		icon, label := alertLevelDisplay(e.AlertLevel)
		level := strings.ToUpper(strings.TrimSpace(e.AlertLevel))
		if level == "CRITICAL" {
			criticalCount++
		} else if level == "WARNING" {
			warningCount++
		}
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

	return baseReportData{
		Period:             PeriodNames[r.Period],
		StartDate:          r.StartDate,
		EndDate:            r.EndDate,
		GeneratedAt:        r.GeneratedAt,
		TotalTasks:         r.TotalTasks,
		SuccessTasks:       r.SuccessTasks,
		FailedTasks:        r.FailedTasks,
		SuccessRate:        successRate,
		AvgDuration:        formatAvgDuration(r.InterfaceStats),
		InterfaceStats:     stats,
		AggregatedErrors:   errs,
		CriticalAlertCount: criticalCount,
		WarningAlertCount:  warningCount,
		TotalAlertCount:    len(r.AggregatedErrors),
	}
}

func buildStartupData(r *Report, info *StartupInfo) startupReportData {
	cfg := config.GlobalConfig
	data := startupReportData{
		GeneratedAt:        timeutil.FormatDateTime(r.GeneratedAt),
		DeviceName:         getDeviceName(),
		Now:                timeutil.FormatDateTime(time.Now()),
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

func buildScraperTargets(targets []config.ScraperTargetConfig) []scraperTargetRow {
	rows := make([]scraperTargetRow, 0, len(targets))
	for _, t := range targets {
		rows = append(rows, scraperTargetRow{Name: t.Name, URL: t.URL})
	}
	return rows
}

func buildHourlyResourceData(r *Report) hourlyResourceData {
	metrics := make([]hourlyMetricRow, 0, len(r.HourlyMetrics))
	for _, m := range r.HourlyMetrics {
		values := make([]string, 24)
		for i, d := range m.HourlyData {
			if d.AvgValue >= 0 {
				values[i] = fmt.Sprintf("%7.1f", d.AvgValue)
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
	}
}

func buildDailyResourceData(r *Report, title string) dailyResourceData {
	metrics := make([]dailyMetricRow, 0, len(r.DailyMetrics))
	for _, m := range r.DailyMetrics {
		labels := make([]string, len(m.DailyData))
		values := make([]string, len(m.DailyData))
		for i, d := range m.DailyData {
			labels[i] = d.DayLabel
			if d.AvgValue >= 0 {
				values[i] = fmt.Sprintf("%-10.1f", d.AvgValue)
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
	}
}

func buildMonthlyResourceData(r *Report) monthlyResourceData {
	metrics := make([]monthlyMetricRow, 0, len(r.MonthlyMetrics))
	for _, m := range r.MonthlyMetrics {
		labels := make([]string, len(m.MonthlyData))
		values := make([]string, len(m.MonthlyData))
		for i, d := range m.MonthlyData {
			labels[i] = d.MonthLabel
			if d.AvgValue >= 0 {
				values[i] = fmt.Sprintf("%-8.1f", d.AvgValue)
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
	}
}
