//go:build !windows && !linux

package cli

// peakRSS is not implemented on this platform.
func peakRSS() (uint64, bool) {
	return 0, false
}
