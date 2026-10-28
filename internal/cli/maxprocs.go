package cli

import (
	"os"
	"runtime"
)

// CapGOMAXPROCS limits GOMAXPROCS on the SIMD build unless the environment
// already set it. maxProcs is 0 when the cap does not apply.
func CapGOMAXPROCS() {
	if os.Getenv("GOMAXPROCS") == "" && maxProcs > 0 && runtime.GOMAXPROCS(0) > maxProcs {
		runtime.GOMAXPROCS(maxProcs)
	}
}
