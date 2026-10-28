package version

import (
	"runtime"
	"strings"
	"testing"

	"github.com/DawnMagnet/mfile/internal/cpuguard"
)

func TestResolve(t *testing.T) {
	if got := Resolve("v1.2.3"); got != "v1.2.3" {
		t.Fatalf("ldflags version: %q", got)
	}
	if got := Resolve(""); got != "dev" {
		t.Fatalf("fallback version: %q, want dev", got)
	}
}

func TestFormatReportsPathAndGuard(t *testing.T) {
	t.Setenv(cpuguard.MarkerVar, cpuguard.MarkerValue)
	out := Format("testver")
	path := SIMDPath(hostCPU(), currentRuntime())
	want := "mfile testver\n" + runtime.Version() + "\nGOAMD64=" + goamd64Label() + "\nsimd=" + path + "\ncpu_guard=true\n"
	if out != want {
		t.Fatalf("format:\n%s\nwant:\n%s", out, want)
	}
	if path != PathAVX512 && path != PathAVX2 && path != PathAVX128NoSIMD && path != PathSoftware {
		t.Fatalf("unexpected path %q", path)
	}
	if !strings.Contains(out, "cpu_guard=true") {
		t.Fatalf("marker not reported:\n%s", out)
	}
}
