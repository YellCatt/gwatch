package storage

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func UpdateSystemAlertSummary(record SystemAlertRecord) error {
	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	dateStr := record.Date
	timestampStr := record.LastOccurrence

	header, records, err := readRecords(systemAlertCSVPath())
	if err != nil {
		return err
	}

	records = ensureColumnsSystemAlert(header, records)

	colIndex := make(map[string]int)
	for i, h := range systemAlertHeader {
		colIndex[h] = i
	}

	get := func(rec []string, name string) string {
		if idx, ok := colIndex[name]; ok && idx < len(rec) {
			return rec[idx]
		}
		return ""
	}

	foundIdx := -1
	var existing SystemAlertRecord
	for i, rec := range records {
		if get(rec, "date") == dateStr && get(rec, "metric") == record.Metric {
			foundIdx = i
			existing = SystemAlertRecord{
				Date:            get(rec, "date"),
				Metric:          get(rec, "metric"),
				MetricAlias:     get(rec, "metric_alias"),
				Value:           parseFloat64(get(rec, "value")),
				Threshold:       parseFloat64(get(rec, "threshold")),
				Unit:            get(rec, "unit"),
				AlertLevel:      get(rec, "alert_level"),
				AlertCount:      parseInt64(get(rec, "alert_count")),
				FirstOccurrence: get(rec, "first_occurrence"),
				LastOccurrence:  get(rec, "last_occurrence"),
				Message:         get(rec, "message"),
			}
			break
		}
	}

	if foundIdx == -1 {
		existing = record
		if existing.FirstOccurrence == "" {
			existing.FirstOccurrence = timestampStr
		}
	} else if alertLevelRank(record.AlertLevel) > alertLevelRank(existing.AlertLevel) {
		existing.AlertLevel = record.AlertLevel
		existing.Value = record.Value
		existing.Threshold = record.Threshold
	}

	existing.AlertCount++
	existing.LastOccurrence = timestampStr
	existing.Message = record.Message

	newRecord := []string{
		existing.Date,
		existing.Metric,
		existing.MetricAlias,
		strconv.FormatFloat(existing.Value, 'f', 2, 64),
		strconv.FormatFloat(existing.Threshold, 'f', 2, 64),
		existing.Unit,
		existing.AlertLevel,
		strconv.FormatInt(existing.AlertCount, 10),
		existing.FirstOccurrence,
		existing.LastOccurrence,
		existing.Message,
	}

	if foundIdx >= 0 {
		records[foundIdx] = newRecord
	} else {
		records = append(records, newRecord)
	}

	return writeRecords(systemAlertCSVPath(), systemAlertHeader, records)
}

func ensureColumnsSystemAlert(header []string, records [][]string) [][]string {
	hasAll := make(map[string]bool)
	for _, h := range header {
		hasAll[strings.TrimSpace(h)] = true
	}

	needCol := func(name string, idx int) bool {
		return !hasAll[name]
	}

	updated := make([][]string, 0, len(records))
	for _, rec := range records {
		row := rec
		for _, col := range systemAlertHeader {
			if needCol(col, len(row)) {
				row = append(row, "")
			}
		}
		updated = append(updated, row)
	}
	return updated
}

func GetSystemAlertsByPeriod(startDate, endDate time.Time) ([]SystemAlertRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	header, records, err := readRecords(systemAlertCSVPath())
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
		metric string
	}
	aggMap := make(map[key]*SystemAlertRecord)

	for _, rec := range records {
		dateStr := get(rec, "date")
		if dateStr < startStr || dateStr >= endStr {
			continue
		}

		k := key{metric: get(rec, "metric")}
		if aggMap[k] == nil {
			aggMap[k] = &SystemAlertRecord{
				Date:            startStr + "~" + endStr,
				Metric:          get(rec, "metric"),
				MetricAlias:     get(rec, "metric_alias"),
				Value:           parseFloat64(get(rec, "value")),
				Threshold:       parseFloat64(get(rec, "threshold")),
				Unit:            get(rec, "unit"),
				AlertLevel:      parseAlertLevel(get(rec, "alert_level")),
				FirstOccurrence: get(rec, "first_occurrence"),
			}
		}

		agg := aggMap[k]
		agg.AlertCount += parseInt64(get(rec, "alert_count"))
		agg.LastOccurrence = get(rec, "last_occurrence")
		if alertLevelRank(parseAlertLevel(get(rec, "alert_level"))) > alertLevelRank(agg.AlertLevel) {
			agg.AlertLevel = parseAlertLevel(get(rec, "alert_level"))
		}
		if get(rec, "message") != "" {
			agg.Message = get(rec, "message")
		}
	}

	var results []SystemAlertRecord
	for _, agg := range aggMap {
		results = append(results, *agg)
	}

	return results, nil
}

