package report

import (
	"time"

	"go.uber.org/zap"

	"gwatch/config"
	"gwatch/internal/logger"
)

// YearlyRange 计算年度报告的统计区间（左闭右开）。
//
// 统计规则：从当年 1 月 1 日 00:00 到当月 1 日 00:00，即「当年 1 月 ~ 上月末」的完整月份数据。
// 年份没有走完也照常统计 —— 年度报告不完整不代表没有内容，年初至今的累计数据同样有价值，
// 因此不会因为"年份未结束"就跳过生成或返回空报告。
//
// 只有当当前正好处于 1 月（本年度还没有任何已完成的完整月份）时，
// 才回退到上一个完整年度：去年 1 月 1 日 ~ 今年 1 月 1 日。
func YearlyRange(now time.Time) (startDate, endDate time.Time) {
	loc := now.Location()
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)

	if monthStart.After(yearStart) {
		logger.Info("年度报告统计区间：当年已完成的完整月份",
			zap.String("起始", yearStart.Format("2006-01-02")),
			zap.String("结束(不含)", monthStart.Format("2006-01-02")),
			zap.Int("完整月份数", int(now.Month())-1),
		)
		return yearStart, monthStart
	}

	logger.Info("年度报告统计区间：当年尚无完整月份，回退到上一年度",
		zap.String("起始", yearStart.AddDate(-1, 0, 0).Format("2006-01-02")),
		zap.String("结束(不含)", yearStart.Format("2006-01-02")),
	)
	return yearStart.AddDate(-1, 0, 0), yearStart
}

// GenerateYearlyReportFromStorage 从存储中生成年度报告。
// 统计区间由 YearlyRange 决定（当年 1 月 ~ 上月末，1 月时回退为上一年度）。
func GenerateYearlyReportFromStorage(date time.Time) *Report {
	startDate, endDate := YearlyRange(date)
	return GenerateReportFromStorage(PeriodYearly, startDate, endDate)
}

// GenerateYearlyContent 生成年报告的文本内容，包含每月资源数据和系统状态。
//
// 月度资源区块在采集器启用时始终渲染：即使统计年度内没有任何采集数据（例如系统刚部署、
// 上一年度尚未积累数据），也要保留图表区块并显示"无数据"占位，
// 而不是让整块图表从年报中消失。
func (r *Report) GenerateYearlyContent() string {
	scraperEnabled := config.GlobalConfig.Scraper.Enabled && len(config.GlobalConfig.Scraper.Targets) > 0

	data := struct {
		Base            baseReportData
		HasMonthly      bool
		Monthly         monthlyResourceData
		HasSystemStatus bool
		SystemStatus    *SystemMetricsSnapshot
	}{
		Base:            buildBaseData(r),
		HasMonthly:      scraperEnabled || len(r.MonthlyMetrics) > 0,
		Monthly:         buildMonthlyResourceData(r),
		HasSystemStatus: r.SystemMetrics != nil,
		SystemStatus:    r.SystemMetrics,
	}
	return executeTemplate("yearly", data)
}
