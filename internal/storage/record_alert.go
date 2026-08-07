package storage

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// UpdateAlertSummary 根据监控执行结果更新或创建告警每日汇总记录。
// 失败为 CRITICAL 级别，慢响应为 WARNING 级别。
func UpdateAlertSummary(record MonitorResultRecord) error {
	if record.Success && record.AlertType == "" {
		return nil
	}

	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	dateStr := record.Timestamp.Format("2006-01-02")
	timestampStr := record.Timestamp.Format("2006-01-02 15:04:05")

	alertLevel := AlertLevelCritical
	if record.Success && record.AlertType == "slow" {
		alertLevel = AlertLevelWarning
	}

	header, records, err := readRecords(alertSummaryCSVPath())
	if err != nil {
		return err
	}

	records = upgradeAlertSummaryRecords(header, records)

	colIndex := make(map[string]int)
	for i, h := range alertSummaryHeader {
		colIndex[h] = i
	}

	get := func(rec []string, name string) string {
		if idx, ok := colIndex[name]; ok && idx < len(rec) {
			return rec[idx]
		}
		return ""
	}

	foundIdx := -1
	var summary AlertSummaryRecord
	for i, rec := range records {
		recDate := get(rec, "date")
		recID := get(rec, "test_case_id")
		if recDate == dateStr && recID == record.TestCaseID {
			foundIdx = i
			summary = AlertSummaryRecord{
				Date:            recDate,
				TestCaseID:      recID,
				TestCaseDesc:    get(rec, "test_case_desc"),
				URL:             get(rec, "url"),
				Method:          get(rec, "method"),
				ExpectedStatus:  int(parseInt64(get(rec, "expected_status"))),
				AlertLevel:      get(rec, "alert_level"),
				AlertCount:      parseInt64(get(rec, "alert_count")),
				FirstOccurrence: get(rec, "first_occurrence"),
				LastOccurrence:  get(rec, "last_occurrence"),
				ErrorMsg:        get(rec, "error_msg"),
			}
			break
		}
	}

	if foundIdx == -1 {
		summary = AlertSummaryRecord{
			Date:            dateStr,
			TestCaseID:      record.TestCaseID,
			TestCaseDesc:    record.TestCaseDesc,
			URL:             record.URL,
			Method:          record.Method,
			ExpectedStatus:  record.ExpectedStatus,
			AlertLevel:      alertLevel,
			FirstOccurrence: timestampStr,
		}
	} else if alertLevelRank(alertLevel) > alertLevelRank(summary.AlertLevel) {
		summary.AlertLevel = alertLevel
	}

	summary.AlertCount++
	summary.LastOccurrence = timestampStr
	if record.ErrorMsg != "" {
		summary.ErrorMsg = record.ErrorMsg
	}

	newRecord := []string{
		summary.Date,
		summary.TestCaseID,
		summary.TestCaseDesc,
		summary.URL,
		summary.Method,
		strconv.FormatInt(int64(summary.ExpectedStatus), 10),
		summary.AlertLevel,
		strconv.FormatInt(summary.AlertCount, 10),
		summary.FirstOccurrence,
		summary.LastOccurrence,
		summary.ErrorMsg,
	}

	if foundIdx >= 0 {
		records[foundIdx] = newRecord
	} else {
		records = append(records, newRecord)
	}

	return writeRecords(alertSummaryCSVPath(), alertSummaryHeader, records)
}

// upgradeAlertSummaryRecords 将旧版告警汇总记录升级为新版（增加 alert_level 字段）。
func upgradeAlertSummaryRecords(header []string, records [][]string) [][]string {
	for _, h := range header {
		if strings.TrimSpace(h) == "alert_level" {
			return records
		}
	}

	upgraded := make([][]string, 0, len(records))
	for _, rec := range records {
		row := make([]string, 10)
		copy(row, rec)
		upgraded = append(upgraded, []string{
			row[0], row[1], row[2], row[3], row[4], row[5],
			AlertLevelCritical,
			row[6], row[7], row[8], row[9],
		})
	}
	return upgraded
}

// GetAlertSummaryByDate 获取指定日期的告警汇总记录。
func GetAlertSummaryByDate(date time.Time) ([]AlertSummaryRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	header, records, err := readRecords(alertSummaryCSVPath())
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
	var results []AlertSummaryRecord
	for _, rec := range records {
		if get(rec, "date") == dateStr {
			results = append(results, AlertSummaryRecord{
				Date:            get(rec, "date"),
				TestCaseID:      get(rec, "test_case_id"),
				TestCaseDesc:    get(rec, "test_case_desc"),
				URL:             get(rec, "url"),
				Method:          get(rec, "method"),
				ExpectedStatus:  int(parseInt64(get(rec, "expected_status"))),
				AlertLevel:      parseAlertLevel(get(rec, "alert_level")),
				AlertCount:      parseInt64(get(rec, "alert_count")),
				FirstOccurrence: get(rec, "first_occurrence"),
				LastOccurrence:  get(rec, "last_occurrence"),
				ErrorMsg:        get(rec, "error_msg"),
			})
		}
	}

	return results, nil
}

// GetAlertSummaryByPeriod 获取指定时间区间的告警汇总记录，并按任务 ID + URL 进行跨天聚合。
func GetAlertSummaryByPeriod(startDate, endDate time.Time) ([]AlertSummaryRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	header, records, err := readRecords(alertSummaryCSVPath())
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
	aggMap := make(map[key]*AlertSummaryRecord)

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
			aggMap[k] = &AlertSummaryRecord{
				Date:            startStr + "~" + endStr,
				TestCaseID:      get(rec, "test_case_id"),
				TestCaseDesc:    get(rec, "test_case_desc"),
				URL:             get(rec, "url"),
				Method:          get(rec, "method"),
				ExpectedStatus:  int(parseInt64(get(rec, "expected_status"))),
				AlertLevel:      parseAlertLevel(get(rec, "alert_level")),
				FirstOccurrence: get(rec, "first_occurrence"),
			}
		}

		agg := aggMap[k]
		agg.AlertCount += parseInt64(get(rec, "alert_count"))
		agg.LastOccurrence = get(rec, "last_occurrence")
		if recLevel := parseAlertLevel(get(rec, "alert_level")); alertLevelRank(recLevel) > alertLevelRank(agg.AlertLevel) {
			agg.AlertLevel = recLevel
		}
		if get(rec, "error_msg") != "" {
			agg.ErrorMsg = get(rec, "error_msg")
		}
	}

	var results []AlertSummaryRecord
	for _, agg := range aggMap {
		results = append(results, *agg)
	}

	return results, nil
}
