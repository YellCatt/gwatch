package sysmon

import (
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

// CollectMetrics 采集一次系统指标快照，包括 CPU、内存、磁盘使用率以及网络/磁盘的读写速度。
func CollectMetrics() (SystemMetric, error) {
	metric := SystemMetric{
		Timestamp: timeutil.Now(),
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

	metric.Partitions = collectPartitions()
	if len(metric.Partitions) > 0 {
		rootIdx := 0
		for i, p := range metric.Partitions {
			if p.MountPoint == "/" || p.MountPoint == "\\" || p.Name == "C:" {
				rootIdx = i
				break
			}
		}
		metric.DiskPercent = metric.Partitions[rootIdx].Percent
		metric.DiskUsed = metric.Partitions[rootIdx].Used
		metric.DiskTotal = metric.Partitions[rootIdx].Total
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

// virtualFstypes 虚拟/特殊文件系统类型，不参与分区使用率监控。
var virtualFstypes = map[string]bool{
	"tmpfs":         true,
	"devtmpfs":      true,
	"overlay":       true,
	"squashfs":      true,
	"proc":          true,
	"sysfs":         true,
	"cgroup":        true,
	"cgroup2":       true,
	"devpts":        true,
	"mqueue":        true,
	"pstore":        true,
	"bpf":           true,
	"configfs":      true,
	"debugfs":       true,
	"hugetlbfs":     true,
	"fusectl":       true,
	"autofs":        true,
	"binfmt_misc":   true,
	"securityfs":    true,
	"pstorefs":      true,
	"rpc_pipefs":    true,
	"nfsd":          true,
	"tracefs":       true,
	"binderfs":      true,
}

// collectPartitions 采集所有物理分区的使用率信息，过滤掉虚拟/特殊文件系统。
func collectPartitions() []DiskPartition {
	partitions, err := disk.Partitions(false)
	if err != nil {
		logger.Warn("获取分区列表失败", zap.Error(err))
		return nil
	}

	var result []DiskPartition
	for _, p := range partitions {
		if virtualFstypes[strings.ToLower(p.Fstype)] {
			continue
		}

		usage, err := disk.Usage(p.Mountpoint)
		if err != nil {
			continue
		}

		if usage.Total == 0 {
			continue
		}

		result = append(result, DiskPartition{
			Name:       p.Device,
			MountPoint: p.Mountpoint,
			Fstype:     p.Fstype,
			Percent:    usage.UsedPercent,
			Used:       usage.Used,
			Total:      usage.Total,
		})
	}

	return result
}