//go:build windows

package sysmon

import "github.com/shirou/gopsutil/v3/process"

type procNetCounters struct {
	BytesRecv uint64
	BytesSent uint64
}

func processNetIOCounters(p *process.Process) (*procNetCounters, error) {
	counters, err := p.NetIOCounters()
	if err != nil {
		return nil, err
	}
	if counters == nil {
		return nil, nil
	}
	return &procNetCounters{
		BytesRecv: counters.BytesRecv,
		BytesSent: counters.BytesSent,
	}, nil
}