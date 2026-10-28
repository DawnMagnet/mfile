// Package detect stats paths and classifies regular files with Magika.
package detect

import (
	"time"

	"github.com/DawnMagnet/mfile/internal/magika"
)

const (
	// SourceModel means the model score was accepted, after overwrite_map.
	SourceModel = magika.SourceModel
	// SourceFallback means a low score was replaced with txt or unknown.
	SourceFallback = magika.SourceFallback
	// SourceRule means a pre-model rule decided (empty, or below min size).
	SourceRule = magika.SourceRule
)

// Result is one path after stat and, for regular files, content detection.
// Label, MIME, group, description, score, size, and source describe a
// classification. The remaining fields preserve the metadata the CLI prints.
//
// Index is not part of the JSON object. Plain and JSON output stay in the
// historical shape; Index is for callers that need to match a result back
// to an input after a future renderer reorders or buffers them.
type Result struct {
	// Index is the 0-based position of this result in the input list, in the
	// order paths were given. Directories are one input, not walked.
	Index       int
	Path        string
	Label       string
	MIME        string
	Group       string
	Description string
	Score       float32
	Size        int64
	// Source is model, fallback, or rule. Empty when the path was not classified.
	Source string

	Kind       string
	Mode       string
	ModTime    time.Time
	Target     string
	Extensions []string
	IsText     bool
	DLLabel    string
	Error      string
	// Classified is true when a content type was produced for this path.
	Classified bool
}
