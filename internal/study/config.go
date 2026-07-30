package study

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	StudyConfigFileName = "study.json"
	StudyConfigVersion  = 1
)

var (
	ErrStudyConfigMalformed   = errors.New("study config malformed")
	ErrStudyConfigUnsupported = errors.New("study config version unsupported")
	ErrStudyConfigInvalid     = errors.New("study config invalid")
)

type StudyConfig struct {
	Version        int      `json:"version"`
	DimensionOrder []string `json:"dimension_order"`
	Present        bool     `json:"-"`
}

func StudyConfigPath(study Study) string {
	return filepath.Join(study.Path, StudyConfigFileName)
}

func LoadStudyConfig(study Study, dimensions []Dimension) (StudyConfig, []Dimension, error) {
	path := StudyConfigPath(study)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return StudyConfig{Version: StudyConfigVersion}, nil, nil
	}
	if err != nil {
		return StudyConfig{}, nil, fmt.Errorf("read study config %s: %w", path, err)
	}
	defer file.Close()

	var config StudyConfig
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return StudyConfig{}, nil, fmt.Errorf("%w: parse %s: %v", ErrStudyConfigMalformed, path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return StudyConfig{}, nil, fmt.Errorf("%w: parse %s: %v", ErrStudyConfigMalformed, path, err)
	}
	if config.Version != StudyConfigVersion {
		return StudyConfig{}, nil, fmt.Errorf("%w: %s has version %d; expected %d", ErrStudyConfigUnsupported, path, config.Version, StudyConfigVersion)
	}
	config.Present = true

	order := make([]Dimension, 0, len(config.DimensionOrder))
	seen := make(map[string]bool, len(config.DimensionOrder))
	for i, ref := range config.DimensionOrder {
		dimension, err := ResolveDimension(dimensions, ref)
		if err != nil {
			return StudyConfig{}, nil, fmt.Errorf("%w: %s dimension_order[%d]: %v", ErrStudyConfigInvalid, path, i, err)
		}
		if seen[dimension.Ref()] {
			return StudyConfig{}, nil, fmt.Errorf("%w: %s dimension_order[%d] duplicates %q", ErrStudyConfigInvalid, path, i, dimension.Ref())
		}
		seen[dimension.Ref()] = true
		order = append(order, dimension)
	}
	return config, order, nil
}

func renderStudyConfigJSON() string {
	return "{\n  \"version\": 1,\n  \"dimension_order\": []\n}\n"
}
