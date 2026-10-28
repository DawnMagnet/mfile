package version

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/DawnMagnet/mfile/internal/cpuguard"
)

// Resolve returns the version to print. ldflagsVersion is main.version
// (-X main.version=...). Otherwise the build-info module version is used,
// and "dev" when that is empty or "(devel)".
func Resolve(ldflagsVersion string) string {
	if ldflagsVersion != "" {
		return ldflagsVersion
	}
	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// Format is the --version text: version, Go version, GOAMD64 level,
// the SIMD path actually taken, and whether the CPU guard re-exec'd.
func Format(ldflagsVersion string) string {
	return fmt.Sprintf("mfile %s\n%s\nGOAMD64=%s\nsimd=%s\ncpu_guard=%s\n",
		Resolve(ldflagsVersion),
		runtime.Version(),
		goamd64Label(),
		SIMDPath(hostCPU(), currentRuntime()),
		cpuGuardLabel(os.Getenv(cpuguard.MarkerVar)),
	)
}

// cpuGuardLabel maps MFILE_CPU_GUARD to the cpu_guard field.
// Only the exact re-exec marker is "true". Any other non-empty value was
// set manually and is "manual", never a real re-exec. Unset and empty are
// "false".
func cpuGuardLabel(marker string) string {
	switch marker {
	case "":
		return "false"
	case cpuguard.MarkerValue:
		return "true"
	default:
		return "manual"
	}
}

func currentRuntime() Runtime {
	return Runtime{
		GODEBUG:        os.Getenv("GODEBUG"),
		GOMLXAVX2:      os.Getenv("GOMLX_GO_SIMD_AVX2"),
		GOMLXAVX512:    os.Getenv("GOMLX_GO_SIMD_AVX512"),
		GOAMD64:        goamd64Level(),
		SIMDExperiment: simdExperiment,
	}
}

func goamd64Level() int {
	switch goamd64Label() {
	case "v2":
		return 2
	case "v3":
		return 3
	case "v4":
		return 4
	default:
		return 1
	}
}

func goamd64Label() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "n/a"
	}
	for _, s := range info.Settings {
		if s.Key == "GOAMD64" && s.Value != "" {
			return s.Value
		}
	}
	return "n/a"
}
