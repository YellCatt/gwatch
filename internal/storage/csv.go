// Package storage 提供基于 CSV 文件的测试执行数据持久化
// 替代原来的 SQLite 数据库，每张表对应一个 CSV 文件
package storage

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
)

// InitDB 初始化 CSV 数据目录（单例模式，保持与原 SQLite 接口一致）
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

	logger.Info("CSV 存储初始化成功",
		zap.String("executionCSV", executionCSVPath()),
		zap.String("averageCSV", averageCSVPath()))
	return nil
}

func executionCSVPath() string {
	return filepath.Join(dataDir, "test_execution_times.csv")
}

func averageCSVPath() string {
	return filepath.Join(dataDir, "test_average_times.csv")
}

// ensureCSV 如果 CSV 文件不存在或为空，则创建并写入表头
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

// readRecords 读取 CSV 文件，返回表头和数据行
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

// appendRecord 向 CSV 文件追加一行记录
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

// writeRecords 覆盖写入 CSV 文件（包含表头）
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

// RecordExecutionTime 记录测试执行时间
func RecordExecutionTime(testCaseID, testCaseDesc, fileName, url string, duration time.Duration, success bool) error {
	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	record := []string{
		testCaseID,
		testCaseDesc,
		fileName,
		url,
		strconv.FormatInt(int64(duration/time.Millisecond), 10),
		strconv.FormatBool(success),
		time.Now().Format("2006-01-02 15:04:05"),
	}

	if err := appendRecord(executionCSVPath(), record); err != nil {
		logger.Error("Failed to record execution time", zap.Error(err))
		return err
	}
	return nil
}

// GetAverageDuration 获取指定 URL 的平均执行时间
func GetAverageDuration(url string) (time.Duration, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return 0, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return 0, err
	}

	var sum int64
	var count int64
	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		if rec[3] != url {
			continue
		}
		if !parseSuccess(rec[5]) {
			continue
		}
		sum += parseInt64(rec[4])
		count++
	}

	if count == 0 {
		return 0, nil
	}
	return time.Duration(sum/count) * time.Millisecond, nil
}

// GetAllAverageDurations 获取所有 URL 的平均执行时间
func GetAllAverageDurations() (map[string]time.Duration, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		logger.Warn("GetAllAverageDurations: storage not initialized")
		return nil, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		logger.Warn("GetAllAverageDurations: read failed", zap.Error(err))
		return nil, err
	}

	type agg struct {
		sum   int64
		count int64
	}
	groups := make(map[string]*agg)

	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		if !parseSuccess(rec[5]) {
			continue
		}
		url := rec[3]
		if groups[url] == nil {
			groups[url] = &agg{}
		}
		groups[url].sum += parseInt64(rec[4])
		groups[url].count++
	}

	averages := make(map[string]time.Duration)
	for url, g := range groups {
		if g.count > 0 {
			averages[url] = time.Duration(g.sum/g.count) * time.Millisecond
		}
	}

	logger.Info("GetAllAverageDurations: found", zap.Int("count", len(averages)), zap.Any("averages", averages))
	if len(averages) == 0 {
		logger.Warn("GetAllAverageDurations: no historical data found")
	}
	return averages, nil
}

// GetExecutionCount 获取指定 URL 的成功执行次数
func GetExecutionCount(url string) (int, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return 0, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return 0, err
	}

	count := 0
	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		if rec[3] == url && parseSuccess(rec[5]) {
			count++
		}
	}
	return count, nil
}

// GetTotalExecutionCount 获取成功执行的总记录数
func GetTotalExecutionCount() (int, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return 0, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return 0, err
	}

	count := 0
	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		if parseSuccess(rec[5]) {
			count++
		}
	}
	return count, nil
}

// CalculateAndStoreAverages 计算所有成功测试用例的平均执行时间并存储到 CSV
func CalculateAndStoreAverages() error {
	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return err
	}

	type agg struct {
		testCaseID   string
		testCaseDesc string
		fileName     string
		url          string
		sum          int64
		count        int64
	}
	groups := make(map[string]*agg)

	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		if !parseSuccess(rec[5]) {
			continue
		}
		testCaseID := rec[0]
		fileName := rec[2]
		url := rec[3]
		key := testCaseID + "\x00" + fileName + "\x00" + url
		if groups[key] == nil {
			var desc string
			if len(rec) > 1 {
				desc = rec[1]
			}
			groups[key] = &agg{
				testCaseID:   testCaseID,
				testCaseDesc: desc,
				fileName:     fileName,
				url:          url,
			}
		}
		groups[key].sum += parseInt64(rec[4])
		groups[key].count++
	}

	var keys []string
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	now := time.Now().Format("2006-01-02 15:04:05")
	var avgRecords [][]string
	for _, k := range keys {
		g := groups[k]
		if g.count == 0 {
			continue
		}
		avg := float64(g.sum) / float64(g.count)
		avgRecords = append(avgRecords, []string{
			g.testCaseID,
			g.testCaseDesc,
			g.fileName,
			g.url,
			strconv.FormatFloat(avg, 'f', -1, 64),
			strconv.FormatInt(g.count, 10),
			now,
		})
		logger.Info("Stored average duration",
			zap.String("test_case_id", g.testCaseID),
			zap.String("file_name", g.fileName),
			zap.String("url", g.url),
			zap.Float64("avg_ms", avg),
			zap.Int64("count", g.count))
	}

	if err := writeRecords(averageCSVPath(), averageHeader, avgRecords); err != nil {
		logger.Error("Failed to store averages", zap.Error(err))
		return err
	}

	return nil
}

// GetAllStoredAverages 获取所有已存储的平均执行时间
func GetAllStoredAverages() ([]map[string]interface{}, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	header, records, err := readRecords(averageCSVPath())
	if err != nil {
		return nil, err
	}

	colIndex := make(map[string]int)
	for i, h := range header {
		colIndex[strings.TrimSpace(h)] = i
	}

	get := func(rec []string, name string) string {
		if idx, ok := colIndex[name]; ok && idx < len(rec) {
			return rec[idx]
		}
		return ""
	}

	var averages []map[string]interface{}
	for _, rec := range records {
		averages = append(averages, map[string]interface{}{
			"test_case_id":        get(rec, "test_case_id"),
			"test_case_desc":      get(rec, "test_case_desc"),
			"file_name":           get(rec, "file_name"),
			"url":                 get(rec, "url"),
			"average_duration_ms": parseFloat64(get(rec, "average_duration_ms")),
			"execution_count":     int(parseInt64(get(rec, "execution_count"))),
			"last_updated":        get(rec, "last_updated"),
		})
	}

	return averages, nil
}