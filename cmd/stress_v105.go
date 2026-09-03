package cmd

// v1.0.5 stress tests: the engine compatibility ladder (the gemma exit-1
// fix), stderr-tail surfacing in launch failures, the self-update signal
// for unknown architectures, version bump, and the case-insensitive model
// listing.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
)

// stressV105Defaults locks in the v1.0.5 config surface: version bump and
// the EngineCompat ladder field. (Version assertions are forward-compatible:
// later releases keep passing.)
func stressV105Defaults() error {
	if !versionAtLeast(config.AppVersion, "1.0.5") {
		return fmt.Errorf("AppVersion = %q, want >= 1.0.5", config.AppVersion)
	}
	cfg := config.Default()
	if cfg.EngineCompat != 0 {
		return fmt.Errorf("EngineCompat default = %d, want 0 (full speed)", cfg.EngineCompat)
	}
	return nil
}

// stressCompatLadderArgs locks the per-level launch-flag contract of
// buildArgs:
//
//	0 = speed pack + gpu + extras
//	1 = speed pack + gpu + extras + --jinja
//	2 = gpu + extras + --jinja, NO speed flags
//	3 = bare: no jinja, no gpu, no speed, no extras
func stressCompatLadderArgs() error {
	cfg := config.Default()
	cfg.DataDir = os.TempDir()
	cfg.LlamaBinPath = filepath.Join(cfg.DataDir, "bin", "llama-server.exe")
	cfg.LlamaExtraArgs = "--my-custom-flag"
	// Neutralize environment-dependent GPU auto-offload so the contract is
	// deterministic: explicit layers only.
	cfg.GPUAutoOffload = false
	cfg.LLM.NumGPU = 42
	cfg.FlashAttention = true
	cfg.CacheReuse = 32
	cfg.UBatchSize = 512
	cfg.ThreadsBatch = 8
	cfg.DraftModel = ""

	srv := llm.NewLlamaServer(cfg)
	model := filepath.Join(os.TempDir(), "model.gguf")

	l0 := strings.Join(srv.BuildArgsForTest(model, 0), " ")
	for _, want := range []string{"--model", "--n-gpu-layers 42", "--flash-attn", "--cache-reuse 32", "--no-webui", "--my-custom-flag"} {
		if !strings.Contains(l0, want) {
			return fmt.Errorf("level 0 missing %q (have: %s)", want, l0)
		}
	}
	if strings.Contains(l0, "--jinja") {
		return fmt.Errorf("level 0 must not carry --jinja (have: %s)", l0)
	}

	l1 := strings.Join(srv.BuildArgsForTest(model, 1), " ")
	if !strings.Contains(l1, "--jinja") || !strings.Contains(l1, "--flash-attn") || !strings.Contains(l1, "--n-gpu-layers 42") {
		return fmt.Errorf("level 1 must add --jinja and keep speed+gpu (have: %s)", l1)
	}

	l2 := strings.Join(srv.BuildArgsForTest(model, 2), " ")
	if !strings.Contains(l2, "--jinja") || !strings.Contains(l2, "--n-gpu-layers 42") || !strings.Contains(l2, "--my-custom-flag") {
		return fmt.Errorf("level 2 must keep jinja+gpu+extras (have: %s)", l2)
	}
	for _, banned := range []string{"--flash-attn", "--cache-reuse", "--ubatch-size", "--threads-batch", "--cache-type-kv"} {
		if strings.Contains(l2, banned) {
			return fmt.Errorf("level 2 must drop speed flag %q (have: %s)", banned, l2)
		}
	}

	l3 := strings.Join(srv.BuildArgsForTest(model, 3), " ")
	for _, banned := range []string{"--jinja", "--n-gpu-layers", "--flash-attn", "--no-webui", "--my-custom-flag"} {
		if strings.Contains(l3, banned) {
			return fmt.Errorf("level 3 (safe mode) must not carry %q (have: %s)", banned, l3)
		}
	}
	for _, want := range []string{"--model", "--host", "--port", "--ctx-size", "--threads"} {
		if !strings.Contains(l3, want) {
			return fmt.Errorf("level 3 missing bare flag %q (have: %s)", want, l3)
		}
	}
	return nil
}

// stressExitFailureTail locks the "real reason in the error" contract: an
// exitFailure message includes the engine's last stderr lines.
func stressExitFailureTail() error {
	err := llm.MakeExitFailureForTest(
		errors.New("exit status 1"),
		[]string{"llama_model_load: unknown model architecture: gemma4n", "load: error loading model"},
	)
	msg := err.Error()
	if !strings.Contains(msg, "exit status 1") {
		return fmt.Errorf("exit error text missing: %v", err)
	}
	if !strings.Contains(msg, "unknown model architecture") {
		return fmt.Errorf("stderr tail missing from error: %v", err)
	}
	if !llm.IsExitFailureForTest(err) {
		return fmt.Errorf("error must be an exitFailure (retry-worthy)")
	}
	// A plain timeout-style error must NOT be classified retry-worthy.
	if llm.IsExitFailureForTest(errors.New("did not become ready")) {
		return fmt.Errorf("plain error misclassified as exitFailure")
	}
	return nil
}

// stressNeedsNewerEngine locks the architecture-mismatch signals that
// trigger the v1.0.5 auto engine update.
func stressNeedsNewerEngine() error {
	if !llm.NeedsNewerEngineForTest(errors.New("llama_model_load: unknown model architecture: gemma4n")) {
		return fmt.Errorf("unknown model architecture must request an engine update")
	}
	if !llm.NeedsNewerEngineForTest(errors.New("error: unsupported architecture 'foo'")) {
		return fmt.Errorf("unsupported architecture must request an engine update")
	}
	if llm.NeedsNewerEngineForTest(errors.New("out of memory")) {
		return fmt.Errorf("OOM must NOT trigger an engine update")
	}
	if llm.NeedsNewerEngineForTest(errors.New("exit status 1")) {
		return fmt.Errorf("bare exit status must NOT trigger an engine update")
	}
	return nil
}

// stressModelListingCase locks the case-insensitive .gguf listing.
func stressModelListingCase() error {
	dir, err := os.MkdirTemp("", "sheytan-models-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for _, name := range []string{"m1.gguf", "M2.GGUF", "notes.txt", "M3.Gguf"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			return err
		}
	}
	got := llm.ListLocalModels(dir)
	if len(got) != 3 {
		return fmt.Errorf("ListLocalModels = %v, want 3 (case-insensitive .gguf)", got)
	}
	return nil
}

// stressCompactLines locks the tail-clipping helper used in launch errors.
func stressCompactLines() error {
	lines := []string{"a", "", "b"}
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("line-%d", i))
	}
	out := llm.CompactLinesForTest(lines, 5)
	if strings.Contains(out, "line-0") {
		return fmt.Errorf("compactLines must keep only the LAST lines: %q", out)
	}
	if !strings.Contains(out, "line-29") {
		return fmt.Errorf("compactLines lost the newest line: %q", out)
	}
	if strings.Contains(out, "\n\n") {
		return fmt.Errorf("compactLines must drop empty lines: %q", out)
	}
	return nil
}
