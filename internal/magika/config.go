package magika

import (
	"encoding/json"
	"fmt"
)

const (
	labelEmpty   = "empty"
	labelTxt     = "txt"
	labelUnknown = "unknown"
)

// Config holds the inference-relevant part of a Magika model config.
type Config struct {
	BegSize                   int                `json:"beg_size"`
	MidSize                   int                `json:"mid_size"`
	EndSize                   int                `json:"end_size"`
	MediumConfidenceThreshold float32            `json:"medium_confidence_threshold"`
	MinFileSizeForDl          int                `json:"min_file_size_for_dl"`
	PaddingToken              int                `json:"padding_token"`
	BlockSize                 int                `json:"block_size"`
	TargetLabelsSpace         []string           `json:"target_labels_space"`
	Thresholds                map[string]float32 `json:"thresholds"`
	Overwrite                 map[string]string  `json:"overwrite_map"`
}

// ContentType describes one entry of the content types knowledge base.
type ContentType struct {
	Label       string   `json:"label"`
	MimeType    string   `json:"mime_type"`
	Group       string   `json:"group"`
	Description string   `json:"description"`
	Extensions  []string `json:"extensions"`
	IsText      bool     `json:"is_text"`
}

func parseConfig(b []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}
	if cfg.BegSize+cfg.MidSize+cfg.EndSize == 0 || len(cfg.TargetLabelsSpace) == 0 {
		return Config{}, fmt.Errorf("invalid config: empty input or label space")
	}
	return cfg, nil
}

func parseContentTypesKB(b []byte) (map[string]ContentType, error) {
	var kb map[string]ContentType
	if err := json.Unmarshal(b, &kb); err != nil {
		return nil, fmt.Errorf("unmarshal content types KB: %w", err)
	}
	for label, ct := range kb {
		ct.Label = label
		kb[label] = ct
	}
	return kb, nil
}
