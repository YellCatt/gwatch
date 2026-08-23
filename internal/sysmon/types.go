// Package sysmon 实现本机系统资源监控模块。
// 负责采集 CPU、内存、磁盘、网络等指标，周期性聚合为小时/日/月统计，
// 并在超过阈值时通过邮件触发系统告警。
package sysmon

import "time"

// DiskPartition 单个磁盘分区的使用情况。
type DiskPartition struct {
	Name       string  // 分区名（如 C:、/dev/sda1）
	MountPoint string  // 挂载点
	Fstype     string  // 文件系统类型
	Percent    float64 // 使用率百分比
	Used       uint64  // 已用空间（字节）
	Total      uint64  // 总空间（字节）
}

// SystemMetric 一次完整的系统资源快照，包含 CPU、内存、磁盘、网络等关键指标。
type SystemMetric struct {
	CPUPercent       float64         // 当前 CPU 使用率（%）
	CPUMaxPercent    float64         // 聚合周期内的 CPU 最大使用率（%）
	MemoryPercent    float64         // 当前内存使用率（%）
	MemoryMaxPercent float64         // 聚合周期内的内存最大使用率（%）
	MemoryUsed       uint64          // 已用内存（字节）
	MemoryTotal      uint64          // 总内存（字节）
	DiskPercent      float64         // 根分区使用率（%）
	DiskUsed         uint64          // 根分区已用空间（字节）
	DiskTotal        uint64          // 根分区总空间（字节）
	NetDownKBps      float64         // 当前下行速率（KB/s）
	NetUpKBps        float64         // 当前上行速率（KB/s）
	NetDownMaxKBps   float64         // 聚合周期内下行最大速率（KB/s）
	NetUpMaxKBps     float64         // 聚合周期内上行最大速率（KB/s）
	DiskReadKBps     float64         // 磁盘读取速率（KB/s）
	DiskWriteKBps    float64         // 磁盘写入速率（KB/s）
	Partitions       []DiskPartition // 所有磁盘分区详情
	Timestamp        time.Time       // 采集时间
}

// ProcessInfo 单个进程的监控信息。
type ProcessInfo struct {
	PID         int32   // 进程 ID
	Name        string  // 进程名
	CPUPercent  float64 // CPU 占用百分比
	MemPercent  float64 // 内存占用百分比
	MemUsed     uint64  // 已用内存（字节）
	NetDownKBps float64 // 下行速率（KB/s）
	NetUpKBps   float64 // 上行速率（KB/s）
}

// AlertItem 单条系统告警条目。
type AlertItem struct {
	Metric       string        // 告警指标名（cpu/memory/disk 等）
	Value        float64       // 当前值
	Threshold    float64       // 阈值
	Unit         string        // 单位
	Message      string        // 告警描述
	Level        string        // 告警级别（CRITICAL/WARNING）
	Timestamp    time.Time     // 告警发生时间
	TopProcesses []ProcessInfo // 与该指标相关的 Top 进程列表（可选）
	ProcessLabel string        // Top 进程对应的标题标签
}

// ChartData 生成 ASCII 图表所需的时间序列数据。
type ChartData struct {
	Labels         []string  // X 轴标签（时间）
	CPUData        []float64 // CPU 平均使用率序列
	CPUMaxData     []float64 // CPU 最大使用率序列
	MemoryData     []float64 // 内存平均使用率序列
	MemoryMaxData  []float64 // 内存最大使用率序列
	DiskData       []float64 // 磁盘使用率序列
	NetDownData    []float64 // 下行速率序列
	NetUpData      []float64 // 上行速率序列
	NetDownMaxData []float64 // 下行最大速率序列
	NetUpMaxData   []float64 // 上行最大速率序列
}

// MetricRecord 聚合到分钟级的系统指标记录，用于 CSV 存储和后续日/月聚合。
type MetricRecord struct {
	CPUPercent       float64   // CPU 平均使用率
	CPUMaxPercent    float64   // CPU 最大使用率
	MemoryPercent    float64   // 内存平均使用率
	MemoryMaxPercent float64   // 内存最大使用率
	DiskPercent      float64   // 磁盘使用率
	NetDownKBps      float64   // 下行平均速率
	NetUpKBps        float64   // 上行平均速率
	NetDownMaxKBps   float64   // 下行最大速率
	NetUpMaxKBps     float64   // 上行最大速率
	Timestamp        time.Time // 聚合时间
}

