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
	Metric     string
	Value      float64
	Threshold  float64
	Unit       string
	Message    string
	Level      string
	Timestamp  time.Time
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