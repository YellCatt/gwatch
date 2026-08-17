package sysmon

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

	"gwatch/config"
	"gwatch/internal/timeutil"
	"gwatch/internal/logger"
)

var (
	storageMu sync.Mutex
)

var metricsHeader = []string{
	"time",
	"cpu_percent",
	"memory_percent",
	"disk_percent",
	"net_down_kbps",
	"net_up_kbps",
	"disk_read_kbps",
	"disk_write_kbps",
	"memory_used",
	"memory_total",
	"disk_used",
	"disk_total",
	"sample_count",
}

// hourlyPath 返回小时级指标 CSV 文件路径。
func hourlyPath() string {
	return filepath.Join(config.GlobalConfig.App.DataDir, "system_metrics_hourly.csv")
}

// dailyPath 返回日级指标 CSV 文件路径。
func dailyPath() string {
	return filepath.Join(config.GlobalConfig.App.DataDir, "system_metrics_daily.csv")
}

// monthlyPath 返回月级指标 CSV 文件路径。
func monthlyPath() string {
	return filepath.Join(config.GlobalConfig.App.DataDir, "system_metrics_monthly.csv")
}

// yearlyPath 返回年级指标 CSV 文件路径。
func yearlyPath() string {
	return filepath.Join(config.GlobalConfig.App.DataDir, "system_metrics_yearly.csv")
}

// InitStorage 初始化系统指标存储，确保各级 CSV 文件存在且包含表头。
// 返回 true 表示初始化成功。
func InitStorage() bool {
	paths := []string{hourlyPath(), dailyPath(), monthlyPath(), yearlyPath()}
	ok := true
	for _, p := range paths {
		if err := ensureCSV(p); err != nil {
			logger.Warn("Failed to init metrics storage", zap.String("path", p), zap.Error(err))
			ok = false
		}
	}
	return ok
}

