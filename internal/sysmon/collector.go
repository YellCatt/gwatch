package sysmon

import (
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"

	"gwatch/internal/logger"
	"gwatch/internal/timeutil"
)

const (
	maxProcessNetBytesPerSample uint64 = 1024 * 1024 * 1024
)

// CollectMetrics 采集一次系统指标快照，包括 CPU、内存、磁盘使用率以及网络/磁盘的读写速度。
func CollectMetrics() (SystemMetric, error) {
	metric := SystemMetric{
		Timestamp: timeutil.Now(),
	}

	cpuPct, err := cpu.Percent(0, false)
	if err == nil && len(cpuPct) > 0 {
		metric.CPUPercent = cpuPct[0]
		metric.CPUMaxPercent = cpuPct[0]
	}

	memStat, err := mem.VirtualMemory()
	if err == nil {
		metric.MemoryPercent = memStat.UsedPercent
		metric.MemoryMaxPercent = memStat.UsedPercent
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

	const maxNetBytesPerSample uint64 = 1024 * 1024 * 1024 * 1024
	if downDiff > maxNetBytesPerSample {
		downDiff = 0
	}
	if upDiff > maxNetBytesPerSample {
		upDiff = 0
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

	const maxDiskBytesPerSample uint64 = 1024 * 1024 * 1024 * 1024
	if readDiff > maxDiskBytesPerSample {
		readDiff = 0
	}
	if writeDiff > maxDiskBytesPerSample {
		writeDiff = 0
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

type ProcessSortBy int

const (
	SortByCPU ProcessSortBy = iota
	SortByMem
	SortByNet
)

type procNetSnap struct {
	pid       int32
	downBytes uint64
	upBytes   uint64
}

func CollectAllProcesses() []ProcessInfo {
	procs, err := process.Processes()
	if err != nil {
		logger.Warn("获取进程列表失败", zap.Error(err))
		return nil
	}

	pidIndex := make(map[int32]int)
	var infos []ProcessInfo
	var netSnaps []procNetSnap

	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}

		pid := p.Pid
		p.CPUPercent()

		idx := len(infos)
		pidIndex[pid] = idx
		infos = append(infos, ProcessInfo{
			PID:  pid,
			Name: name,
		})

		if counters, err := processNetIOCounters(p); err == nil && counters != nil {
			netSnaps = append(netSnaps, procNetSnap{
				pid:       pid,
				downBytes: counters.BytesRecv,
				upBytes:   counters.BytesSent,
			})
		}
	}

	time.Sleep(1 * time.Second)

	for _, p := range procs {
		snapIdx, ok := pidIndex[p.Pid]
		if !ok {
			continue
		}

		cpuPercent, _ := p.CPUPercent()
		memPercent, _ := p.MemoryPercent()
		memInfo, _ := p.MemoryInfo()
		var memUsed uint64
		if memInfo != nil {
			memUsed = memInfo.RSS
		}

		infos[snapIdx].CPUPercent = cpuPercent
		infos[snapIdx].MemPercent = float64(memPercent)
		infos[snapIdx].MemUsed = memUsed

		if counters, err := processNetIOCounters(p); err == nil && counters != nil {
			for _, snap := range netSnaps {
				if snap.pid == p.Pid {
					if counters.BytesRecv >= snap.downBytes {
						downDiff := counters.BytesRecv - snap.downBytes
						if downDiff > 1024 && downDiff < maxProcessNetBytesPerSample {
							infos[snapIdx].NetDownKBps = float64(downDiff) / 1024.0
						}
					}
					if counters.BytesSent >= snap.upBytes {
						upDiff := counters.BytesSent - snap.upBytes
						if upDiff > 1024 && upDiff < maxProcessNetBytesPerSample {
							infos[snapIdx].NetUpKBps = float64(upDiff) / 1024.0
						}
					}
					break
				}
			}
		}
	}

	return infos
}

func SortProcesses(procs []ProcessInfo, sortBy ProcessSortBy) []ProcessInfo {
	sorted := make([]ProcessInfo, len(procs))
	copy(sorted, procs)

	sort.Slice(sorted, func(i, j int) bool {
		switch sortBy {
		case SortByMem:
			return sorted[i].MemPercent > sorted[j].MemPercent
		case SortByNet:
			netI := sorted[i].NetDownKBps + sorted[i].NetUpKBps
			netJ := sorted[j].NetDownKBps + sorted[j].NetUpKBps
			return netI > netJ
		default:
			return sorted[i].CPUPercent > sorted[j].CPUPercent
		}
	})

	return sorted
}

func CollectTopProcesses(n int) []ProcessInfo {
	if n <= 0 {
		n = 5
	}

	infos := CollectAllProcesses()
	sorted := SortProcesses(infos, SortByCPU)

	if len(sorted) > n {
		sorted = sorted[:n]
	}

	return sorted
}