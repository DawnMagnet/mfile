package cpuguard

import "testing"

func TestDecide(t *testing.T) {
	sandy := Features{HasAVX: true}
	haswellNoFMA := Features{HasAVX: true, HasAVX2: true}
	modern := Features{HasAVX: true, HasAVX2: true, HasFMA: true}
	avx512ish := Features{HasAVX: true, HasAVX2: true, HasFMA: true}

	tests := []struct {
		name    string
		cpu     Features
		env     []string
		reexec  bool
		godebug string
	}{
		{
			name:    "sandy bridge avx only",
			cpu:     sandy,
			reexec:  true,
			godebug: "simd=0,cpu.avx2=off,cpu.fma=off",
		},
		{
			name:    "haswell without fma",
			cpu:     haswellNoFMA,
			reexec:  true,
			godebug: "simd=0,cpu.avx2=off,cpu.fma=off",
		},
		{
			name: "avx2 and fma",
			cpu:  modern,
		},
		{
			name: "avx512 implies no guard",
			cpu:  avx512ish,
		},
		{
			name: "no avx",
			cpu:  Features{},
		},
		{
			name: "no avx even if fma bit is set",
			cpu:  Features{HasFMA: true},
		},
		{
			name: "no avx and no avx2 with empty env",
			cpu:  Features{HasAVX: false, HasAVX2: false, HasFMA: false},
			env:  []string{"PATH=/usr/bin", "GODEBUG="},
		},
		{
			name: "marker prevents loop",
			cpu:  sandy,
			env:  []string{"MFILE_CPU_GUARD=1", "GODEBUG=simd=0,cpu.avx2=off,cpu.fma=off"},
		},
		{
			name:    "marker other value does not count",
			cpu:     sandy,
			env:     []string{"MFILE_CPU_GUARD=0"},
			reexec:  true,
			godebug: "simd=0,cpu.avx2=off,cpu.fma=off",
		},
		{
			name:    "godebug merged and ours last",
			cpu:     sandy,
			env:     []string{"GODEBUG=gctrace=1,simd=1,cpu.avx2=on"},
			reexec:  true,
			godebug: "gctrace=1,simd=1,cpu.avx2=on,simd=0,cpu.avx2=off,cpu.fma=off",
		},
		{
			name:    "empty godebug has no leading comma",
			cpu:     haswellNoFMA,
			env:     []string{"GODEBUG="},
			reexec:  true,
			godebug: "simd=0,cpu.avx2=off,cpu.fma=off",
		},
		{
			name: "case insensitive marker",
			cpu:  sandy,
			env:  []string{"mfile_cpu_guard=1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reexec, env := Decide(tc.cpu, tc.env)
			if reexec != tc.reexec {
				t.Fatalf("reexec=%v, want %v (env %v)", reexec, tc.reexec, env)
			}
			if !tc.reexec {
				if env != nil {
					t.Fatalf("new env = %v, want nil", env)
				}
				return
			}
			if got, _ := lookup(env, "GODEBUG"); got != tc.godebug {
				t.Fatalf("GODEBUG=%q, want %q", got, tc.godebug)
			}
			if stringsCountFold(env, "GODEBUG") != 1 {
				t.Fatalf("GODEBUG entries: %v", env)
			}
			if got, _ := lookup(env, "GOMLX_GO_SIMD_AVX2"); got != "0" {
				t.Fatalf("GOMLX_GO_SIMD_AVX2=%q", got)
			}
			if got, _ := lookup(env, MarkerVar); got != MarkerValue {
				t.Fatalf("marker=%q", got)
			}
			if stringsCountFold(env, "GOMLX_GO_SIMD_AVX2") != 1 || stringsCountFold(env, MarkerVar) != 1 {
				t.Fatalf("duplicate keys: %v", env)
			}
		})
	}
}

func TestDecidePreservesAndReplacesEnv(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"GODEBUG=gctrace=1",
		"godebug=stale",
		"GOMLX_GO_SIMD_AVX2=1",
		"HOME=/home/me",
	}
	reexec, got := Decide(Features{HasAVX: true, HasAVX2: true}, env)
	if !reexec {
		t.Fatal("expected reexec")
	}
	want := []string{
		"PATH=/usr/bin",
		"GODEBUG=gctrace=1,simd=0,cpu.avx2=off,cpu.fma=off",
		"GOMLX_GO_SIMD_AVX2=0",
		"HOME=/home/me",
		"MFILE_CPU_GUARD=1",
	}
	if len(got) != len(want) {
		t.Fatalf("env=%v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("env[%d]=%q, want %q\nfull %v", i, got[i], want[i], got)
		}
	}
}

func stringsCountFold(env []string, key string) int {
	n := 0
	for _, kv := range env {
		k, _, ok := cutKey(kv)
		if ok && equalFold(k, key) {
			n++
		}
	}
	return n
}

func cutKey(kv string) (string, string, bool) {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return "", "", false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
