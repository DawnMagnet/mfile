package magika

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/DawnMagnet/mfile/assets"
)

func newTestScanner(tb testing.TB) *Scanner {
	tb.Helper()
	s, err := NewScanner(assets.Model, assets.Config, assets.ContentTypesKB)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(s.Close)
	return s
}

func TestBatchLen(t *testing.T) {
	for _, limit := range []int{1, 4, 16} {
		for remaining := 1; remaining <= 3*limit; remaining++ {
			got := batchLen(remaining, limit)
			if got > remaining || got > limit || got&(got-1) != 0 || 2*got <= min(remaining, limit) {
				t.Errorf("batchLen(%d, %d) = %d, want largest power of two <= %d", remaining, limit, got, min(remaining, limit))
			}
		}
	}
	if got := batchLen(5, 5); got != 4 {
		t.Errorf("batchLen(5, 5) = %d, want 4", got)
	}
}

// TestBatchIndependence checks that a file gets the same result whether it
// is scanned alone or as part of a batch split into several bucket sizes and
// run concurrently.
func TestBatchIndependence(t *testing.T) {
	s := newTestScanner(t)
	samples := [][]byte{
		bytes.Repeat([]byte("package main\n\nfunc main() { println(42) }\n"), 50),
		bytes.Repeat([]byte("def f(x):\n    return x * 2\n"), 50),
		bytes.Repeat([]byte(`{"a": [1, 2, 3], "b": {"c": "d"}}`), 50),
		bytes.Repeat([]byte("# Title\n\nSome *markdown* text with a [link](http://x).\n"), 50),
		append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0, 1, 2, 3, 250}, 400)...),
	}
	// 13 inputs run as several full batches plus a remainder.
	var inputs []Input
	for i := range 13 {
		c := samples[i%len(samples)]
		inputs = append(inputs, Input{R: bytes.NewReader(c), Size: len(c)})
	}
	batched, errs := s.ScanAll(inputs)
	for i, in := range inputs {
		if errs[i] != nil {
			t.Fatalf("input %d: %v", i, errs[i])
		}
		alone, aerrs := s.ScanAll([]Input{in})
		if aerrs[0] != nil {
			t.Fatalf("input %d alone: %v", i, aerrs[0])
		}
		if alone[0].Label != batched[i].Label || math.Abs(float64(alone[0].Score-batched[i].Score)) > 1e-5 {
			t.Errorf("input %d: alone %s/%.6f, batched %s/%.6f", i, alone[0].Label, alone[0].Score, batched[i].Label, batched[i].Score)
		}
	}
}

// BenchmarkThroughput measures ms/file for 200 inputs with several worker
// counts, the shape of a typical multi-file invocation.
func BenchmarkThroughput(b *testing.B) {
	s := newTestScanner(b)
	content := bytes.Repeat([]byte("package main\n\nfunc main() { println(42) }\n"), 200)
	inputs := make([]Input, 200)
	for i := range inputs {
		inputs[i] = Input{R: bytes.NewReader(content), Size: len(content)}
	}
	for _, w := range []int{4, 8, 12, 16} {
		b.Run(fmt.Sprintf("workers=%d", w), func(b *testing.B) {
			s.SetWorkers(w)
			s.ScanAll(inputs) // compile all shapes
			b.ResetTimer()
			for b.Loop() {
				s.ScanAll(inputs)
			}
			b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N*len(inputs))/1000, "ms/file")
		})
	}
}

// BenchmarkBatch measures inference per file for each bucket size.
func BenchmarkBatch(b *testing.B) {
	s := newTestScanner(b)
	content := bytes.Repeat([]byte("package main\n\nfunc main() { println(42) }\n"), 200)
	for n := 1; n <= defaultMaxBatch; n *= 2 {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			inputs := make([]Input, n)
			for i := range inputs {
				inputs[i] = Input{R: bytes.NewReader(content), Size: len(content)}
			}
			// Compile outside the timed loop.
			if _, errs := s.ScanAll(inputs); errs[0] != nil {
				b.Fatal(errs[0])
			}
			b.ResetTimer()
			for b.Loop() {
				s.ScanAll(inputs)
			}
			b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N*n)/1000, "ms/file")
		})
	}
}
