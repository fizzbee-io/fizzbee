package main

import (
	"bufio"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// tuneGC trades heap headroom for less collector work. Model checking
// allocates a fresh deep copy of the state on every fork, so with the
// default GOGC=100 the collector runs roughly once per doubling of a small
// live heap and ends up taking about half the CPU. A higher percentage lets
// the heap grow further between collections, and the memory limit makes
// the collector aggressive again before the machine swaps. Both are
// defaults only: GOGC and GOMEMLIMIT in the environment win.
func tuneGC() {
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(400)
	}
	if os.Getenv("GOMEMLIMIT") == "" {
		if total := availableMemoryBytes(); total > 0 {
			debug.SetMemoryLimit(int64(float64(total) * 0.8))
		}
	}
}

// availableMemoryBytes is the memory this process can actually use: the
// machine's RAM, capped by the cgroup limit when one is set. Inside a
// container /proc/meminfo reports the host, so RAM alone would overshoot.
func availableMemoryBytes() uint64 {
	total := physicalMemoryBytes()
	if limit := cgroupMemoryLimitBytes(); limit > 0 && (total == 0 || limit < total) {
		return limit
	}
	return total
}

// cgroupMemoryLimitBytes returns the cgroup v2 or v1 memory limit, or 0
// when there is none (no cgroup, or an unlimited one).
func cgroupMemoryLimitBytes() uint64 {
	for _, path := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(data))
		if text == "max" {
			return 0
		}
		limit, err := strconv.ParseUint(text, 10, 64)
		if err != nil {
			continue
		}
		// cgroup v1 reports "no limit" as a huge number near 2^63.
		if limit == 0 || limit >= 1<<62 {
			return 0
		}
		return limit
	}
	return 0
}

// physicalMemoryBytes returns the machine's RAM from /proc/meminfo, or 0
// where that file is missing (non-Linux hosts).
func physicalMemoryBytes() uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0
			}
			return kb * 1024
		}
	}
	return 0
}
