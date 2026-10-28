//go:build !amd64

package cpuguard

// Apply is a no-op off amd64. The SIGILL cases are x86 SIMD kernels.
func Apply() {}
