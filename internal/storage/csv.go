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
	if err := ensureCSV(monitorCSVPath(), monitorHeader); err != nil {
		logger.Error("初始化监控记录 CSV 失败", zap.Error(err))
		return err
	}
	if err := ensureCSV(monitorSummaryCSVPath(), monitorSummaryHeader); err != nil {
		logger.Error("初始化监控汇总 CSV 失败", zap.Error(err))
		return err
	}

	logger.Info("CSV 存储初始化成功",
		zap.String("executionCSV", executionCSVPath()),
		zap.String("averageCSV", averageCSVPath()),
		zap.String("monitorCSV", monitorCSVPath()),
		zap.String("monitorSummaryCSV", monitorSummaryCSVPath()))
	return nil
}

func executionCSVPath() string {
	return filepath.Join(dataDir, "test_execution_times.csv")
}

func averageCSVPath() string {
	return filepath.Join(dataDir, "test_average_times.csv")
}

func monitorCSVPath() string {
	return filepath.Join(dataDir, "monitor_results.csv")
}

func monitorSummaryCSVPath() string {
	return filepath.Join(dataDir, "monitor_summary.csv")
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

// MonitorResult 表示监控结果记录
type MonitorResultRecord struct {
	TestCaseID     string
	TestCaseDesc   string
	URL            string
	Method         string
	ExpectedStatus int
	ActualStatus   int
	ExpectedBody   string
	ActualBody     string
	ErrorMsg       string
	DurationMS     int64
	Success        bool
	Timestamp      time.Time
}

// MonitorSummaryRecord 表示监控每日汇总记录
type MonitorSummaryRecord struct {
	Date            string
	TestCaseID      string
	TestCaseDesc    string
	URL             string
	Method          string
	TotalCount      int64
	SuccessCount    int64
	FailedCount     int64
	TotalDurationMS int64
	MinDurationMS   int64
	MaxDurationMS   int64
	LastSuccessTime string
	LastFailureTime string
}

// RecordMonitorResult 记录监控结果到CSV
func RecordMonitorResult(record MonitorResultRecord) error {
	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	rec := []string{
		record.TestCaseID,
		record.TestCaseDesc,
		record.URL,
		record.Method,
		strconv.FormatInt(int64(record.ExpectedStatus), 10),
		strconv.FormatInt(int64(record.ActualStatus), 10),
		record.ExpectedBody,
		record.ActualBody,
		record.ErrorMsg,
		strconv.FormatInt(record.DurationMS, 10),
		strconv.FormatBool(record.Success),
		record.Timestamp.Format("2006-01-02 15:04:05"),
	}

	if err := appendRecord(monitorCSVPath(), rec); err != nil {
		logger.Error("Failed to record monitor result", zap.Error(err))
		return err
	}
	return nil
}

// GetMonitorResultsByDate 获取指定日期的监控结果
func GetMonitorResultsByDate(date time.Time) ([]MonitorResultRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	header, records, err := readRecords(monitorCSVPath())
	if err != nil {
		return nil, err
	}

	if len(header) == 0 {
		return nil, nil
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

	dateStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	dateEnd := dateStart.Add(24 * time.Hour)

	var results []MonitorResultRecord
	for _, rec := range records {
		timestampStr := get(rec, "timestamp")
		timestamp, err := time.Parse("2006-01-02 15:04:05", timestampStr)
		if err != nil {
			continue
		}

		if timestamp.After(dateStart) && timestamp.Before(dateEnd) {
			results = append(results, MonitorResultRecord{
				TestCaseID:     get(rec, "test_case_id"),
				TestCaseDesc:   get(rec, "test_case_desc"),
				URL:            get(rec, "url"),
				Method:         get(rec, "method"),
				ExpectedStatus: int(parseInt64(get(rec, "expected_status"))),
				ActualStatus:   int(parseInt64(get(rec, "actual_status"))),
				ExpectedBody:   get(rec, "expected_body"),
				ActualBody:     get(rec, "actual_body"),
				ErrorMsg:       get(rec, "error_msg"),
				DurationMS:     parseInt64(get(rec, "duration_ms")),
				Success:        parseSuccess(get(rec, "success")),
				Timestamp:      timestamp,
			})
		}
	}

	return results, nil
}

// UpdateMonitorSummary 更新每日监控汇总记录
func UpdateMonitorSummary(record MonitorResultRecord) error {
	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	dateStr := record.Timestamp.Format("2006-01-02")

	header, records, err := readRecords(monitorSummaryCSVPath())
	if err != nil {
		return err
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

	foundIdx := -1
	var summary MonitorSummaryRecord
	for i, rec := range records {
		recDate := get(rec, "date")
		recID := get(rec, "test_case_id")
		if recDate == dateStr && recID == record.TestCaseID {
			foundIdx = i
			summary = MonitorSummaryRecord{
				Date:            recDate,
				TestCaseID:      recID,
				TestCaseDesc:    get(rec, "test_case_desc"),
				URL:             get(rec, "url"),
				Method:          get(rec, "method"),
				TotalCount:      parseInt64(get(rec, "total_count")),
				SuccessCount:    parseInt64(get(rec, "success_count")),
				FailedCount:     parseInt64(get(rec, "failed_count")),
				TotalDurationMS: parseInt64(get(rec, "total_duration_ms")),
				MinDurationMS:   parseInt64(get(rec, "min_duration_ms")),
				MaxDurationMS:   parseInt64(get(rec, "max_duration_ms")),
				LastSuccessTime: get(rec, "last_success_time"),
				LastFailureTime: get(rec, "last_failure_time"),
			}
			break
		}
	}

	if foundIdx == -1 {
		summary = MonitorSummaryRecord{
			Date:          dateStr,
			TestCaseID:    record.TestCaseID,
			TestCaseDesc:  record.TestCaseDesc,
			URL:           record.URL,
			Method:        record.Method,
			MinDurationMS: record.DurationMS,
			MaxDurationMS: record.DurationMS,
		}
	}

	summary.TotalCount++
	summary.TotalDurationMS += record.DurationMS

	if record.DurationMS < summary.MinDurationMS {
		summary.MinDurationMS = record.DurationMS
	}
	if record.DurationMS > summary.MaxDurationMS {
		summary.MaxDurationMS = record.DurationMS
	}

	if record.Success {
		summary.SuccessCount++
		summary.LastSuccessTime = record.Timestamp.Format("2006-01-02 15:04:05")
	} else {
		summary.FailedCount++
		summary.LastFailureTime = record.Timestamp.Format("2006-01-02 15:04:05")
	}

	newRecord := []string{
		summary.Date,
		summary.TestCaseID,
		summary.TestCaseDesc,
		summary.URL,
		summary.Method,
		strconv.FormatInt(summary.TotalCount, 10),
		strconv.FormatInt(summary.SuccessCount, 10),
		strconv.FormatInt(summary.FailedCount, 10),
		strconv.FormatInt(summary.TotalDurationMS, 10),
		strconv.FormatInt(summary.MinDurationMS, 10),
		strconv.FormatInt(summary.MaxDurationMS, 10),
		summary.LastSuccessTime,
		summary.LastFailureTime,
	}

	if foundIdx >= 0 {
		records[foundIdx] = newRecord
	} else {
		records = append(records, newRecord)
	}

	return writeRecords(monitorSummaryCSVPath(), monitorSummaryHeader, records)
}

// GetMonitorSummaryByDate 获取指定日期的监控汇总
func GetMonitorSummaryByDate(date time.Time) ([]MonitorSummaryRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	header, records, err := readRecords(monitorSummaryCSVPath())
	if err != nil {
		return nil, err
	}

	if len(header) == 0 {
		return nil, nil
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

	dateStr := date.Format("2006-01-02")
	var results []MonitorSummaryRecord
	for _, rec := range records {
		if get(rec, "date") == dateStr {
			results = append(results, MonitorSummaryRecord{
				Date:            get(rec, "date"),
				TestCaseID:      get(rec, "test_case_id"),
				TestCaseDesc:    get(rec, "test_case_desc"),
				URL:             get(rec, "url"),
				Method:          get(rec, "method"),
				TotalCount:      parseInt64(get(rec, "total_count")),
				SuccessCount:    parseInt64(get(rec, "success_count")),
				FailedCount:     parseInt64(get(rec, "failed_count")),
				TotalDurationMS: parseInt64(get(rec, "total_duration_ms")),
				MinDurationMS:   parseInt64(get(rec, "min_duration_ms")),
				MaxDurationMS:   parseInt64(get(rec, "max_duration_ms")),
				LastSuccessTime: get(rec, "last_success_time"),
				LastFailureTime: get(rec, "last_failure_time"),
			})
		}
	}

	return results, nil
}

// GetMonitorSummaryByPeriod 获取指定时间段的监控汇总
func GetMonitorSummaryByPeriod(startDate, endDate time.Time) ([]MonitorSummaryRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	header, records, err := readRecords(monitorSummaryCSVPath())
	if err != nil {
		return nil, err
	}

	if len(header) == 0 {
		return nil, nil
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

	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")

	type key struct {
		testCaseID string
		url        string
	}
	aggMap := make(map[key]*MonitorSummaryRecord)

	for _, rec := range records {
		dateStr := get(rec, "date")
		if dateStr < startStr || dateStr >= endStr {
			continue
		}

		k := key{
			testCaseID: get(rec, "test_case_id"),
			url:        get(rec, "url"),
		}

		if aggMap[k] == nil {
			aggMap[k] = &MonitorSummaryRecord{
				Date:          startStr + "~" + endStr,
				TestCaseID:    get(rec, "test_case_id"),
				TestCaseDesc:  get(rec, "test_case_desc"),
				URL:           get(rec, "url"),
				Method:        get(rec, "method"),
				MinDurationMS: 999999999,
			}
		}

		agg := aggMap[k]
		agg.TotalCount += parseInt64(get(rec, "total_count"))
		agg.SuccessCount += parseInt64(get(rec, "success_count"))
		agg.FailedCount += parseInt64(get(rec, "failed_count"))
		agg.TotalDurationMS += parseInt64(get(rec, "total_duration_ms"))

		minMS := parseInt64(get(rec, "min_duration_ms"))
		if minMS < agg.MinDurationMS {
			agg.MinDurationMS = minMS
		}

		maxMS := parseInt64(get(rec, "max_duration_ms"))
		if maxMS > agg.MaxDurationMS {
			agg.MaxDurationMS = maxMS
		}

		lastSuccess := get(rec, "last_success_time")
		if lastSuccess > agg.LastSuccessTime {
			agg.LastSuccessTime = lastSuccess
		}

		lastFailure := get(rec, "last_failure_time")
		if lastFailure > agg.LastFailureTime {
			agg.LastFailureTime = lastFailure
		}
	}

	var results []MonitorSummaryRecord
	for _, agg := range aggMap {
		results = append(results, *agg)
	}

	return results, nil
}