// ensureCSV 确保指定路径的 CSV 文件存在且非空；若不存在则创建并写入表头。
func ensureCSV(path string) error {
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
	if err := w.Write(metricsHeader); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

// recordMetric 将一条系统指标记录追加写入到指定 CSV 文件（线程安全）。
func recordMetric(path string, metric SystemMetric, sampleCount int) error {
	storageMu.Lock()
	defer storageMu.Unlock()

	if err := ensureCSV(path); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	rec := []string{
		metric.Timestamp.Format("2006-01-02 15:04:05"),
		strconv.FormatFloat(metric.CPUPercent, 'f', 2, 64),
		strconv.FormatFloat(metric.MemoryPercent, 'f', 2, 64),
		strconv.FormatFloat(metric.DiskPercent, 'f', 2, 64),
		strconv.FormatFloat(metric.NetDownKBps, 'f', 2, 64),
		strconv.FormatFloat(metric.NetUpKBps, 'f', 2, 64),
		strconv.FormatFloat(metric.DiskReadKBps, 'f', 2, 64),
		strconv.FormatFloat(metric.DiskWriteKBps, 'f', 2, 64),
		strconv.FormatUint(metric.MemoryUsed, 10),
		strconv.FormatUint(metric.MemoryTotal, 10),
		strconv.FormatUint(metric.DiskUsed, 10),
		strconv.FormatUint(metric.DiskTotal, 10),
		strconv.Itoa(sampleCount),
	}

	w := csv.NewWriter(file)
	if err := w.Write(rec); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

// RecordHourlyMetric 将小时级指标记录写入小时 CSV 文件。
func RecordHourlyMetric(metric SystemMetric, sampleCount int) error {
	return recordMetric(hourlyPath(), metric, sampleCount)
}

// RecordDailyMetric 将日级指标记录写入日 CSV 文件。
func RecordDailyMetric(metric SystemMetric, sampleCount int) error {
	return recordMetric(dailyPath(), metric, sampleCount)
}

// RecordMonthlyMetric 将月级指标记录写入月 CSV 文件。
func RecordMonthlyMetric(metric SystemMetric, sampleCount int) error {
	return recordMetric(monthlyPath(), metric, sampleCount)
}

// RecordYearlyMetric 将年级指标记录写入年 CSV 文件。
func RecordYearlyMetric(metric SystemMetric, sampleCount int) error {
	return recordMetric(yearlyPath(), metric, sampleCount)
}

// loadMetrics 从指定 CSV 文件加载系统指标列表，支持按 since 时间过滤。
func loadMetrics(path string, since time.Time) ([]SystemMetric, error) {
	storageMu.Lock()
	defer storageMu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer file.Close()

	r := csv.NewReader(file)
	r.FieldsPerRecord = -1
	all, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(all) <= 1 {
		return nil, nil
	}

	colIndex := make(map[string]int)
	for i, h := range all[0] {
		colIndex[strings.TrimSpace(h)] = i
	}

	var results []SystemMetric

	for _, rec := range all[1:] {
		tsStr := getCol(rec, colIndex, "time")
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", tsStr, time.Local)
		if err != nil {
			continue
		}
		if !since.IsZero() && ts.Before(since) {
			continue
		}

		results = append(results, SystemMetric{
			CPUPercent:    parseFloat(getCol(rec, colIndex, "cpu_percent")),
			MemoryPercent: parseFloat(getCol(rec, colIndex, "memory_percent")),
			MemoryUsed:    parseUint64(getCol(rec, colIndex, "memory_used")),
			MemoryTotal:   parseUint64(getCol(rec, colIndex, "memory_total")),
			DiskPercent:   parseFloat(getCol(rec, colIndex, "disk_percent")),
			DiskUsed:      parseUint64(getCol(rec, colIndex, "disk_used")),
			DiskTotal:     parseUint64(getCol(rec, colIndex, "disk_total")),
			NetDownKBps:   parseFloat(getCol(rec, colIndex, "net_down_kbps")),
			NetUpKBps:     parseFloat(getCol(rec, colIndex, "net_up_kbps")),
			DiskReadKBps:  parseFloat(getCol(rec, colIndex, "disk_read_kbps")),
			DiskWriteKBps: parseFloat(getCol(rec, colIndex, "disk_write_kbps")),
			Timestamp:     ts,
		})
	}

	return results, nil
}

// LoadRecentMetrics 加载最近 N 小时内的小时级指标记录。
func LoadRecentMetrics(hours int) ([]SystemMetric, error) {
	if hours <= 0 {
		hours = 24
	}
	cutoff := timeutil.Now().Add(-time.Duration(hours) * time.Hour)
	return loadMetrics(hourlyPath(), cutoff)
}

// LoadDailyMetrics 加载自指定时间以来的日级指标记录。
func LoadDailyMetrics(since time.Time) ([]SystemMetric, error) {
	return loadMetrics(dailyPath(), since)
}

// LoadMonthlyMetrics 加载自指定时间以来的月级指标记录。
func LoadMonthlyMetrics(since time.Time) ([]SystemMetric, error) {
	return loadMetrics(monthlyPath(), since)
}

// LoadYearlyMetrics 加载自指定时间以来的年级指标记录。
func LoadYearlyMetrics(since time.Time) ([]SystemMetric, error) {
	return loadMetrics(yearlyPath(), since)
}

// aggregateMetrics 聚合一组系统指标，返回平均值指标与样本数量。
func aggregateMetrics(metrics []SystemMetric) (SystemMetric, int) {
	if len(metrics) == 0 {
		return SystemMetric{}, 0
	}

	var agg hourlyAggregator
	agg.reset(metrics[0].Timestamp)
	for _, m := range metrics {
		agg.add(m)
	}
	return agg.toSystemMetric(), len(metrics)
}

// aggregateAndRecord 聚合一组指标并将结果写入指定 CSV 文件。
func aggregateAndRecord(path string, metrics []SystemMetric) error {
	if len(metrics) == 0 {
		return nil
	}
	avg, count := aggregateMetrics(metrics)
	return recordMetric(path, avg, count)
}

// getCol 从 CSV 记录中按列名获取对应字段值。
func getCol(rec []string, colIndex map[string]int, name string) string {
	if idx, ok := colIndex[name]; ok && idx < len(rec) {
		return rec[idx]
	}
	return ""
}

// parseFloat 将字符串解析为 float64，解析失败返回 0。
func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

// parseUint64 将字符串解析为 uint64，解析失败返回 0。
func parseUint64(s string) uint64 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return v
}

// GetStoragePath 返回小时级指标存储文件路径。
func GetStoragePath() string {
	return hourlyPath()
}

// EnsureStorage 确保指标存储目录存在，并初始化小时级 CSV 文件。
func EnsureStorage() error {
	path := hourlyPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}
	return ensureCSV(path)
}