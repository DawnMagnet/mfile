//go:build accuracy

package accuracy

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/DawnMagnet/mfile/assets"
	"github.com/DawnMagnet/mfile/internal/magika"
)

// scoreTol is the allowed absolute difference between mfile and magika scores,
// and between builds / rounds. Labels must match exactly.
const scoreTol = 1e-4

type roundCfg struct {
	workers int
	batch   int
	repeat  bool
}

type fileResult struct {
	Path      string  `json:"path"`
	Label     string  `json:"label"`
	MIME      string  `json:"mime"`
	DL        string  `json:"dl"`
	Score     float64 `json:"score"`
	ScoreBits uint32  `json:"score_bits"`
}

type roundResult struct {
	Workers int          `json:"workers"`
	Batch   int          `json:"batch"`
	Repeat  bool         `json:"repeat"`
	Files   []fileResult `json:"files"`
}

func TestCorpusStabilityAndMagika(t *testing.T) {
	root := moduleRoot(t)
	corpus := filepath.Join(root, "testdata", "corpus")
	paths := samplePaths(t, corpus)
	if len(paths) == 0 {
		t.Fatal("corpus is empty; run scripts/bench/gen_corpus.py")
	}
	inputs, files := openInputs(t, paths)
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()

	scanner, err := magika.NewScanner(assets.Model, assets.Config, assets.ContentTypesKB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(scanner.Close)

	// Five identical rounds, then other worker/batch shapes. The repeats are
	// the GC-stability check; the extra shapes cover concurrency.
	plan := []roundCfg{
		{1, 1, true},
		{1, 1, true},
		{1, 1, true},
		{1, 1, true},
		{1, 1, true},
		{2, 1, false},
		{4, 1, false},
		{4, 4, false},
		{1, 8, false},
	}
	rounds := make([]roundResult, 0, len(plan))
	for _, cfg := range plan {
		scanner.SetWorkers(cfg.workers)
		scanner.SetMaxBatch(cfg.batch)
		got, errs := scanner.ScanAll(inputs)
		rows := make([]fileResult, len(paths))
		for i, path := range paths {
			if errs[i] != nil {
				t.Fatalf("scan %s workers=%d batch=%d: %v", path, cfg.workers, cfg.batch, errs[i])
			}
			rel, err := filepath.Rel(corpus, path)
			if err != nil {
				t.Fatal(err)
			}
			rows[i] = fileResult{
				Path:      filepath.ToSlash(rel),
				Label:     got[i].Label,
				MIME:      got[i].MimeType,
				DL:        got[i].DLLabel,
				Score:     float64(got[i].Score),
				ScoreBits: math.Float32bits(got[i].Score),
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Path < rows[j].Path })
		rounds = append(rounds, roundResult{Workers: cfg.workers, Batch: cfg.batch, Repeat: cfg.repeat, Files: rows})
	}

	var unstable []map[string]any
	base := rounds[0].Files
	for _, round := range rounds[1:] {
		for i, row := range round.Files {
			ref := base[i]
			if row.Path != ref.Path {
				t.Fatalf("path order changed: %s vs %s", row.Path, ref.Path)
			}
			delta := math.Abs(row.Score - ref.Score)
			if row.Label != ref.Label || row.DL != ref.DL || delta > scoreTol {
				t.Errorf("unstable %s workers=%d batch=%d: label %s/%s dl %s/%s score %.8g vs %.8g",
					row.Path, round.Workers, round.Batch, ref.Label, row.Label, ref.DL, row.DL, ref.Score, row.Score)
			}
			if row.ScoreBits != ref.ScoreBits {
				unstable = append(unstable, map[string]any{
					"path":    row.Path,
					"workers": round.Workers,
					"batch":   round.Batch,
					"ref":     ref.Score,
					"got":     row.Score,
					"delta":   delta,
				})
			}
		}
	}
	if len(unstable) > 0 {
		t.Logf("score bits differ on %d file/round pairs (gate allows <= %g)", len(unstable), scoreTol)
	}

	py := magikaResults(t, root, paths)
	var magikaDelta float64
	var magikaMismatches int
	for _, path := range paths {
		rel, _ := filepath.Rel(corpus, path)
		rel = filepath.ToSlash(rel)
		idx := indexOf(rounds[0].Files, rel)
		if idx < 0 {
			t.Fatalf("missing scan row for %s", rel)
		}
		got := rounds[0].Files[idx]
		want, ok := py[path]
		if !ok {
			t.Errorf("magika returned no row for %s", path)
			continue
		}
		if got.Label != want.Label || normDL(got.DL) != normDL(want.DL) {
			magikaMismatches++
			t.Errorf("magika mismatch %s: mfile %s dl=%s magika %s dl=%s", rel, got.Label, got.DL, want.Label, want.DL)
		}
		delta := math.Abs(got.Score - want.Score)
		if delta > magikaDelta {
			magikaDelta = delta
		}
		if delta > scoreTol {
			magikaMismatches++
			t.Errorf("score delta %s: mfile %.8g magika %.8g (%.3g)", rel, got.Score, want.Score, delta)
		}
	}
	t.Logf("files=%d rounds=%d magika_max_delta=%.3g magika_mismatches=%d bit_diffs=%d", len(paths), len(rounds), magikaDelta, magikaMismatches, len(unstable))

	if out := os.Getenv("MFILE_ACCURACY_OUT"); out != "" {
		payload := map[string]any{
			"name":             os.Getenv("MFILE_ACCURACY_NAME"),
			"gogc":             os.Getenv("GOGC"),
			"godebug":          os.Getenv("GODEBUG"),
			"gomlx_avx2":       os.Getenv("GOMLX_GO_SIMD_AVX2"),
			"gomlx_avx512":     os.Getenv("GOMLX_GO_SIMD_AVX512"),
			"magika_max_delta": magikaDelta,
			"bit_diffs":        unstable,
			"rounds":           rounds,
		}
		buf, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, buf, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func indexOf(rows []fileResult, path string) int {
	for i, row := range rows {
		if row.Path == path {
			return i
		}
	}
	return -1
}

func normDL(label string) string {
	if label == "" || label == "undefined" {
		return ""
	}
	return label
}

type pyRow struct {
	Path  string
	Label string
	DL    string
	Score float64
}

func magikaResults(t *testing.T, root string, paths []string) map[string]pyRow {
	t.Helper()
	script := filepath.Join(root, "scripts", "bench", "magika_dump.py")
	req, err := json.Marshal(map[string]any{"paths": paths})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", script)
	cmd.Stdin = bytes.NewReader(req)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("magika dump: %v\n%s", err, stderr.String())
	}
	var payload struct {
		Model   string `json:"model"`
		Results []struct {
			Path  string  `json:"path"`
			Label string  `json:"label"`
			DL    string  `json:"dl"`
			Score float64 `json:"score"`
			Error string  `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Model != "standard_v3_3" {
		t.Fatalf("magika model %q, want standard_v3_3", payload.Model)
	}
	out := make(map[string]pyRow, len(payload.Results))
	for _, row := range payload.Results {
		if row.Error != "" {
			t.Fatalf("magika %s: %s", row.Path, row.Error)
		}
		out[row.Path] = pyRow{Path: row.Path, Label: row.Label, DL: row.DL, Score: row.Score}
	}
	return out
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found from cwd; run from the module")
		}
		dir = parent
	}
}

func samplePaths(t *testing.T, corpus string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(corpus, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() == "expected.tsv" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	return paths
}

func openInputs(t *testing.T, paths []string) ([]magika.Input, []*os.File) {
	t.Helper()
	inputs := make([]magika.Input, len(paths))
	files := make([]*os.File, len(paths))
	for i, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		files[i] = f
		inputs[i] = magika.Input{R: f, Size: int(info.Size())}
	}
	return inputs, files
}
