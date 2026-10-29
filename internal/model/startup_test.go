package model

import (
	"bytes"
	"testing"
	"time"

	"github.com/DawnMagnet/mfile/assets"
)

// TestStartupPhases logs how long scanner creation and the first (compiling)
// batch take.
func TestStartupPhases(t *testing.T) {
	t0 := time.Now()
	s, err := NewScanner(assets.Model, assets.Config, assets.ContentTypesKB)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t1 := time.Now()
	content := bytes.Repeat([]byte("hello world\n"), 100)
	in := []Input{{R: bytes.NewReader(content), Size: len(content)}}
	if _, errs := s.ScanAll(in); errs[0] != nil {
		t.Fatal(errs[0])
	}
	t2 := time.Now()
	s.ScanAll(in)
	t3 := time.Now()
	t.Logf("NewScanner %v, first batch (compile+run) %v, second batch %v", t1.Sub(t0), t2.Sub(t1), t3.Sub(t2))
}
