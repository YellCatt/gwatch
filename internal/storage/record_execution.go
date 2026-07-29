package storage

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
)

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