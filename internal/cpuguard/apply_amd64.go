//go:build amd64

package cpuguard

import (
	"fmt"
	"os"

	"golang.org/x/sys/cpu"
)

// Apply re-execs the current process when the CPU cannot run gomlx's
// AVX2 or FMA kernels. It does not return when re-exec succeeds.
// x/sys/cpu ignores GODEBUG, so the marker is what stops the loop after
// the child starts with cpu.avx2=off.
func Apply() {
	reexec, env := Decide(Features{
		HasAVX:  cpu.X86.HasAVX,
		HasAVX2: cpu.X86.HasAVX2,
		HasFMA:  cpu.X86.HasFMA,
	}, os.Environ())
	if !reexec {
		return
	}
	if err := reexecSelf(env); err != nil {
		fmt.Fprintln(os.Stderr, "mfile:", err)
		os.Exit(1)
	}
}
