package cli

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// peakRSS returns the process peak resident set size (VmHWM) in bytes.
func peakRSS() (uint64, bool) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, false
	}
	defer func() {
		// Read-only status file; a close error is not actionable.
		_ = f.Close()
	}()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		v, ok := strings.CutPrefix(sc.Text(), "VmHWM:")
		if !ok {
			continue
		}
		kb, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
		if err != nil {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}
