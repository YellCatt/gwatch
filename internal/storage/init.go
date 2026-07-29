package storage

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
)

var (
	dataDir string
	mu      sync.RWMutex
	once    sync.Once
)

var (
	executionHeader = []string{
		"test_case_id",
		"test_case_desc",
		"file_name",
		"url",
		"duration_ms",
		"success",
		"executed_at",
	}

	averageHeader = []string{
		"test_case_id",
		"test_case_desc",
		"file_name",
		"url",
		"average_duration_ms",
		"execution_count",
		"last_updated",
	}

	monitorHeader = []string{
		"test_case_id",
		"test_case_desc",
		"url",
		"method",
		"expected_status",
		"actual_status",
		"expected_body",
		"actual_body",
		"error_msg",
		"duration_ms",
		"success",
		"timestamp",
	}

	monitorSummaryHeader = []string{
		"date",
		"test_case_id",
		"test_case_desc",
		"url",
		"method",
		"total_count",
		"success_count",
		"failed_count",
		"total_duration_ms",
		"min_duration_ms",
		"max_duration_ms",
		"last_success_time",
		"last_failure_time",
	}

	alertSummaryHeader = []string{
		"date",
		"test_case_id",
		"test_case_desc",
		"url",
		"method",
		"expected_status",
		"alert_level",
		"alert_count",
		"first_occurrence",
		"last_occurrence",
		"error_msg",
	}

	scraperMetricHeader = []string{
		"target_name",
		"target_url",
		"metric_name",
		"metric_alias",
		"value",
		"unit",
		"success",
		"timestamp",
	}

	indexHeader = []string{
		"file_name",
		"description",
		"write_mode",
		"columns",
		"updated_at",
	}
)

func alertLevelRank(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case AlertLevelCritical:
		return 2
	case AlertLevelWarning:
		return 1
	default:
		return 0
	}
}

func parseAlertLevel(level string) string {
	level = strings.ToUpper(strings.TrimSpace(level))
	if level == "" {
		return AlertLevelCritical
	}
	return level
}

func InitDB(dir string) error {
	var initErr error
	once.Do(func() {
		initErr = initCSVInternal(dir)
	})
	return initErr
}

func initCSVInternal(dir string) error {
	logger.Info("========== 开始初始化 CSV 存储 ==========")
	logger.Info("数据目录参数值", zap.String("dataDir", dir))

	if dir == "" {
		logger.Info("数据目录为空，使用默认值 ./sql")
		dir = "./sql"
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Error("创建数据目录失败", zap.String("dataDir", dir), zap.Error(err))
		return err
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		logger.Error("获取数据目录绝对路径失败", zap.String("dir", dir), zap.Error(err))
		return err
	}
	dataDir = absDir
	logger.Info("数据目录创建成功", zap.String("dataDir", dataDir))

	if err := ensureCSV(executionCSVPath(), executionHeader); err != nil {
		logger.Error("初始化执行记录 CSV 失败", zap.Error(err))
		return err
	}
	if err := ensureCSV(averageCSVPath(), averageHeader); err != nil {
		logger.Error("初始化平均时间 CSV 失败", zap.Error(err))
		return err
	}
	if err := ensureCSV(monitorCSVPath(), monitorHeader); err != nil {
		logger.Error("初始化监控记录 CSV 失败", zap.Error(err))
		return err
	}
	if err := ensureCSV(monitorSummaryCSVPath(), monitorSummaryHeader); err != nil {
		logger.Error("初始化监控汇总 CSV 失败", zap.Error(err))
		return err
	}
	if err := ensureCSV(alertSummaryCSVPath(), alertSummaryHeader); err != nil {
		logger.Error("初始化告警汇总 CSV 失败", zap.Error(err))
		return err
	}
	if err := ensureCSV(scraperMetricCSVPath(), scraperMetricHeader); err != nil {
		logger.Error("初始化采集指标 CSV 失败", zap.Error(err))
		return err
	}

	if err := writeIndexCSV(); err != nil {
		logger.Error("生成 CSV 数据表总表失败", zap.Error(err))
		return err
	}

	logger.Info("CSV 存储初始化成功",
		zap.String("executionCSV", executionCSVPath()),
		zap.String("averageCSV", averageCSVPath()),
		zap.String("monitorCSV", monitorCSVPath()),
		zap.String("monitorSummaryCSV", monitorSummaryCSVPath()),
		zap.String("scraperMetricCSV", scraperMetricCSVPath()),
		zap.String("indexCSV", indexCSVPath()))
	return nil
}

