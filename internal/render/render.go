// Package render writes detection results.
//
// Plain and JSON renderers stream: each Write call emits one result.
// Pretty streams too, and holds a result until every lower Index has been
// written. Buffering renderers (table, tree) can collect results until End
// and should live in their own files.
package render

import "github.com/DawnMagnet/mfile/internal/detect"

// Renderer emits detection results.
// Begin prepares output, Write emits one result, and End finishes.
type Renderer interface {
	Begin() error
	Write(detect.Result) error
	End() error
}
