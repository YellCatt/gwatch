package storage

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"gwatch/internal/logger"
)

// RecordMonitorResult 记录一次监控执行的详细结果到 CSV 存储中。
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

// GetMonitorResultsByDate 获取指定日期的所有监控执行结果明细。
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

// UpdateMonitorSummary 更新或创建指定监控任务的每日汇总记录（增量聚合）。
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

// GetMonitorSummaryByDate 获取指定日期的监控汇总记录。
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

// GetMonitorSummaryByPeriod 获取指定时间区间的监控汇总记录，并按任务 ID + URL 进行跨天聚合。
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
