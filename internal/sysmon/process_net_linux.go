//go:build linux

package sysmon

import (
	"fmt"

	"github.com/shirou/gopsutil/v3/process"
)

type procNetCounters struct {
	BytesRecv uint64
	BytesSent uint64
}

func processNetIOCounters(p *process.Process) (*procNetCounters, error) {
	return nil, fmt.Errorf("per-process network I/O not available on Linux")
}