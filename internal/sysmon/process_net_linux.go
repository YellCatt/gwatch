//go:build linux

package sysmon

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type procNetCounters struct {
	BytesRecv uint64
	BytesSent uint64
}

var socketInodeRe = regexp.MustCompile(`socket:\[(\d+)\]`)

func processNetIOCounters(p *process.Process) (*procNetCounters, error) {
	ports := getProcessPorts(p.Pid)
	if len(ports) == 0 {
		return nil, fmt.Errorf("no ports found for pid %d", p.Pid)
	}

	f, err := os.Open("/proc/net/nf_conntrack")
	if err != nil {
		return nil, fmt.Errorf("cannot open nf_conntrack: %v", err)
	}
	defer f.Close()

	var bytesRecv, bytesSent uint64
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		var isOrigUpload, isOrigDownload bool
		byteCount := 0
		var origBytes, replyBytes uint64

		for _, field := range fields {
			if strings.HasPrefix(field, "sport=") {
				portStr := field[len("sport="):]
				if port, err := strconv.Atoi(portStr); err == nil {
					if _, ok := ports[port]; ok && byteCount == 0 {
						isOrigUpload = true
					}
				}
			} else if strings.HasPrefix(field, "dport=") {
				portStr := field[len("dport="):]
				if port, err := strconv.Atoi(portStr); err == nil {
					if _, ok := ports[port]; ok && byteCount == 0 {
						isOrigDownload = true
					}
				}
			} else if strings.HasPrefix(field, "bytes=") {
				val, _ := strconv.ParseUint(strings.TrimPrefix(field, "bytes="), 10, 64)
				if byteCount == 0 {
					origBytes = val
				} else {
					replyBytes = val
				}
				byteCount++
			}
		}

		if byteCount == 0 {
			continue
		}

		if isOrigUpload {
			bytesSent += origBytes
			bytesRecv += replyBytes
		} else if isOrigDownload {
			bytesRecv += origBytes
			bytesSent += replyBytes
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if bytesRecv == 0 && bytesSent == 0 {
		return nil, fmt.Errorf("no netflow for pid %d", p.Pid)
	}

	return &procNetCounters{
		BytesRecv: bytesRecv,
		BytesSent: bytesSent,
	}, nil
}

func getProcessPorts(pid int32) map[int]struct{} {
	ports := make(map[int]struct{})

	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return ports
	}

	socketInodes := make(map[string]struct{})
	for _, entry := range entries {
		link, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			continue
		}
		m := socketInodeRe.FindStringSubmatch(link)
		if m != nil {
			socketInodes[m[1]] = struct{}{}
		}
	}

	if len(socketInodes) == 0 {
		return ports
	}

	for _, proto := range []string{"tcp", "udp"} {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/net/%s", pid, proto))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			inode := fields[9]
			if _, ok := socketInodes[inode]; !ok {
				continue
			}
			if idx := strings.Index(fields[1], ":"); idx != -1 {
				portHex := fields[1][idx+1:]
				if port, err := strconv.ParseInt(portHex, 16, 32); err == nil {
					ports[int(port)] = struct{}{}
				}
			}
		}
	}

	return ports
}