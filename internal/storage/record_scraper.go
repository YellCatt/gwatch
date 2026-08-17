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

// RecordScraperMetric 记录一条采集器指标到 CSV 存储中。
func RecordScraperMetric(record ScraperMetricRecord) error {
	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	rec := []string{
		record.TargetName,
		record.TargetURL,
		record.MetricName,
		record.MetricAlias,
		strconv.FormatFloat(record.Value, 'f', -1, 64),
		record.Unit,
		strconv.FormatBool(record.Success),
		record.Timestamp.Format("2006-01-02 15:04:05"),
	}

	if err := appendRecord(scraperMetricCSVPath(), rec); err != nil {
		logger.Warn("记录采集器指标失败", zap.Error(err))
		return err
	}
	return nil
}

// GetScraperMetricsByPeriod 获取指定时间区间内的所有采集器指标记录。
func GetScraperMetricsByPeriod(startDate, endDate time.Time) ([]ScraperMetricRecord, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	header, records, err := readRecords(scraperMetricCSVPath())
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

	var results []ScraperMetricRecord
	for _, rec := range records {
		timestampStr := get(rec, "timestamp")
		timestamp, err := time.Parse("2006-01-02 15:04:05", timestampStr)
		if err != nil {
			continue
		}

		if timestamp.After(startDate) && timestamp.Before(endDate) {
			results = append(results, ScraperMetricRecord{
				TargetName:  get(rec, "target_name"),
				TargetURL:   get(rec, "target_url"),
				MetricName:  get(rec, "metric_name"),
				MetricAlias: get(rec, "metric_alias"),
				Value:       parseFloat64(get(rec, "value")),
				Unit:        get(rec, "unit"),
				Success:     parseSuccess(get(rec, "success")),
				Timestamp:   timestamp,
			})
		}
	}

	return results, nil
}

// GetScraperMetricsHourlyAvg 获取指定时间区间内每小时采集器指标的平均值。
func GetScraperMetricsHourlyAvg(startDate, endDate time.Time) ([]ScraperMetricHourlyAvg, error) {
	metrics, err := GetScraperMetricsByPeriod(startDate, endDate)
	if err != nil {
		return nil, err
	}

	type key struct {
		targetName string
		metricName string
		hour       int
	}

	type agg struct {
		metricAlias string
		unit        string
		sum         float64
		count       int
	}

	aggMap := make(map[key]*agg)

	for _, m := range metrics {
		if !m.Success {
			continue
		}
		hour := m.Timestamp.Hour()
		k := key{
			targetName: m.TargetName,
			metricName: m.MetricName,
			hour:       hour,
		}

		if aggMap[k] == nil {
			aggMap[k] = &agg{
				metricAlias: m.MetricAlias,
				unit:        m.Unit,
			}
		}
		aggMap[k].sum += m.Value
		aggMap[k].count++
	}

	var results []ScraperMetricHourlyAvg
	for k, v := range aggMap {
		if v.count > 0 {
			results = append(results, ScraperMetricHourlyAvg{
				TargetName:  k.targetName,
				MetricName:  k.metricName,
				MetricAlias: v.metricAlias,
				Unit:        v.unit,
				Hour:        k.hour,
				AvgValue:    v.sum / float64(v.count),
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].TargetName != results[j].TargetName {
			return results[i].TargetName < results[j].TargetName
		}
		if results[i].MetricName != results[j].MetricName {
			return results[i].MetricName < results[j].MetricName
		}
		return results[i].Hour < results[j].Hour
	})

	return results, nil
}

// GetScraperMetricsDailyAvg 获取指定时间区间内每日采集器指标的平均值。
func GetScraperMetricsDailyAvg(startDate, endDate time.Time) ([]ScraperMetricDailyAvg, error) {
	metrics, err := GetScraperMetricsByPeriod(startDate, endDate)
	if err != nil {
		return nil, err
	}

	if len(metrics) == 0 {
		return nil, nil
	}

	type key struct {
		targetName string
		metricName string
		dayOffset  int
	}

	type agg struct {
		metricAlias string
		unit        string
		sum         float64
		count       int
	}

	aggMap := make(map[key]*agg)

	startOfStartDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())

	for _, m := range metrics {
		if !m.Success {
			continue
		}
		dayOffset := int(m.Timestamp.Sub(startOfStartDate).Hours() / 24)
		k := key{
			targetName: m.TargetName,
			metricName: m.MetricName,
			dayOffset:  dayOffset,
		}

		if aggMap[k] == nil {
			aggMap[k] = &agg{
				metricAlias: m.MetricAlias,
				unit:        m.Unit,
			}
		}
		aggMap[k].sum += m.Value
		aggMap[k].count++
	}

	var results []ScraperMetricDailyAvg
	for k, v := range aggMap {
		if v.count > 0 {
			results = append(results, ScraperMetricDailyAvg{
				TargetName:  k.targetName,
				MetricName:  k.metricName,
				MetricAlias: v.metricAlias,
				Unit:        v.unit,
				Day:         k.dayOffset,
				AvgValue:    v.sum / float64(v.count),
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].TargetName != results[j].TargetName {
			return results[i].TargetName < results[j].TargetName
		}
		if results[i].MetricName != results[j].MetricName {
			return results[i].MetricName < results[j].MetricName
		}
		return results[i].Day < results[j].Day
	})

	return results, nil
}

// GetScraperMetricsMonthlyAvg 获取指定时间区间内每月采集器指标的平均值。
func GetScraperMetricsMonthlyAvg(startDate, endDate time.Time) ([]ScraperMetricMonthlyAvg, error) {
	metrics, err := GetScraperMetricsByPeriod(startDate, endDate)
	if err != nil {
		return nil, err
	}

	if len(metrics) == 0 {
		return nil, nil
	}

	type key struct {
		targetName string
		metricName string
		monthKey   int
	}

	type agg struct {
		metricAlias string
		unit        string
		sum         float64
		count       int
	}

	aggMap := make(map[key]*agg)

	for _, m := range metrics {
		if !m.Success {
			continue
		}
		monthKey := m.Timestamp.Year()*12 + int(m.Timestamp.Month())
		k := key{
			targetName: m.TargetName,
			metricName: m.MetricName,
			monthKey:   monthKey,
		}

		if aggMap[k] == nil {
			aggMap[k] = &agg{
				metricAlias: m.MetricAlias,
				unit:        m.Unit,
			}
		}
		aggMap[k].sum += m.Value
		aggMap[k].count++
	}

	var results []ScraperMetricMonthlyAvg
	for k, v := range aggMap {
		if v.count > 0 {
			month := (k.monthKey % 12)
			if month == 0 {
				month = 12
			}
			results = append(results, ScraperMetricMonthlyAvg{
				TargetName:  k.targetName,
				MetricName:  k.metricName,
				MetricAlias: v.metricAlias,
				Unit:        v.unit,
				Month:       month,
				AvgValue:    v.sum / float64(v.count),
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].TargetName != results[j].TargetName {
			return results[i].TargetName < results[j].TargetName
		}
		if results[i].MetricName != results[j].MetricName {
			return results[i].MetricName < results[j].MetricName
		}
		return results[i].Month < results[j].Month
	})

	return results, nil
}