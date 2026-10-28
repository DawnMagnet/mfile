package version

import "strings"

const (
	// PathAVX512 is the gomlx AVX-512 kernel (F+CD+BW+DQ+VL).
	PathAVX512 = "avx512"
	// PathAVX2 is the gomlx AVX2 kernel.
	PathAVX2 = "avx2"
	// PathSoftware is scalar code. That includes a CPU with no AVX, AVX
	// without the AVX2/AVX-512 kernels (the CPU-guard fallback produces
	// the same bytes as this path), and a binary built without
	// GOEXPERIMENT=simd (those kernels are not compiled in).
	// cpu_guard is what distinguishes a guard fallback from a CPU with no AVX.
	PathSoftware = "software"
)

// CPUInfo is the hardware feature set, before GODEBUG masking.
// HasAVX512 is the Go runtime's bundled feature: AVX512F+CD+BW+DQ+VL.
type CPUInfo struct {
	HasAVX      bool
	HasAVX2     bool
	HasFMA      bool
	HasAVX512F  bool
	HasAVX512CD bool
	HasAVX512BW bool
	HasAVX512DQ bool
	HasAVX512VL bool
}

// Runtime is the environment that selects a SIMD implementation.
type Runtime struct {
	// GODEBUG is the raw variable. cpu.* is applied the way internal/cpu
	// does (last assignment wins; cpu.all covers every optional feature;
	// features required by GOAMD64 cannot be cleared). GODEBUG=simd=0 does
	// not by itself turn off gomlx's archsimd AVX2/AVX-512 kernels; those
	// follow the cpu feature flags and the GOMLX_GO_SIMD_* variables.
	GODEBUG        string
	GOMLXAVX2      string
	GOMLXAVX512    string
	GOAMD64        int
	SIMDExperiment bool
}

// SIMDPath returns the implementation this process will actually run.
// The path is inferred by mirroring gomlx's kernel-selection logic. It is
// not queried from gomlx, so it can drift when gomlx is upgraded.
func SIMDPath(cpu CPUInfo, rt Runtime) string {
	eff := effectiveCPU(cpu, rt.GODEBUG, rt.GOAMD64)
	if rt.SIMDExperiment {
		if eff.hasAVX512() && envBool(rt.GOMLXAVX512, true) {
			return PathAVX512
		}
		if eff.HasAVX2 && envBool(rt.GOMLXAVX2, true) {
			return PathAVX2
		}
	}
	return PathSoftware
}

func (c CPUInfo) hasAVX512() bool {
	return c.HasAVX512F && c.HasAVX512CD && c.HasAVX512BW && c.HasAVX512DQ && c.HasAVX512VL
}

// envBool matches github.com/gomlx/compute/support/envutil.MustReadBool
// without panicking: an empty or unrecognized value keeps the default.
func envBool(val string, def bool) bool {
	if val == "" {
		return def
	}
	switch strings.ToLower(val) {
	case "1", "true", "t", "yes", "on", "enabled":
		return true
	case "0", "false", "f", "no", "off", "disabled":
		return false
	default:
		return def
	}
}

func effectiveCPU(cpu CPUInfo, godebug string, goamd64 int) CPUInfo {
	type bit struct {
		name string
		val  *bool
	}
	out := cpu
	var bits []bit
	add := func(name string, v *bool) {
		bits = append(bits, bit{name, v})
	}
	if goamd64 < 3 {
		add("avx", &out.HasAVX)
		add("avx2", &out.HasAVX2)
		add("fma", &out.HasFMA)
	}
	if goamd64 < 4 {
		add("avx512f", &out.HasAVX512F)
		add("avx512cd", &out.HasAVX512CD)
		add("avx512bw", &out.HasAVX512BW)
		add("avx512dq", &out.HasAVX512DQ)
		add("avx512vl", &out.HasAVX512VL)
	}
	specified := map[string]bool{}
	enable := map[string]bool{}
	known := map[string]bool{}
	for _, b := range bits {
		known[b.name] = true
	}
	for _, field := range strings.Split(godebug, ",") {
		if !strings.HasPrefix(field, "cpu.") {
			continue
		}
		key, val, ok := strings.Cut(field[len("cpu."):], "=")
		if !ok {
			continue
		}
		var on bool
		switch val {
		case "on":
			on = true
		case "off":
			on = false
		default:
			continue
		}
		if key == "all" {
			for _, b := range bits {
				specified[b.name] = true
				enable[b.name] = on
			}
			continue
		}
		if !known[key] {
			continue
		}
		specified[key] = true
		enable[key] = on
	}
	for _, b := range bits {
		if !specified[b.name] {
			continue
		}
		if enable[b.name] && !*b.val {
			// GODEBUG cannot turn on a feature the CPU does not have.
			continue
		}
		*b.val = enable[b.name]
	}
	return out
}
