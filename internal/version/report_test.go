package version

import (
	"os"
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
	tests := []struct {
		name   string
		marker string
		unset  bool
		want   string
	}{
		{name: "unset", unset: true, want: "false"},
		{name: "empty", marker: "", want: "false"},
		{name: "reexec", marker: cpuguard.MarkerValue, want: "true"},
		{name: "one", marker: "1", want: "manual"},
		{name: "arbitrary", marker: "debug", want: "manual"},
		{name: "reexec with trailing space", marker: "reexec ", want: "manual"},
		{name: "uppercase", marker: "REEXEC", want: "manual"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unset {
				t.Setenv(cpuguard.MarkerVar, "sentinel")
				if err := os.Unsetenv(cpuguard.MarkerVar); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv(cpuguard.MarkerVar, tc.marker)
			}
			out := Format("testver")
			path := SIMDPath(hostCPU(), currentRuntime())
			want := "mfile testver\n" + runtime.Version() + "\nGOAMD64=" + goamd64Label() + "\nsimd=" + path + "\ncpu_guard=" + tc.want + "\n"
			if out != want {
				t.Fatalf("format:\n%s\nwant:\n%s", out, want)
			}
			if path != PathAVX512 && path != PathAVX2 && path != PathAVX128NoSIMD && path != PathSoftware {
				t.Fatalf("unexpected path %q", path)
			}
			if !strings.Contains(out, "cpu_guard="+tc.want+"\n") {
				t.Fatalf("cpu_guard field:\n%s", out)
			}
			if tc.want != "true" && strings.Contains(out, "cpu_guard=true") {
				t.Fatalf("manual marker reported as a re-exec:\n%s", out)
			}
		})
	}
}
