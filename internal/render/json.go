package render

import (
	"encoding/json"
	"io"
	"time"

	"github.com/DawnMagnet/mfile/internal/detect"
)

// JSON writes one JSON object per line, in the historical field order.
// Source is not part of that object.
type JSON struct {
	enc *json.Encoder
}

// NewJSON returns a renderer that streams JSON lines to w.
func NewJSON(w io.Writer) *JSON {
	return &JSON{enc: json.NewEncoder(w)}
}

// Begin implements Renderer.
func (j *JSON) Begin() error { return nil }

// Write implements Renderer.
func (j *JSON) Write(r detect.Result) error {
	line := jsonLine{
		Path:    r.Path,
		Kind:    r.Kind,
		Size:    r.Size,
		Mode:    r.Mode,
		ModTime: r.ModTime,
		Target:  r.Target,
		Error:   r.Error,
	}
	if r.Classified {
		line.jsonContent = &jsonContent{
			Label:       r.Label,
			MIME:        r.MIME,
			Group:       r.Group,
			Description: r.Description,
			Extensions:  r.Extensions,
			IsText:      r.IsText,
			Score:       r.Score,
			DLLabel:     r.DLLabel,
		}
	}
	return j.enc.Encode(line)
}

// End implements Renderer.
func (j *JSON) End() error { return nil }

// jsonLine matches the previous CLI object, including field order.
type jsonLine struct {
	Path    string    `json:"path"`
	Kind    string    `json:"kind"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mtime"`
	Target  string    `json:"symlink_target,omitempty"`
	*jsonContent
	Error string `json:"error,omitempty"`
}

type jsonContent struct {
	Label       string   `json:"label"`
	MIME        string   `json:"mime_type"`
	Group       string   `json:"group"`
	Description string   `json:"description"`
	Extensions  []string `json:"extensions"`
	IsText      bool     `json:"is_text"`
	Score       float32  `json:"score"`
	DLLabel     string   `json:"dl_label,omitempty"`
}
