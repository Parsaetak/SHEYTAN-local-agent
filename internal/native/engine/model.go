package engine

// model.go — native engine model concern (v1.1.5Z Phase 1: TYPES ONLY).
//
// The future native engine will load GGUF-class models into its own
// runtime. Phase 1 defines the data model (descriptor + lifecycle state +
// validation) and wires the validation into Backend.LoadModel; NO model
// loading exists in the native engine yet — LoadModel returns
// llm.ErrNotImplemented after validating the spec.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ModelSpec describes a model the native engine should serve.
type ModelSpec struct {
	// Path is the model file (GGUF-class) path.
	Path string `json:"path"`

	// SizeBytes is the file size (measured at validation time).
	SizeBytes int64 `json:"sizeBytes,omitempty"`

	// Quantization, Architecture and ContextLength are model facts the
	// future loader will parse from the file header. Phase 1 leaves them
	// empty — they are not guessed.
	Quantization  string `json:"quantization,omitempty"`
	Architecture  string `json:"architecture,omitempty"`
	ContextLength int    `json:"contextLength,omitempty"`
}

// ModelState is the lifecycle state of a model inside the native engine.
type ModelState struct {
	// Loaded reports whether a model is resident.
	Loaded bool `json:"loaded"`

	// Path of the resident model (empty when none).
	Path string `json:"path,omitempty"`

	// State is one of: "" (nothing attempted), "loading", "loaded",
	// "failed".
	State string `json:"state,omitempty"`

	// Detail carries the failure reason or load notes.
	Detail string `json:"detail,omitempty"`
}

// ValidateModelSpec checks a model spec against the filesystem WITHOUT
// loading anything: the path must be absolute-or-resolvable, must exist,
// must be a regular file, and must sit inside the given models root (path
// jail discipline — the same class of rule every file tool enforces).
func ValidateModelSpec(modelsRoot, path string) (ModelSpec, error) {
	spec := ModelSpec{Path: path}

	if strings.TrimSpace(path) == "" {
		return spec, fmt.Errorf("model path is empty")
	}

	root, err := filepath.Abs(modelsRoot)
	if err != nil {
		return spec, fmt.Errorf("resolve models root: %w", err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return spec, fmt.Errorf("resolve model path: %w", err)
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return spec, fmt.Errorf("validate model path: %w", err)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return spec, fmt.Errorf("model path %q escapes the models directory", path)
	}

	fi, err := os.Stat(abs)
	if err != nil {
		return spec, fmt.Errorf("model file: %w", err)
	}

	if fi.IsDir() {
		return spec, fmt.Errorf("model path %q is a directory", path)
	}

	spec.SizeBytes = fi.Size()

	return spec, nil
}
