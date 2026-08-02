package sysmon

import "time"

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
}

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
}

func (a *hourlyAggregator) toSystemMetric() SystemMetric {
	return SystemMetric{
		CPUPercent:    safeAvg(a.cpuSum, a.cpuCount),
		MemoryPercent: safeAvg(a.memSum, a.memCount),
		DiskPercent:   safeAvg(a.diskSum, a.diskCount),
		NetDownKBps:   safeAvg(a.netDownSum, a.netDownCount),
		NetUpKBps:     safeAvg(a.netUpSum, a.netUpCount),
		DiskReadKBps:  safeAvg(a.diskReadSum, a.diskReadCount),
		DiskWriteKBps: safeAvg(a.diskWriteSum, a.diskWriteCount),
		Timestamp:     a.hour,
	}
}

func (a *hourlyAggregator) reset(hour time.Time) {
	*a = hourlyAggregator{hour: hour}
}

func safeAvg(sum float64, count int) float64 {
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
