package render

import "github.com/DawnMagnet/mfile/internal/detect"

// Renderer streams detection results to an output.
//
// Begin prepares the renderer, Write accepts one result, and End flushes
// anything still buffered. Pretty emits each line from Write as soon as the
// next Seq is available, and emits any remainder from End.
//
// plain.go and json.go implement the same three methods. They belong to
// another change and are not defined here.
type Renderer interface {
	Begin() error
	Write(detect.Result) error
	End() error
}
