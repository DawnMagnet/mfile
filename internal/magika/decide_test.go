package magika

import "testing"

func TestDecideSource(t *testing.T) {
	s := &Scanner{
		cfg: Config{
			TargetLabelsSpace:         []string{"go", "bin"},
			MediumConfidenceThreshold: 0.5,
			Thresholds:                map[string]float32{"go": 0.8},
			Overwrite:                 map[string]string{"bin": "unknown"},
		},
		kb: map[string]ContentType{
			"go":      {Label: "go", IsText: true, Description: "Go source"},
			"bin":     {Label: "bin", IsText: false, Description: "binary"},
			"txt":     {Label: "txt", IsText: true, Description: "text"},
			"unknown": {Label: "unknown", Description: "unknown"},
		},
	}

	hi := s.decide([]float32{0.9, 0.1})
	if hi.Source != SourceModel || hi.Label != "go" || hi.DLLabel != "go" {
		t.Fatalf("high score: %+v", hi)
	}

	lo := s.decide([]float32{0.2, 0.1})
	if lo.Source != SourceFallback || lo.Label != "txt" || lo.DLLabel != "go" {
		t.Fatalf("low score text: %+v", lo)
	}

	// overwrite_map applies, then a high score stays on the model path.
	over := s.decide([]float32{0.05, 0.95})
	if over.Source != SourceModel || over.Label != "unknown" || over.DLLabel != "bin" {
		t.Fatalf("overwrite: %+v", over)
	}

	// A low score on a non-text label falls back to unknown.
	s.cfg.Thresholds["bin"] = 0.99
	fb := s.decide([]float32{0.01, 0.5})
	if fb.Source != SourceFallback || fb.Label != "unknown" || fb.DLLabel != "bin" {
		t.Fatalf("low score binary: %+v", fb)
	}
}

func TestRuleSource(t *testing.T) {
	s := &Scanner{kb: map[string]ContentType{
		labelEmpty: {Label: labelEmpty, Description: "Empty file"},
	}}
	got := s.ruleResult(labelEmpty)
	if got.Source != SourceRule || got.Label != labelEmpty || got.Score != 1 {
		t.Fatalf("%+v", got)
	}
}
