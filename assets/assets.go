// Package assets embeds the Magika model, its config and the content types
// knowledge base, taken from github.com/google/magika/assets.
//
// model.patched.onnx is model.onnx rewritten by tools/patch_model.py into an
// equivalent graph that onnx-gomlx supports and the Go backend runs fast:
// GlobalMaxPool -> ReduceMax, one-hot + MatMul -> Gather, Conv -> im2col +
// MatMul. Weights are only re-laid out, not changed.
package assets

import _ "embed"

// ModelName is the name of the embedded Magika model.
const ModelName = "standard_v3_3"

//go:embed standard_v3_3/model.patched.onnx
var Model []byte

//go:embed standard_v3_3/config.min.json
var Config []byte

//go:embed content_types_kb.min.json
var ContentTypesKB []byte