// hourlyAggregator 小时级指标累加器，按小时聚合多条 SystemMetric。
type hourlyAggregator struct {
	hour           time.Time                // 当前聚合的小时
	cpuSum         float64                  // CPU 累加值
	cpuMax         float64                  // CPU 最大值
	cpuCount       int                      // CPU 采样数
	memSum         float64                  // 内存累加值
	memMax         float64                  // 内存最大值
	memCount       int                      // 内存采样数
	diskSum        float64                  // 磁盘累加值
	diskCount      int                      // 磁盘采样数
	netDownSum     float64                  // 下行速率累加
	netDownCount   int                      // 下行采样数
	netDownMax     float64                  // 下行最大值
	netUpSum       float64                  // 上行速率累加
	netUpCount     int                      // 上行采样数
	netUpMax       float64                  // 上行最大值
	diskReadSum    float64                  // 磁盘读累加
	diskReadCount  int                      // 磁盘读采样数
	diskWriteSum   float64                  // 磁盘写累加
	diskWriteCount int                      // 磁盘写采样数
	partitionData  map[string]*partitionAgg // 分区累加器（按 Name|MountPoint 聚合）
}

// partitionAgg 单个分区的累加数据。
type partitionAgg struct {
	Name       string  // 分区名
	MountPoint string  // 挂载点
	Fstype     string  // 文件系统类型
	percentSum float64 // 使用率累加
	usedSum    uint64  // 已用空间累加
	totalSum   uint64  // 总空间累加
	count      int     // 采样数
}

// addPartition 将单个分区数据累加到对应聚合器。
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

// getPartitions 基于分区累加结果计算各分区的平均值。
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
// 若指标已包含 max 值（来自低层聚合），则使用 max 字段参与最大比较。
func (a *hourlyAggregator) add(metric SystemMetric) {
	a.cpuSum += metric.CPUPercent
	a.cpuCount++
	cpuVal := metric.CPUPercent
	if metric.CPUMaxPercent > cpuVal {
		cpuVal = metric.CPUMaxPercent
	}
	if cpuVal > a.cpuMax {
		a.cpuMax = cpuVal
	}
	a.memSum += metric.MemoryPercent
	a.memCount++
	memVal := metric.MemoryPercent
	if metric.MemoryMaxPercent > memVal {
		memVal = metric.MemoryMaxPercent
	}
	if memVal > a.memMax {
		a.memMax = memVal
	}
	a.diskSum += metric.DiskPercent
	a.diskCount++
	a.netDownSum += metric.NetDownKBps
	a.netDownCount++
	downVal := metric.NetDownKBps
	if metric.NetDownMaxKBps > downVal {
		downVal = metric.NetDownMaxKBps
	}
	if downVal > a.netDownMax {
		a.netDownMax = downVal
	}
	a.netUpSum += metric.NetUpKBps
	a.netUpCount++
	upVal := metric.NetUpKBps
	if metric.NetUpMaxKBps > upVal {
		upVal = metric.NetUpMaxKBps
	}
	if upVal > a.netUpMax {
		a.netUpMax = upVal
	}
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
		CPUPercent:       safeAvg(a.cpuSum, a.cpuCount),
		CPUMaxPercent:    a.cpuMax,
		MemoryPercent:    safeAvg(a.memSum, a.memCount),
		MemoryMaxPercent: a.memMax,
		DiskPercent:      safeAvg(a.diskSum, a.diskCount),
		NetDownKBps:      safeAvg(a.netDownSum, a.netDownCount),
		NetUpKBps:        safeAvg(a.netUpSum, a.netUpCount),
		NetDownMaxKBps:   a.netDownMax,
		NetUpMaxKBps:     a.netUpMax,
		DiskReadKBps:     safeAvg(a.diskReadSum, a.diskReadCount),
		DiskWriteKBps:    safeAvg(a.diskWriteSum, a.diskWriteCount),
		Partitions:       a.getPartitions(),
		Timestamp:        a.hour,
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
