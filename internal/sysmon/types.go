package sysmon

import "time"

type DiskPartition struct {
	Name       string
	MountPoint string
	Fstype     string
	Percent    float64
	Used       uint64
	Total      uint64
}

type SystemMetric struct {
	CPUPercent    float64
	MemoryPercent float64
	MemoryUsed    uint64
	MemoryTotal   uint64
	DiskPercent   float64
	DiskUsed      uint64
	DiskTotal     uint64
	NetDownKBps   float64
	NetUpKBps     float64
	DiskReadKBps  float64
	DiskWriteKBps float64
	Partitions    []DiskPartition
	Timestamp     time.Time
}

type AlertItem struct {
	Metric    string
	Value     float64
	Threshold float64
	Unit      string
	Message   string
	Level     string
	Timestamp time.Time
}

type ChartData struct {
	Labels      []string
	CPUData     []float64
	MemoryData  []float64
	DiskData    []float64
	NetDownData []float64
	NetUpData   []float64
}

type MetricRecord struct {
	CPUPercent    float64
	MemoryPercent float64
	DiskPercent   float64
	NetDownKBps   float64
	NetUpKBps     float64
	Timestamp     time.Time
}

type hourlyAggregator struct {
	hour           time.Time
	cpuSum         float64
	cpuCount       int
	memSum         float64
	memCount       int
	diskSum        float64
	diskCount      int
	netDownSum     float64
	netDownCount   int
	netUpSum       float64
	netUpCount     int
	diskReadSum    float64
	diskReadCount  int
	diskWriteSum   float64
	diskWriteCount int
	partitionData   map[string]*partitionAgg
}

type partitionAgg struct {
	Name       string
	MountPoint string
	Fstype     string
	percentSum float64
	usedSum    uint64
	totalSum   uint64
	count      int
}

func (a *hourlyAggregator) addPartition(p DiskPartition) {
	key := p.Name + "|" + p.MountPoint
	if a.partitionData == nil {
		a.partitionData = make(map[string]*partitionAgg)
	}
	agg, exists := a.partitionData[key]
	if !exists {
		agg = &partitionAgg{Name: p.Name, MountPoint: p.MountPoint, Fstype: p.Fstype}
		a.partitionData[key] = agg
	}
	agg.percentSum += p.Percent
	agg.usedSum += p.Used
	agg.totalSum += p.Total
	agg.count++
}

func (a *hourlyAggregator) getPartitions() []DiskPartition {
	if a.partitionData == nil {
		return nil
	}
	result := make([]DiskPartition, 0, len(a.partitionData))
	for _, agg := range a.partitionData {
		if agg.count == 0 {
			continue
		}
		result = append(result, DiskPartition{
			Name:       agg.Name,
			MountPoint: agg.MountPoint,
			Fstype:     agg.Fstype,
			Percent:    agg.percentSum / float64(agg.count),
			Used:       agg.usedSum / uint64(agg.count),
			Total:      agg.totalSum / uint64(agg.count),
		})
	}
	return result
}

// add 将一条系统指标累加进小时聚合器的对应字段。
func (a *hourlyAggregator) add(metric SystemMetric) {
	a.cpuSum += metric.CPUPercent
	a.cpuCount++
	a.memSum += metric.MemoryPercent
	a.memCount++
	a.diskSum += metric.DiskPercent
	a.diskCount++
	a.netDownSum += metric.NetDownKBps
	a.netDownCount++
	a.netUpSum += metric.NetUpKBps
	a.netUpCount++
	a.diskReadSum += metric.DiskReadKBps
	a.diskReadCount++
	a.diskWriteSum += metric.DiskWriteKBps
	a.diskWriteCount++
	for _, p := range metric.Partitions {
		a.addPartition(p)
	}
}

// toSystemMetric 基于累加结果计算各字段的平均值，生成聚合后的 SystemMetric。
func (a *hourlyAggregator) toSystemMetric() SystemMetric {
	return SystemMetric{
		CPUPercent:    safeAvg(a.cpuSum, a.cpuCount),
		MemoryPercent: safeAvg(a.memSum, a.memCount),
		DiskPercent:   safeAvg(a.diskSum, a.diskCount),
		NetDownKBps:   safeAvg(a.netDownSum, a.netDownCount),
		NetUpKBps:     safeAvg(a.netUpSum, a.netUpCount),
		DiskReadKBps:  safeAvg(a.diskReadSum, a.diskReadCount),
		DiskWriteKBps: safeAvg(a.diskWriteSum, a.diskWriteCount),
		Partitions:    a.getPartitions(),
		Timestamp:     a.hour,
	}
}

// reset 重置小时聚合器到指定小时，清空所有累加数据。
func (a *hourlyAggregator) reset(hour time.Time) {
	*a = hourlyAggregator{hour: hour}
}

// safeAvg 安全计算平均值，count 为 0 时返回 0，避免除零错误。
func safeAvg(sum float64, count int) float64 {
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}