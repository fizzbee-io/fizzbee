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
//
// 200 is the measured middle: on Kafka transactions 3 brokers x 3 clients
// (15,498 states, 4 cores) GOGC=400 peaks at 4.7 GB of RSS for the same
// check time as GOGC=200 at 3.2 GB, because the larger heap costs page
// faults what it saves in marking; GOGC=100 peaks at 2.7 GB but doubles
// the collector's CPU, which is free only while cores are idle.
func tuneGC() {
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(200)
	}
	if os.Getenv("GOMEMLIMIT") == "" {
		if total := physicalMemoryBytes(); total > 0 {
			debug.SetMemoryLimit(int64(float64(total) * 0.8))
		}
	}
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
