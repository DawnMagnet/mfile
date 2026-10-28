package magika

import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"unicode/utf8"

	"github.com/gomlx/compute"
	"github.com/gomlx/compute/gobackend"
	"github.com/gomlx/gomlx/core/graph"
	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/model"
	"github.com/gomlx/onnx-gomlx/onnx/parser"
)

// defaultMaxBatch is the default largest number of rows per inference call.
// Calls only use power-of-two batch sizes up to the maximum, so few graph
// shapes are ever compiled and no row is padding. One row per call, with
// calls running concurrently, is both the fastest and the smallest setting
// on the Go backend: its pooled buffers scale with the batch size.
const defaultMaxBatch = 1

// Result is the outcome of scanning one input.
type Result struct {
	ContentType
	// Score is the model confidence, or 1 when a rule decided the label.
	Score float32 `json:"score"`
	// DLLabel is the raw model prediction, empty when the model was not run.
	DLLabel string `json:"dl_label,omitempty"`
}

// Scanner runs the Magika model on the pure Go (SimpleGo) gomlx backend.
// ScanAll is not safe for concurrent use; it parallelizes internally.
type Scanner struct {
	cfg      Config
	kb       map[string]ContentType
	backend  compute.Backend
	exec     *model.Exec
	inLen    int
	workers  int
	maxBatch int
}

// NewScanner builds a scanner from the serialized ONNX model, its config and
// the content types knowledge base.
func NewScanner(onnxModel, config, contentTypesKB []byte) (*Scanner, error) {
	cfg, err := parseConfig(config)
	if err != nil {
		return nil, err
	}
	kb, err := parseContentTypesKB(contentTypesKB)
	if err != nil {
		return nil, err
	}
	for _, l := range cfg.TargetLabelsSpace {
		if _, ok := kb[l]; !ok {
			return nil, fmt.Errorf("label %q missing from content types KB", l)
		}
	}
	m, err := parser.Parse(onnxModel)
	if err != nil {
		return nil, fmt.Errorf("parse onnx model: %w", err)
	}
	store := model.NewStore()
	if err := m.VariablesToScope(store.RootScope()); err != nil {
		return nil, fmt.Errorf("load onnx variables: %w", err)
	}
	backend, err := gobackend.New("")
	if err != nil {
		return nil, fmt.Errorf("create go backend: %w", err)
	}
	exec, err := model.NewExec(backend, store, func(scope *model.Scope, x *graph.Node) *graph.Node {
		return m.CallGraph(scope, x.Graph(), map[string]*graph.Node{"bytes": x})[0]
	})
	if err != nil {
		return nil, fmt.Errorf("create exec: %w", err)
	}
	return &Scanner{
		cfg:      cfg,
		kb:       kb,
		backend:  backend,
		exec:     exec,
		inLen:    cfg.BegSize + cfg.MidSize + cfg.EndSize,
		workers:  runtime.GOMAXPROCS(0),
		maxBatch: defaultMaxBatch,
	}, nil
}

// SetWorkers sets the number of concurrent inference calls (at least 1).
// The default is GOMAXPROCS.
func (s *Scanner) SetWorkers(n int) {
	s.workers = max(n, 1)
}

// SetMaxBatch sets the largest number of rows per inference call, rounded
// down to a power of two (at least 1). The default is 1; larger batches use
// more memory and were not faster in measurements.
func (s *Scanner) SetMaxBatch(n int) {
	s.maxBatch = batchLen(max(n, 1), max(n, 1))
}

// Close releases the compiled graph and the backend.
func (s *Scanner) Close() {
	s.exec.Finalize()
	s.backend.Finalize()
}

// Input is one item to scan.
type Input struct {
	R    io.ReaderAt
	Size int
}

