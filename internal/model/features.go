// Feature extraction ported from the upstream Go reference implementation
// (Copyright 2024 Google LLC, Apache-2.0).

package model

import (
	"bytes"
	"fmt"
	"io"
)

// Features holds the model input extracted from a file.
type Features struct {
	// firstBlock holds the first BlockSize bytes, used by the small file rule.
	firstBlock []byte
	Beg        []int32
	Mid        []int32
	End        []int32
}

// ExtractFeatures extracts the model features from r, which holds size bytes.
func ExtractFeatures(cfg Config, r io.ReaderAt, size int) (Features, error) {
	er := errReader{r: r, sz: size}
	beg := er.readAt(0, cfg.BlockSize)
	mid := er.readAt((size-cfg.MidSize)/2, cfg.MidSize)
	end := er.readAt(size-cfg.BlockSize, cfg.BlockSize)
	if er.err != nil {
		return Features{}, er.err
	}
	return buildFeatures(cfg, beg, mid, end), nil
}

// Flatten returns the features in model input order.
func (f Features) Flatten() []int32 {
	res := make([]int32, 0, len(f.Beg)+len(f.Mid)+len(f.End))
	res = append(res, f.Beg...)
	res = append(res, f.Mid...)
	res = append(res, f.End...)
	return res
}

// errReader wraps an io.ReaderAt, accumulates read errors and silently
// clips out of range reads.
type errReader struct {
	r   io.ReaderAt
	sz  int
	err error
}

func (e *errReader) readAt(off, n int) []byte {
	if e.err != nil || off >= e.sz || n <= 0 {
		return nil
	}
	if off < 0 {
		n += off
		off = 0
	}
	n = min(n, e.sz-off)
	b := make([]byte, n)
	p, err := e.r.ReadAt(b, int64(off))
	if err != nil && err != io.EOF {
		e.err = fmt.Errorf("read %d bytes at %d: %w", n, off, err)
		return nil
	}
	return b[:p]
}

const asciiSpaces = "\t\n\v\f\r "

func buildFeatures(cfg Config, beg, mid, end []byte) Features {
	firstBlock := beg
	beg = bytes.TrimLeft(beg, asciiSpaces)
	end = bytes.TrimRight(end, asciiSpaces)
	beg = safeSlice(beg, 0, cfg.BegSize)
	end = safeSlice(end, len(end)-cfg.EndSize, len(end))
	return Features{
		firstBlock: firstBlock,
		Beg:        padInt32(cfg, beg, 0, cfg.BegSize),
		Mid:        padInt32(cfg, mid, (cfg.MidSize-len(mid))/2, cfg.MidSize),
		End:        padInt32(cfg, end, cfg.EndSize-len(end), cfg.EndSize),
	}
}

// padInt32 converts b to int32, with prefix padding tokens in front and
// padding tokens after it up to size.
func padInt32(cfg Config, b []byte, prefix, size int) []int32 {
	r := make([]int32, 0, size)
	for len(r) < prefix {
		r = append(r, int32(cfg.PaddingToken))
	}
	for _, bb := range b {
		r = append(r, int32(bb))
	}
	for len(r) < size {
		r = append(r, int32(cfg.PaddingToken))
	}
	return r
}

func safeSlice(b []byte, from, to int) []byte {
	return b[max(from, 0):min(to, len(b))]
}
