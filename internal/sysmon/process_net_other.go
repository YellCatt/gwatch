//go:build !windows && !linux

package sysmon

import "github.com/shirou/gopsutil/v3/process"

type procNetCounters struct {
	BytesRecv uint64
	BytesSent uint64
}

func processNetIOCounters(p *process.Process) (*procNetCounters, error) {
	return nil, nil
}