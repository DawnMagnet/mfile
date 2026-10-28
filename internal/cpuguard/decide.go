// Package cpuguard re-execs mfile on CPUs where gomlx's SIMD kernels
// would raise SIGILL.
//
// qemu runs showed SIGILL on Sandy Bridge (AVX, no AVX2): gomlx
// simdAddVSInt32 at simd128 calls archsimd.BroadcastInt32x4, which is
// the AVX2 instruction VPBROADCASTD. Haswell without FMA also SIGILLs:
// TanhAVX2 uses VFMADD without checking FMA. CPUs without AVX never
// take those paths and are not re-exec'd.
//
// The replacement environment disables the Go simd experiment, masks
// AVX2 and FMA off in GODEBUG (appended last so they win), and sets
// GOMLX_GO_SIMD_AVX2=0. That combination is what cleared both faults.
package cpuguard

import "strings"

const (
	// MarkerVar is set on the re-exec'd process so it does not re-exec again.
	MarkerVar = "MFILE_CPU_GUARD"
	// MarkerValue is the only value that means the guard itself re-exec'd.
	// Any other non-empty value means the user set the variable manually.
	MarkerValue = "reexec"

	gomlxAVX2Var = "GOMLX_GO_SIMD_AVX2"
	godebugKey   = "GODEBUG"
	// Ours are appended last so they override any earlier simd/avx2/fma setting.
	godebugSuffix = "simd=0,cpu.avx2=off,cpu.fma=off"
)

// Features are the CPU flags the guard decision depends on.
// They must be the hardware flags, not flags already masked by GODEBUG:
// golang.org/x/sys/cpu reads CPUID directly.
type Features struct {
	HasAVX  bool
	HasAVX2 bool
	HasFMA  bool
}

// Decide reports whether this process should re-exec, and the environment
// to give the replacement. newEnv is nil when reexec is false.
//
// A non-empty marker skips the re-exec. The replacement is given
// MarkerValue ("reexec"), and a child that already has that value must
// not re-exec again. Any other non-empty value is a manual skip: the
// user set the variable, and this process must not re-exec either.
// Unset and empty are the same, and take the normal CPU decision.
func Decide(f Features, env []string) (reexec bool, newEnv []string) {
	if !needsGuard(f) {
		return false, nil
	}
	if got, ok := lookup(env, MarkerVar); ok && got != "" {
		return false, nil
	}
	godebug, _ := lookup(env, godebugKey)
	merged := godebugSuffix
	if godebug != "" {
		merged = godebug + "," + godebugSuffix
	}
	newEnv = setEnv(env, godebugKey, merged)
	newEnv = setEnv(newEnv, gomlxAVX2Var, "0")
	newEnv = setEnv(newEnv, MarkerVar, MarkerValue)
	return true, newEnv
}

// needsGuard is true for AVX-only CPUs and for AVX2 CPUs that lack FMA.
// A CPU with neither AVX nor AVX2 does not match.
func needsGuard(f Features) bool {
	return (f.HasAVX && !f.HasAVX2) || (f.HasAVX2 && !f.HasFMA)
}

func lookup(env []string, key string) (string, bool) {
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok && strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

// setEnv replaces every matching key, keeping the first occurrence's position.
// Keys are compared case-insensitively because Windows environment blocks are,
// and the Go runtime returns the first match.
func setEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	found := false
	entry := key + "=" + value
	for _, kv := range env {
		k, _, ok := strings.Cut(kv, "=")
		if ok && strings.EqualFold(k, key) {
			if !found {
				out = append(out, entry)
				found = true
			}
			continue
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, entry)
	}
	return out
}