type csvFileMeta struct {
	fileName    string
	description string
	writeMode   string
	header      []string
}

func csvFileMetas() []csvFileMeta {
	return []csvFileMeta{
		{
			fileName:    filepath.Base(executionCSVPath()),
			description: "测试执行时间明细：每次测试/监控执行的耗时记录，用于计算接口平均执行时间",
			writeMode:   "追加",
			header:      executionHeader,
		},
		{
			fileName:    filepath.Base(averageCSVPath()),
			description: "接口平均执行时间：按 任务ID+文件+URL 聚合的成功执行平均耗时，用于预估测试执行时间",
			writeMode:   "覆盖重写",
			header:      averageHeader,
		},
		{
			fileName:    filepath.Base(monitorCSVPath()),
			description: "接口监控结果明细：每次监控执行的完整结果（期望/实际状态码、响应体、错误信息、耗时等）",
			writeMode:   "追加",
			header:      monitorHeader,
		},
		{
			fileName:    filepath.Base(monitorSummaryCSVPath()),
			description: "接口监控每日汇总：按 日期+任务ID 聚合的执行总数/成功数/失败数/耗时极值/最后成功失败时间，是运维报告执行统计的数据来源",
			writeMode:   "覆盖重写",
			header:      monitorSummaryHeader,
		},
		{
			fileName:    filepath.Base(alertSummaryCSVPath()),
			description: "接口告警每日汇总：按 日期+任务ID 聚合的告警级别(CRITICAL严重/WARNING警告)/告警次数/首末次告警时间，是运维报告告警汇总的数据来源",
			writeMode:   "覆盖重写",
			header:      alertSummaryHeader,
		},
		{
			fileName:    filepath.Base(scraperMetricCSVPath()),
			description: "系统资源采集指标明细：通用采集器每次采集的指标值（如 CPU/内存/负载），是运维报告资源监控图表的数据来源",
			writeMode:   "追加",
			header:      scraperMetricHeader,
		},
		{
			fileName:    filepath.Base(indexCSVPath()),
			description: "CSV 数据表总表（本文件）：列出所有数据表的文件名、作用、写入方式与字段说明，程序启动时自动生成",
			writeMode:   "覆盖重写",
			header:      indexHeader,
		},
	}
}

func writeIndexCSV() error {
	now := time.Now().Format("2006-01-02 15:04:05")

	var records [][]string
	for _, m := range csvFileMetas() {
		records = append(records, []string{
			m.fileName,
			m.description,
			m.writeMode,
			strings.Join(m.header, "; "),
			now,
		})
	}

	file, err := os.Create(indexCSVPath())
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}

	w := csv.NewWriter(file)
	if err := w.Write(indexHeader); err != nil {
		return err
	}
	if err := w.WriteAll(records); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func executionCSVPath() string {
	return filepath.Join(dataDir, "monitor_execution_times.csv")
}

func averageCSVPath() string {
	return filepath.Join(dataDir, "monitor_average_times.csv")
}

func monitorCSVPath() string {
	return filepath.Join(dataDir, "monitor_results.csv")
}

func monitorSummaryCSVPath() string {
	return filepath.Join(dataDir, "monitor_summary.csv")
}

func alertSummaryCSVPath() string {
	return filepath.Join(dataDir, "alert_summary.csv")
}

func scraperMetricCSVPath() string {
	return filepath.Join(dataDir, "scraper_metrics.csv")
}

func indexCSVPath() string {
	return filepath.Join(dataDir, "csv_index.csv")
}

func ensureCSV(path string, header []string) error {
	info, err := os.Stat(path)
	if err == nil && info.Size() > 0 {
		return nil
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	w := csv.NewWriter(file)
	if err := w.Write(header); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func readRecords(path string) ([]string, [][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	r := csv.NewReader(file)
	r.FieldsPerRecord = -1
	all, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(all) == 0 {
		return nil, nil, nil
	}
	return all[0], all[1:], nil
}

func appendRecord(path string, record []string) error {
	if err := ensureCSV(path, executionHeader); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	w := csv.NewWriter(file)
	if err := w.Write(record); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func writeRecords(path string, header []string, records [][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	w := csv.NewWriter(file)
	if err := w.Write(header); err != nil {
		return err
	}
	if err := w.WriteAll(records); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func parseSuccess(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1"
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func parseFloat64(s string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return n
}