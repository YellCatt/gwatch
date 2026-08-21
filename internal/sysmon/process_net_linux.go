//go:build linux

package sysmon

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type procNetCounters struct {
	BytesRecv uint64
	BytesSent uint64
}

func processNetIOCounters(p *process.Process) (*procNetCounters, error) {
	pid := p.Pid
	netDevPath := filepath.Join("/proc", strconv.Itoa(int(pid)), "net", "dev")

	f, err := os.Open(netDevPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var totalRecv, totalSent uint64
	scanner := bufio.NewScanner(f)
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine {
			firstLine = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		recv, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		sent, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}
		totalRecv += recv
		totalSent += sent
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if totalRecv == 0 && totalSent == 0 {
		return nil, fmt.Errorf("no network counters available for pid %d", pid)
	}
	return &procNetCounters{
		BytesRecv: totalRecv,
		BytesSent: totalSent,
	}, nil
}