// ScanAll scans all inputs, running the model in power-of-two batches.
// The i-th result or error corresponds to the i-th input.
func (s *Scanner) ScanAll(inputs []Input) ([]Result, []error) {
	results := make([]Result, len(inputs))
	errs := make([]error, len(inputs))
	var pending []int
	var feats [][]int32
	var batches [][2]int
	for i, in := range inputs {
		r, ft, needDL, err := s.prescan(in)
		if err != nil {
			errs[i] = err
			continue
		}
		if !needDL {
			results[i] = r
			continue
		}
		pending = append(pending, i)
		feats = append(feats, ft.Flatten())
	}
	for start := 0; start < len(pending); {
		n := batchLen(len(pending)-start, s.maxBatch)
		batches = append(batches, [2]int{start, start + n})
		start += n
	}
	// Batches run concurrently: the backend parallelizes the big matmuls
	// within one call, but the elementwise, slice and transpose ops between
	// them leave most cores idle unless several calls overlap.
	work := make(chan [2]int)
	var wg sync.WaitGroup
	for range min(s.workers, len(batches)) {
		wg.Go(func() {
			for b := range work {
				scores, err := s.run(feats[b[0]:b[1]])
				for j, idx := range pending[b[0]:b[1]] {
					if err != nil {
						errs[idx] = err
						continue
					}
					results[idx] = s.decide(scores[j])
				}
			}
		})
	}
	for _, b := range batches {
		work <- b
	}
	close(work)
	wg.Wait()
	return results, errs
}

// batchLen returns the largest power of two that is <= min(remaining, limit).
func batchLen(remaining, limit int) int {
	n := 1
	for 2*n <= min(remaining, limit) {
		n *= 2
	}
	return n
}

// prescan applies the rules that do not need the model.
func (s *Scanner) prescan(in Input) (Result, Features, bool, error) {
	if in.Size == 0 {
		return s.ruleResult(labelEmpty), Features{}, false, nil
	}
	ft, err := ExtractFeatures(s.cfg, in.R, in.Size)
	if err != nil {
		return Result{}, Features{}, false, fmt.Errorf("extract features: %w", err)
	}
	// Too few meaningful bytes after stripping whitespace: do not use the model.
	if ft.Beg[s.cfg.MinFileSizeForDl-1] == int32(s.cfg.PaddingToken) {
		if utf8.Valid(ft.firstBlock) {
			return s.ruleResult(labelTxt), ft, false, nil
		}
		return s.ruleResult(labelUnknown), ft, false, nil
	}
	return Result{}, ft, true, nil
}

func (s *Scanner) ruleResult(label string) Result {
	return Result{ContentType: s.kb[label], Score: 1}
}

// run executes the model on the given feature rows; len(rows) is the batch
// dimension.
func (s *Scanner) run(rows [][]int32) ([][]float32, error) {
	flat := make([]int32, 0, len(rows)*s.inLen)
	for _, r := range rows {
		flat = append(flat, r...)
	}
	in := tensors.FromFlatDataAndDimensions(flat, len(rows), s.inLen)
	// Finalize tensors eagerly so their backend buffers return to the pool
	// for the next call instead of waiting for the GC.
	defer func() { _ = in.FinalizeAll() }()
	out, err := s.exec.Call(in)
	if err != nil {
		return nil, fmt.Errorf("run model: %w", err)
	}
	defer func() {
		for _, t := range out {
			_ = t.FinalizeAll()
		}
	}()
	probs, err := tensors.CopyFlatData[float32](out[0])
	if err != nil {
		return nil, fmt.Errorf("read model output: %w", err)
	}
	n := len(s.cfg.TargetLabelsSpace)
	if len(probs) != len(rows)*n {
		return nil, fmt.Errorf("model output has %d values, want %d", len(probs), len(rows)*n)
	}
	res := make([][]float32, len(rows))
	for i := range rows {
		res[i] = probs[i*n : (i+1)*n]
	}
	return res, nil
}

// decide turns model scores into a result, following the reference
// implementation: overwrite map, per-label thresholds, then txt/unknown
// fallback for low confidence predictions.
func (s *Scanner) decide(scores []float32) Result {
	best := 0
	for i, v := range scores {
		if v > scores[best] {
			best = i
		}
	}
	dl := s.cfg.TargetLabelsSpace[best]
	score := scores[best]
	label := dl
	if o, ok := s.cfg.Overwrite[dl]; ok {
		label = o
	}
	th := s.cfg.MediumConfidenceThreshold
	if t, ok := s.cfg.Thresholds[dl]; ok {
		th = t
	}
	if score < th {
		if s.kb[dl].IsText {
			label = labelTxt
		} else {
			label = labelUnknown
		}
	}
	return Result{ContentType: s.kb[label], Score: score, DLLabel: dl}
}
