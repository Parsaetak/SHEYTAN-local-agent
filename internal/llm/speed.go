package llm

// speed.go — v1.0.4 Speed Pack: the llama.cpp launch flags that make
// SHEYTAN the fastest way to run local models on Windows.
//
// Research (Aug 2026, see release notes):
//   - Direct llama.cpp beats wrapper apps (LM Studio / Ollama) by 5-20% on
//     identical hardware — we ARE the engine host, zero wrapper overhead.
//   - --flash-attn: fused attention kernels; a straight throughput win on
//     both CPU and the bundled Vulkan backend.
//   - --cache-reuse N: KV-shift prompt-cache reuse. Agent turns share a
//     long stable prefix (AI context + tool schemas), so after the first
//     turn the repeated prefill collapses to near zero — dramatically
//     lower time-to-first-token on every follow-up message.
//   - --ubatch-size 512: prompt-processing physical batch; the measured
//     sweet spot on x86-64 multicore.
//   - --threads = PHYSICAL cores for generation + --threads-batch =
//     LOGICAL cores for prefill: SMT siblings cost 5-15% tok/s when two
//     generation threads share one physical core, but prefill parallelizes
//     fine across all logical cores.
//   - --cache-type-kv q8_0 (optional): halves KV-cache memory at <5%
//     speed cost on modern GPUs — the one way to fit big contexts in
//     limited VRAM. Off by default: a minority of Vulkan drivers regress.
//   - --model-draft (optional): speculative decoding with a small draft
//     model — 20-50% more tokens/sec for same-family model pairs.
//   - --mlock (optional): pin weights in RAM so nothing evicts them
//     mid-chat.
//   - --no-webui: the engine's built-in web UI is dead weight; SHEYTAN is
//     the interface.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/sysinfo"
)

// threadsFor resolves the generation thread count: explicit config value
// wins; otherwise the detected PHYSICAL core count (falls back to logical).
func threadsFor(cfg *config.Config) int {
	if cfg.LLM.NumThread > 0 {
		return cfg.LLM.NumThread
	}
	gen, _ := sysinfo.RecommendThreads()
	return gen
}

// threadsBatchFor resolves the prefill thread count (logical cores by
// default).
func threadsBatchFor(cfg *config.Config) int {
	if cfg.ThreadsBatch > 0 {
		return cfg.ThreadsBatch
	}
	_, batch := sysinfo.RecommendThreads()
	return batch
}

// SpeedArgs builds the v1.0.4 speed flag set for the llama.cpp server.
// Exported so the stress suite can lock the exact launch contract in.
func SpeedArgs(cfg *config.Config) []string {
	var args []string

	// Fused attention kernels (default ON).
	if cfg.FlashAttention {
		args = append(args, "--flash-attn")
	}

	// Prompt-cache reuse for agent loops (stable prefix across turns).
	if n := cfg.EffectiveCacheReuse(); n > 0 {
		args = append(args, "--cache-reuse", fmt.Sprintf("%d", n))
	}

	// Physical prompt-processing batch.
	args = append(args, "--ubatch-size", fmt.Sprintf("%d", cfg.EffectiveUBatchSize()))

	// Separate prefill thread pool (generation threads come from
	// threadsFor() in llama.go's base args).
	args = append(args, "--threads-batch", fmt.Sprintf("%d", threadsBatchFor(cfg)))

	// KV-cache compression (opt-in).
	if q := cfg.EffectiveKVCacheQuant(); q != "" {
		args = append(args, "--cache-type-kv", q)
	}

	// Pin weights in RAM (opt-in).
	if cfg.Mlock {
		args = append(args, "--mlock")
	}

	// Speculative decoding with a draft model (opt-in). Resolved against
	// the models dir so a bare filename works like the main model.
	if dm := strings.TrimSpace(cfg.DraftModel); dm != "" {
		if path, err := ResolveModelPath(cfg.ModelsDir, dm); err == nil {
			args = append(args, "--model-draft", path, "--draft-max", "16")
		} else if abs := strings.TrimSpace(dm); isExistingFile(abs) {
			args = append(args, "--model-draft", abs, "--draft-max", "16")
		}
		// Unresolvable draft names are silently dropped — a stale config
		// value must never brick the engine.
	}

	// SHEYTAN is the interface; the engine's web UI is dead weight.
	args = append(args, "--no-webui")

	return args
}

func isExistingFile(p string) bool {
	if p == "" {
		return false
	}
	if !filepath.IsAbs(p) {
		if wd, err := os.Getwd(); err == nil {
			p = filepath.Join(wd, p)
		}
	}
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
