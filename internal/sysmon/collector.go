package sysmon

import (
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// CollectMetrics 采集一次系统指标快照，包括 CPU、内存、磁盘使用率以及网络/磁盘的读写速度。
func CollectMetrics() (SystemMetric, error) {
	metric := SystemMetric{
		Timestamp: time.Now(),
	}

	cpuPct, err := cpu.Percent(0, false)
	if err == nil && len(cpuPct) > 0 {
		metric.CPUPercent = cpuPct[0]
	}

	memStat, err := mem.VirtualMemory()
	if err == nil {
		metric.MemoryPercent = memStat.UsedPercent
		metric.MemoryUsed = memStat.Used
		metric.MemoryTotal = memStat.Total
	}

	diskStat, err := disk.Usage("/")
	if err == nil {
		metric.DiskPercent = diskStat.UsedPercent
		metric.DiskUsed = diskStat.Used
		metric.DiskTotal = diskStat.Total
	}

	netSpeed := calcNetworkSpeed()
	metric.NetDownKBps = netSpeed.Down
	metric.NetUpKBps = netSpeed.Up

	diskSpeed := calcDiskSpeed()
	metric.DiskReadKBps = diskSpeed.Read
	metric.DiskWriteKBps = diskSpeed.Write

	return metric, nil
}

type netSpeedResult struct {
	Down float64
	Up   float64
}

// calcNetworkSpeed 通过两次间隔 1 秒的网络 I/O 计数器差值计算当前网络上/下行速度（KB/s）。
func calcNetworkSpeed() netSpeedResult {
	io1, err := net.IOCounters(false)
	if err != nil || len(io1) == 0 {
		return netSpeedResult{}
	}
	time.Sleep(1 * time.Second)
	io2, err := net.IOCounters(false)
	if err != nil || len(io2) == 0 {
		return netSpeedResult{}
	}

	var downDiff, upDiff uint64
	for i := range io1 {
		if i < len(io2) {
			if io2[i].BytesRecv >= io1[i].BytesRecv {
				downDiff += io2[i].BytesRecv - io1[i].BytesRecv
			}
			if io2[i].BytesSent >= io1[i].BytesSent {
				upDiff += io2[i].BytesSent - io1[i].BytesSent
			}
		}
	}

	return netSpeedResult{
		Down: float64(downDiff) / 1024.0,
		Up:   float64(upDiff) / 1024.0,
	}
}

type diskSpeedResult struct {
	Read  float64
	Write float64
}

// calcDiskSpeed 通过两次间隔 1 秒的磁盘 I/O 计数器差值计算当前磁盘读/写速度（KB/s）。
func calcDiskSpeed() diskSpeedResult {
	io1, err := disk.IOCounters()
	if err != nil {
		return diskSpeedResult{}
	}
	time.Sleep(1 * time.Second)
	io2, err := disk.IOCounters()
	if err != nil {
		return diskSpeedResult{}
	}

	var readDiff, writeDiff uint64
	for name, v1 := range io1 {
		if v2, ok := io2[name]; ok {
			if v2.ReadBytes >= v1.ReadBytes {
				readDiff += v2.ReadBytes - v1.ReadBytes
			}
			if v2.WriteBytes >= v1.WriteBytes {
				writeDiff += v2.WriteBytes - v1.WriteBytes
			}
		}
	}

	return diskSpeedResult{
		Read:  float64(readDiff) / 1024.0,
		Write: float64(writeDiff) / 1024.0,
	}
}

// GetHostInfo 获取主机名和操作系统平台信息。
func GetHostInfo() (string, string) {
	info, err := host.Info()
	if err != nil {
		return "", ""
	}
	return info.Hostname, info.Platform
}
