//go:build !amd64

package version

func hostCPU() CPUInfo { return CPUInfo{} }
