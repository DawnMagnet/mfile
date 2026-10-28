//go:build amd64

package version

import "golang.org/x/sys/cpu"

func hostCPU() CPUInfo {
	return CPUInfo{
		HasAVX:      cpu.X86.HasAVX,
		HasAVX2:     cpu.X86.HasAVX2,
		HasFMA:      cpu.X86.HasFMA,
		HasAVX512F:  cpu.X86.HasAVX512F,
		HasAVX512CD: cpu.X86.HasAVX512CD,
		HasAVX512BW: cpu.X86.HasAVX512BW,
		HasAVX512DQ: cpu.X86.HasAVX512DQ,
		HasAVX512VL: cpu.X86.HasAVX512VL,
	}
}
