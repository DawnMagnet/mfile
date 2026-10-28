package version

import "testing"

func avx2CPU() CPUInfo {
	return CPUInfo{HasAVX: true, HasAVX2: true, HasFMA: true}
}

func avx512CPU() CPUInfo {
	c := avx2CPU()
	c.HasAVX512F = true
	c.HasAVX512CD = true
	c.HasAVX512BW = true
	c.HasAVX512DQ = true
	c.HasAVX512VL = true
	return c
}

func TestSIMDPath(t *testing.T) {
	exp := Runtime{GOAMD64: 1, SIMDExperiment: true}
	guard := exp
	guard.GODEBUG = "simd=0,cpu.avx2=off,cpu.fma=off"
	guard.GOMLXAVX2 = "0"

	tests := []struct {
		name string
		cpu  CPUInfo
		rt   Runtime
		want string
	}{
		{name: "avx2", cpu: avx2CPU(), rt: exp, want: PathAVX2},
		{name: "avx512", cpu: avx512CPU(), rt: exp, want: PathAVX512},
		{name: "avx only", cpu: CPUInfo{HasAVX: true}, rt: exp, want: PathSoftware},
		{name: "no avx", cpu: CPUInfo{}, rt: exp, want: PathSoftware},
		{name: "no experiment", cpu: avx512CPU(), rt: Runtime{GOAMD64: 1}, want: PathSoftware},
		{name: "guard env on avx2 cpu", cpu: avx2CPU(), rt: guard, want: PathSoftware},
		{name: "guard env on avx512 cpu", cpu: avx512CPU(), rt: guard, want: PathAVX512},
		{
			name: "simd=0 alone keeps avx2 kernels",
			cpu:  avx2CPU(),
			rt:   Runtime{GODEBUG: "simd=0", GOAMD64: 1, SIMDExperiment: true},
			want: PathAVX2,
		},
		{
			name: "gomlx avx2 off",
			cpu:  avx2CPU(),
			rt:   Runtime{GOMLXAVX2: "off", GOAMD64: 1, SIMDExperiment: true},
			want: PathSoftware,
		},
		{
			name: "gomlx avx2 case insensitive",
			cpu:  avx2CPU(),
			rt:   Runtime{GOMLXAVX2: "OFF", GOAMD64: 1, SIMDExperiment: true},
			want: PathSoftware,
		},
		{
			name: "gomlx avx512 off falls through to avx2",
			cpu:  avx512CPU(),
			rt:   Runtime{GOMLXAVX512: "false", GOAMD64: 1, SIMDExperiment: true},
			want: PathAVX2,
		},
		{
			name: "invalid gomlx value keeps default",
			cpu:  avx2CPU(),
			rt:   Runtime{GOMLXAVX2: "maybe", GOAMD64: 1, SIMDExperiment: true},
			want: PathAVX2,
		},
		{
			name: "godebug last wins",
			cpu:  avx2CPU(),
			rt:   Runtime{GODEBUG: "cpu.avx2=off,cpu.avx2=on", GOAMD64: 1, SIMDExperiment: true},
			want: PathAVX2,
		},
		{
			name: "godebug last wins off",
			cpu:  avx2CPU(),
			rt:   Runtime{GODEBUG: "gctrace=1,cpu.avx2=on,cpu.avx2=off", GOAMD64: 1, SIMDExperiment: true},
			want: PathSoftware,
		},
		{
			name: "v3 ignores cpu.avx2=off",
			cpu:  avx2CPU(),
			rt:   Runtime{GODEBUG: "cpu.avx2=off,cpu.fma=off", GOAMD64: 3, SIMDExperiment: true},
			want: PathAVX2,
		},
		{
			name: "v4 ignores avx512f=off",
			cpu:  avx512CPU(),
			rt:   Runtime{GODEBUG: "cpu.avx512f=off", GOAMD64: 4, SIMDExperiment: true},
			want: PathAVX512,
		},
		{
			name: "avx512f off breaks the bundle",
			cpu:  avx512CPU(),
			rt:   Runtime{GODEBUG: "cpu.avx512f=off", GOAMD64: 1, SIMDExperiment: true},
			want: PathAVX2,
		},
		{
			name: "incomplete avx512 is not the bundle",
			cpu:  CPUInfo{HasAVX: true, HasAVX2: true, HasFMA: true, HasAVX512F: true},
			rt:   exp,
			want: PathAVX2,
		},
		{
			name: "cpu.all=off",
			cpu:  avx2CPU(),
			rt:   Runtime{GODEBUG: "cpu.all=off", GOAMD64: 1, SIMDExperiment: true},
			want: PathSoftware,
		},
		{
			name: "cpu.all=off then avx=on",
			cpu:  avx2CPU(),
			rt:   Runtime{GODEBUG: "cpu.all=off,cpu.avx=on", GOAMD64: 1, SIMDExperiment: true},
			want: PathSoftware,
		},
		{
			name: "cannot enable a missing feature",
			cpu:  CPUInfo{},
			rt:   Runtime{GODEBUG: "cpu.all=on,cpu.avx2=on", GOAMD64: 1, SIMDExperiment: true},
			want: PathSoftware,
		},
		{
			name: "fma off does not deselect avx2 kernels",
			cpu:  CPUInfo{HasAVX: true, HasAVX2: true},
			rt:   exp,
			want: PathAVX2,
		},
		{
			name: "user godebug then guard suffix",
			cpu:  avx2CPU(),
			rt: Runtime{
				GODEBUG:        "gctrace=1,simd=1,cpu.avx2=on,simd=0,cpu.avx2=off,cpu.fma=off",
				GOMLXAVX2:      "0",
				GOAMD64:        1,
				SIMDExperiment: true,
			},
			want: PathSoftware,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SIMDPath(tc.cpu, tc.rt); got != tc.want {
				t.Fatalf("SIMDPath() = %s, want %s", got, tc.want)
			}
		})
	}
}
