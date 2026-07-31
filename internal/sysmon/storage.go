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
	"gwatch/internal/logger"
)

var (
	storagePath string
	storageMu   sync.Mutex
)

var hourlyHeader = []string{
	"hour",
	"cpu_percent",
	"memory_percent",
	"disk_percent",
	"net_down_kbps",
	"net_up_kbps",
	"disk_read_kbps",
	"disk_write_kbps",
	"sample_count",
}

func InitStorage() {
	storagePath = filepath.Join(config.GlobalConfig.App.DataDir, "system_metrics_hourly.csv")
	if err := ensureHourlyCSV(); err != nil {
		logger.Error("Failed to init system metrics hourly storage", zap.Error(err))
	}
}

func ensureHourlyCSV() error {
	info, err := os.Stat(storagePath)
	if err == nil && info.Size() > 0 {
		return nil
	}

	file, err := os.Create(storagePath)
	if err != nil {
		return err
	}
	defer file.Close()

	w := csv.NewWriter(file)
	if err := w.Write(hourlyHeader); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func RecordHourlyMetric(metric SystemMetric, sampleCount int) error {
	storageMu.Lock()
	defer storageMu.Unlock()

	if err := ensureHourlyCSV(); err != nil {
		return err
	}

	file, err := os.OpenFile(storagePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
		strconv.Itoa(sampleCount),
	}

	w := csv.NewWriter(file)
	if err := w.Write(rec); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func LoadRecentMetrics(hours int) ([]SystemMetric, error) {
	storageMu.Lock()
	defer storageMu.Unlock()

	if hours <= 0 {
		hours = 24
	}

	file, err := os.Open(storagePath)
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

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	var results []SystemMetric

	for _, rec := range all[1:] {
		tsStr := getCol(rec, colIndex, "hour")
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", tsStr, time.Local)
		if err != nil {
			continue
		}
		if ts.Before(cutoff) {
			continue
		}

		results = append(results, SystemMetric{
			CPUPercent:    parseFloat(getCol(rec, colIndex, "cpu_percent")),
			MemoryPercent: parseFloat(getCol(rec, colIndex, "memory_percent")),
			DiskPercent:   parseFloat(getCol(rec, colIndex, "disk_percent")),
			NetDownKBps:   parseFloat(getCol(rec, colIndex, "net_down_kbps")),
			NetUpKBps:     parseFloat(getCol(rec, colIndex, "net_up_kbps")),
			DiskReadKBps:  parseFloat(getCol(rec, colIndex, "disk_read_kbps")),
			DiskWriteKBps: parseFloat(getCol(rec, colIndex, "disk_write_kbps")),
			Timestamp:     ts,
		})
	}

	return results, nil
}

func getCol(rec []string, colIndex map[string]int, name string) string {
	if idx, ok := colIndex[name]; ok && idx < len(rec) {
		return rec[idx]
	}
	return ""
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func GetStoragePath() string {
	return storagePath
}

func EnsureStorage() error {
	if storagePath == "" {
		storagePath = filepath.Join(config.GlobalConfig.App.DataDir, "system_metrics_hourly.csv")
	}
	if err := os.MkdirAll(filepath.Dir(storagePath), 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}
	return ensureHourlyCSV()
}