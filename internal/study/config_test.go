package study

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStudyConfigResolvesOrderedDimensionReferences(t *testing.T) {
	_, study := executionFixture(t)
	writeReport(t, filepath.Join(study.Path, "dimensions", "02-runtime.md"), "# Runtime\n")
	writeReport(t, StudyConfigPath(study), "{\n  \"version\": 1,\n  \"dimension_order\": [\"runtime\", \"01\"]\n}\n")
	dimensions, err := DiscoverDimensions(study)
	if err != nil {
		t.Fatal(err)
	}

	config, order, err := LoadStudyConfig(study, dimensions)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Present || config.Version != StudyConfigVersion {
		t.Fatalf("config = %#v", config)
	}
	if len(order) != 2 || order[0].Ref() != "02-runtime" || order[1].Ref() != "01-structure" {
		t.Fatalf("order = %#v", order)
	}
}

func TestLoadStudyConfigMissingPreservesNaturalBehavior(t *testing.T) {
	_, study := executionFixture(t)
	dimensions, err := DiscoverDimensions(study)
	if err != nil {
		t.Fatal(err)
	}
	config, order, err := LoadStudyConfig(study, dimensions)
	if err != nil {
		t.Fatal(err)
	}
	if config.Present || config.Version != StudyConfigVersion || len(order) != 0 {
		t.Fatalf("config = %#v order = %#v", config, order)
	}
}

func TestLoadStudyConfigRejectsMalformedUnsupportedUnknownAndDuplicateValues(t *testing.T) {
	_, study := executionFixture(t)
	writeReport(t, filepath.Join(study.Path, "dimensions", "02-runtime.md"), "# Runtime\n")
	dimensions, err := DiscoverDimensions(study)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		content string
		target  error
	}{
		{name: "malformed", content: "{", target: ErrStudyConfigMalformed},
		{name: "unknown field", content: `{"version":1,"dimension_order":[],"extra":true}`, target: ErrStudyConfigMalformed},
		{name: "unsupported", content: `{"version":2,"dimension_order":[]}`, target: ErrStudyConfigUnsupported},
		{name: "unknown dimension", content: `{"version":1,"dimension_order":["missing"]}`, target: ErrStudyConfigInvalid},
		{name: "duplicate dimension", content: `{"version":1,"dimension_order":["01","structure"]}`, target: ErrStudyConfigInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(StudyConfigPath(study), []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadStudyConfig(study, dimensions); !errors.Is(err, tc.target) {
				t.Fatalf("err = %v, want %v", err, tc.target)
			}
		})
	}
}
