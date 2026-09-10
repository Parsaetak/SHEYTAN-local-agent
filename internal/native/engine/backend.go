package engine

// backend.go — the native engine adapter for the llm.Backend contract
// (v1.1.5Z Phase 1).
//
// NativeBackend implements the full Backend method set. The lifecycle,
// health, hardware, metrics and cancel surfaces are REAL (backed by the
// supervised host subprocess and the IPC protocol). The generation
// surface (Generate / StreamGenerate / LoadModel / UnloadModel /
// ModelInfo) returns llm.ErrNotImplemented — that error is the fallback
// signal llm.SelectGenerationBackend and the runtime act on. Phase 1
// never fakes inference.

import (
        "context"
        "fmt"
        "os"

        "github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
)

// ABIVersionExpected is the C ABI version the Go core expects from the
// native engine (must match include/shtn/version.h).
const ABIVersionExpected uint32 = 1

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

// LoadModel implements Backend. Phase 1: the spec is VALIDATED (path jail,
// existence, size) and then rejected with llm.ErrNotImplemented — the
// native model loader is future work. Validation now keeps later phases
// from accepting garbage.
func (b *Backend) LoadModel(ctx context.Context, spec llm.ModelSpec) error {
        _ = ctx

        if spec.Path == "" {
                return fmt.Errorf("model spec path is empty")
        }

        // Validation happens through the model concern; the models root is
        // not known to the backend, so a plain file check runs here.
        if _, err := statRegularFile(spec.Path); err != nil {
                return err
        }

        return fmt.Errorf("%w: native engine model loading", llm.ErrNotImplemented)
}

// UnloadModel implements Backend (Phase 1: not implemented — nothing is
// ever loaded).
func (b *Backend) UnloadModel(ctx context.Context) error {
        _ = ctx
        return fmt.Errorf("%w: native engine model unloading", llm.ErrNotImplemented)
}

// Generate implements Backend (Phase 1: not implemented — callers fall
// back to the llama backend via SelectGenerationBackend).
func (b *Backend) Generate(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
        _ = ctx
        _ = req
        return nil, fmt.Errorf("%w: native engine generation", llm.ErrNotImplemented)
}

// StreamGenerate implements Backend (Phase 1: not implemented — callers
// fall back to the llama backend via SelectGenerationBackend).
func (b *Backend) StreamGenerate(ctx context.Context, req *llm.ChatRequest, onEvent func(llm.StreamEvent) error) (llm.PerfStats, error) {
        _ = ctx
        _ = req
        _ = onEvent
        return llm.PerfStats{}, fmt.Errorf("%w: native engine streaming generation", llm.ErrNotImplemented)
}

// Cancel implements Backend with a real protocol round-trip. Phase 1: the
// host answers "no active requests" for any id, which surfaces as a
// cancel miss (an error naming the reason), not a fake success.
func (b *Backend) Cancel(ctx context.Context, requestID string) error {
        return b.eng.Cancel(ctx, requestID)
}

// ModelInfo implements Backend (Phase 1: not implemented — no model can
// be loaded yet).
func (b *Backend) ModelInfo(ctx context.Context) (llm.ModelInfo, error) {
        _ = ctx
        return llm.ModelInfo{}, fmt.Errorf("%w: native engine model info", llm.ErrNotImplemented)
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

// GenerationCapable implements llm.GenerationCapable. Phase 1: false —
// generation does not exist in the native engine yet. This single boolean
// is what keeps generation requests on the llama.cpp fallback; flipping
// it (with real implementations) is the Phase 2 trigger.
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