func UpdateScraperAlertSummary(record ScraperAlertRecord) error {
	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	dateStr := record.Date
	timestampStr := record.LastOccurrence

	_, records, err := readRecords(scraperAlertCSVPath())
	if err != nil {
		return err
	}

	colIndex := make(map[string]int)
	for i, h := range scraperAlertHeader {
		colIndex[h] = i
	}

	get := func(rec []string, name string) string {
		if idx, ok := colIndex[name]; ok && idx < len(rec) {
			return rec[idx]
		}
		return ""
	}

	foundIdx := -1
	var existing ScraperAlertRecord
	for i, rec := range records {
		if get(rec, "date") == dateStr &&
			get(rec, "target_name") == record.TargetName &&
			get(rec, "metric_name") == record.MetricName {
			foundIdx = i
			existing = ScraperAlertRecord{
				Date:            get(rec, "date"),
				TargetName:      get(rec, "target_name"),
				TargetURL:       get(rec, "target_url"),
				MetricName:      get(rec, "metric_name"),
				MetricAlias:     get(rec, "metric_alias"),
				Value:           parseFloat64(get(rec, "value")),
				Threshold:       parseFloat64(get(rec, "threshold")),
				Unit:            get(rec, "unit"),
				AlertLevel:      get(rec, "alert_level"),
				AlertCount:      parseInt64(get(rec, "alert_count")),
				FirstOccurrence: get(rec, "first_occurrence"),
				LastOccurrence:  get(rec, "last_occurrence"),
				Message:         get(rec, "message"),
			}
			break
		}
	}

	if foundIdx == -1 {
		existing = record
		if existing.FirstOccurrence == "" {
			existing.FirstOccurrence = timestampStr
		}
	} else if alertLevelRank(record.AlertLevel) > alertLevelRank(existing.AlertLevel) {
		existing.AlertLevel = record.AlertLevel
		existing.Value = record.Value
		existing.Threshold = record.Threshold
	}

	existing.AlertCount++
	existing.LastOccurrence = timestampStr
	existing.Message = record.Message

	newRecord := []string{
		existing.Date,
		existing.TargetName,
		existing.TargetURL,
		existing.MetricName,
		existing.MetricAlias,
		strconv.FormatFloat(existing.Value, 'f', 2, 64),
		strconv.FormatFloat(existing.Threshold, 'f', 2, 64),
		existing.Unit,
		existing.AlertLevel,
		strconv.FormatInt(existing.AlertCount, 10),
		existing.FirstOccurrence,
		existing.LastOccurrence,
		existing.Message,
	}

	if foundIdx >= 0 {
		records[foundIdx] = newRecord
	} else {
		records = append(records, newRecord)
	}

	return writeRecords(scraperAlertCSVPath(), scraperAlertHeader, records)
}

func GetScraperAlertsByPeriod(startDate, endDate time.Time) ([]ScraperAlertRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	header, records, err := readRecords(scraperAlertCSVPath())
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
		targetName string
		metricName string
	}
	aggMap := make(map[key]*ScraperAlertRecord)

	for _, rec := range records {
		dateStr := get(rec, "date")
		if dateStr < startStr || dateStr >= endStr {
			continue
		}

		k := key{
			targetName: get(rec, "target_name"),
			metricName: get(rec, "metric_name"),
		}
		if aggMap[k] == nil {
			aggMap[k] = &ScraperAlertRecord{
				Date:            startStr + "~" + endStr,
				TargetName:      get(rec, "target_name"),
				TargetURL:       get(rec, "target_url"),
				MetricName:      get(rec, "metric_name"),
				MetricAlias:     get(rec, "metric_alias"),
				Value:           parseFloat64(get(rec, "value")),
				Threshold:       parseFloat64(get(rec, "threshold")),
				Unit:            get(rec, "unit"),
				AlertLevel:      parseAlertLevel(get(rec, "alert_level")),
				FirstOccurrence: get(rec, "first_occurrence"),
			}
		}

		agg := aggMap[k]
		agg.AlertCount += parseInt64(get(rec, "alert_count"))
		agg.LastOccurrence = get(rec, "last_occurrence")
		if alertLevelRank(parseAlertLevel(get(rec, "alert_level"))) > alertLevelRank(agg.AlertLevel) {
			agg.AlertLevel = parseAlertLevel(get(rec, "alert_level"))
		}
		if get(rec, "message") != "" {
			agg.Message = get(rec, "message")
		}
	}

	var results []ScraperAlertRecord
	for _, agg := range aggMap {
		results = append(results, *agg)
	}

	return results, nil
}
