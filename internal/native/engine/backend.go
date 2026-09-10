package engine

// backend.go — the native engine adapter for the llm.Backend contract
// (v1.1.5Z Phase 2).
//
// NativeBackend implements the full Backend method set. The lifecycle,
// health, hardware, metrics, cancel and MODEL surfaces (LoadModel /
// UnloadModel / ModelInfo — native GGUF loading with metadata + memory
// plan) are REAL, backed by the supervised host subprocess and the IPC
// protocol. The generation surface (Generate / StreamGenerate) still
// returns llm.ErrNotImplemented — that error is the fallback signal
// llm.SelectGenerationBackend and the runtime act on. Phase 2 never fakes
// inference, and GenerationCapable stays false until generation exists.

import (
	"context"
	"fmt"
	"os"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
)

// ABIVersionExpected is the C ABI version the Go core expects from the
// native engine (must match include/shtn/version.h).
const ABIVersionExpected uint32 = 3

// Backend adapts an *Engine to the llm.Backend contract.
type Backend struct {
	eng *Engine
}

// compile-time contract check.
var _ llm.Backend = (*Backend)(nil)

// NewBackend wraps an Engine.
func NewBackend(eng *Engine) *Backend { return &Backend{eng: eng} }

// Engine exposes the supervised engine.
func (b *Backend) Engine() *Engine { return b.eng }

// Name implements Backend.
func (b *Backend) Name() string { return "native" }

// Start implements Backend: spawn + handshake + health + ready.
func (b *Backend) Start(ctx context.Context) error { return b.eng.Start(ctx) }

// Stop implements Backend: graceful ask, bounded grace, kill.
func (b *Backend) Stop(ctx context.Context) error { return b.eng.Stop(ctx) }

// Health implements Backend with a real IPC round-trip.
func (b *Backend) Health(ctx context.Context) (llm.HealthReport, error) {
	return b.eng.Health(ctx)
}

// LoadModel implements Backend: the engine validates the file, loads it
// natively (GGUF validate → memory-map → metadata → memory plan) and
// reports real model state. Requires a running engine.
func (b *Backend) LoadModel(ctx context.Context, spec llm.ModelSpec) error {
	if spec.Path == "" {
		return fmt.Errorf("model spec path is empty")
	}

	// Plain file validation (the backend does not know the models
	// root; a path jail can be applied by callers through
	// engine.ValidateModelSpec).
	if _, err := statRegularFile(spec.Path); err != nil {
		return err
	}

	return b.eng.LoadModel(ctx, ModelSpec{Path: spec.Path})
}

// UnloadModel implements Backend: releases the natively loaded model
// (mapping, handles, cached metadata). Idempotent.
func (b *Backend) UnloadModel(ctx context.Context) error {
	return b.eng.UnloadModel(ctx)
}

// Generate implements Backend (NOT implemented — generation is a later
// phase; callers fall back to the llama backend via
// SelectGenerationBackend).
func (b *Backend) Generate(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	_ = ctx
	_ = req
	return nil, fmt.Errorf("%w: native engine generation", llm.ErrNotImplemented)
}

// StreamGenerate implements Backend (NOT implemented — see Generate).
func (b *Backend) StreamGenerate(ctx context.Context, req *llm.ChatRequest, onEvent func(llm.StreamEvent) error) (llm.PerfStats, error) {
	_ = ctx
	_ = req
	_ = onEvent
	return llm.PerfStats{}, fmt.Errorf("%w: native engine streaming generation", llm.ErrNotImplemented)
}

// Cancel implements Backend with a real protocol round-trip. In this
// phase the host answers "no active requests" for any id, which surfaces
// as a cancel miss (an error naming the reason), not a fake success.
func (b *Backend) Cancel(ctx context.Context, requestID string) error {
	return b.eng.Cancel(ctx, requestID)
}

// ModelInfo implements Backend: a real round-trip to the engine's model
// concern, mapped onto the shared llm.ModelInfo contract. Additive
// native fields (vocab size, layers, tensor count, memory plan summary)
// are carried in the shared struct; the llama path leaves them zero.
func (b *Backend) ModelInfo(ctx context.Context) (llm.ModelInfo, error) {
	info := llm.ModelInfo{Backend: b.Name()}

	if b.eng == nil || !b.eng.IsAlive() {
		// Engine down: nothing can be resident.
		info.Loaded = false
		return info, nil
	}

	result, err := b.eng.ModelInfo(ctx)
	if err != nil {
		return info, err
	}

	info.Loaded = result.Loaded
	info.State = result.State

	if result.Model != nil {
		m := result.Model

		info.ModelPath = m.Path
		info.Architecture = m.Architecture
		info.Quantization = m.Quantization
		info.ContextLength = int(m.ContextLength)
		info.Parameters = llm.FormatParameterCount(m.ParameterCount)
		info.FileSizeBytes = m.FileSizeBytes
		info.TensorCount = int(m.TensorCount)
		info.VocabSize = int(m.VocabularySize)
		info.EmbeddingLength = int(m.EmbeddingLength)
		info.LayerCount = int(m.LayerCount)
		info.GGUFVersion = int(m.GGUFVersion)

		if result.Memory != nil {
			info.KVCacheEstimateBytes = result.Memory.KVCacheBytes
			info.WorkspaceEstimateBytes = result.Memory.WorkspaceBytes
			info.TotalMemoryEstimateBytes = result.Memory.TotalBytes
		}
	}

	return info, nil
}

// HardwareInfo implements Backend: the native probe merged with sysinfo.
func (b *Backend) HardwareInfo(ctx context.Context) (llm.HardwareInfo, error) {
	return b.eng.Hardware(ctx)
}

// Metrics implements Backend: measured lifecycle facts + the engine's own
// metrics reading.
func (b *Backend) Metrics(ctx context.Context) (llm.Metrics, error) {
	return b.eng.MetricsSnapshot(ctx)
}

// GenerationCapable implements llm.GenerationCapable. Phase 2: still
// false — model LOADING exists, generation does not. This single boolean
// keeps generation requests on the llama.cpp fallback; flipping it (with
// a real generation implementation) is the Phase 3+ trigger.
func (b *Backend) GenerationCapable() bool { return false }

// statRegularFile validates that path exists and is a regular file
// (no directory, no dangling symlink).
func statRegularFile(path string) (os.FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("model file: %w", err)
	}

	if fi.IsDir() {
		return nil, fmt.Errorf("model path %q is a directory", path)
	}

	return fi, nil
}
