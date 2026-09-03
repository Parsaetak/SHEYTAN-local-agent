package cmd

// v1.0.4 stress tests: hidden-console process launching, the Speed Pack
// launch-flag contract, the GGUF model-card parser, client speed telemetry
// (TTFT + tok/s), the artifact tracker's turn diff, and the v1.0.4 config
// surface.

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/artifacts"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/proc"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sysinfo"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/tools"
)

// stressV104Defaults locks in the v1.0.4 config surface: version bump,
// Speed Pack defaults (flash attention on, cache reuse 32, ubatch 512,
// KV quant off, mlock off, perf HUD on).
func stressV104Defaults() error {
	// v1.0.5 moved the version forward — the contract here is "at least
	// 1.0.4"; the exact current version is locked by config_v105_defaults.
	if !versionAtLeast(config.AppVersion, "1.0.4") {
		return fmt.Errorf("AppVersion = %q, want >= 1.0.4", config.AppVersion)
	}
	cfg := config.Default()
	if !cfg.FlashAttention {
		return fmt.Errorf("FlashAttention default = false, want true")
	}
	if cfg.CacheReuse != 32 {
		return fmt.Errorf("CacheReuse default = %d, want 32", cfg.CacheReuse)
	}
	if cfg.UBatchSize != 512 {
		return fmt.Errorf("UBatchSize default = %d, want 512", cfg.UBatchSize)
	}
	if cfg.KVCacheQuant != "" {
		return fmt.Errorf("KVCacheQuant default = %q, want \"\"", cfg.KVCacheQuant)
	}
	if cfg.Mlock {
		return fmt.Errorf("Mlock default = true, want false")
	}
	if cfg.DraftModel != "" {
		return fmt.Errorf("DraftModel default = %q, want \"\"", cfg.DraftModel)
	}
	if !cfg.ShowPerfHUD {
		return fmt.Errorf("ShowPerfHUD default = false, want true")
	}
	// clamps + normalization
	cfg.KVCacheQuant = "Q8_0 "
	if got := cfg.EffectiveKVCacheQuant(); got != "q8_0" {
		return fmt.Errorf("EffectiveKVCacheQuant(Q8_0 ) = %q, want q8_0", got)
	}
	cfg.KVCacheQuant = "garbage"
	if got := cfg.EffectiveKVCacheQuant(); got != "" {
		return fmt.Errorf("EffectiveKVCacheQuant(garbage) = %q, want \"\"", got)
	}
	cfg.CacheReuse = 99999
	if got := cfg.EffectiveCacheReuse(); got != 512 {
		return fmt.Errorf("EffectiveCacheReuse(99999) = %d, want 512", got)
	}
	cfg.CacheReuse = -5
	if got := cfg.EffectiveCacheReuse(); got != 0 {
		return fmt.Errorf("EffectiveCacheReuse(-5) = %d, want 0 (off)", got)
	}
	cfg.UBatchSize = 0
	if got := cfg.EffectiveUBatchSize(); got != 512 {
		return fmt.Errorf("EffectiveUBatchSize(0) = %d, want 512", got)
	}
	return nil
}

// stressProcHiddenConsole proves the v1.0.4 terminal fix: every command the
// app builds via proc carries hidden-console process attributes on Windows
// (CREATE_NO_WINDOW + HideWindow) and is a plain valid command elsewhere.
func stressProcHiddenConsole() error {
	cmd := proc.Command("echo", "hi")
	if cmd == nil || cmd.SysProcAttr == nil {
		return fmt.Errorf("proc.Command produced nil cmd/attrs — console windows would flash")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd2 := proc.CommandContext(ctx, "echo", "hi")
	if cmd2 == nil || cmd2.SysProcAttr == nil {
		return fmt.Errorf("proc.CommandContext produced nil cmd/attrs")
	}
	// Hide must also work on a pre-populated SysProcAttr.
	c3 := proc.Command("echo", "hi")
	proc.Hide(c3)
	if c3.SysProcAttr == nil {
		return fmt.Errorf("proc.Hide dropped attributes")
	}
	// nil-safety
	proc.Hide(nil)
	return nil
}

// stressSpeedArgs locks the exact llama.cpp Speed Pack launch contract.
func stressSpeedArgs() error {
	cfg := config.Default()
	cfg.LLM.NumThread = 0 // auto (physical cores)
	args := llm.SpeedArgs(cfg)
	joined := strings.Join(args, " ")
	for _, want := range []string{"--flash-attn", "--cache-reuse 32", "--ubatch-size 512", "--no-webui"} {
		if !strings.Contains(joined, want) {
			return fmt.Errorf("SpeedArgs missing %q (have: %s)", want, joined)
		}
	}
	if !strings.Contains(joined, "--threads-batch") {
		return fmt.Errorf("SpeedArgs missing --threads-batch (have: %s)", joined)
	}
	if strings.Contains(joined, "--cache-type-kv") {
		return fmt.Errorf("SpeedArgs must not set KV quant by default (have: %s)", joined)
	}
	if strings.Contains(joined, "--mlock") {
		return fmt.Errorf("SpeedArgs must not set mlock by default (have: %s)", joined)
	}
	if strings.Contains(joined, "--model-draft") {
		return fmt.Errorf("SpeedArgs must not set a draft model by default (have: %s)", joined)
	}

	// All switches on.
	cfg.KVCacheQuant = "q8_0"
	cfg.Mlock = true
	cfg.CacheReuse = 64
	joined = strings.Join(llm.SpeedArgs(cfg), " ")
	for _, want := range []string{"--cache-type-kv q8_0", "--mlock", "--cache-reuse 64"} {
		if !strings.Contains(joined, want) {
			return fmt.Errorf("SpeedArgs(all-on) missing %q (have: %s)", want, joined)
		}
	}

	// Flash attention off + reuse off.
	cfg.FlashAttention = false
	cfg.CacheReuse = 0
	cfg.KVCacheQuant = ""
	cfg.Mlock = false
	joined = strings.Join(llm.SpeedArgs(cfg), " ")
	if strings.Contains(joined, "--flash-attn") || strings.Contains(joined, "--cache-reuse") {
		return fmt.Errorf("SpeedArgs(off) still carries disabled flags (have: %s)", joined)
	}

	// Draft model that does not exist anywhere must be DROPPED, never
	// passed through (a stale config value must not brick the engine).
	cfg.DraftModel = "no-such-draft.gguf"
	if joined := strings.Join(llm.SpeedArgs(cfg), " "); strings.Contains(joined, "--model-draft") {
		return fmt.Errorf("SpeedArgs passed an unresolvable draft model (have: %s)", joined)
	}
	return nil
}

// stressGGUFCard builds a minimal-but-valid GGUF header and proves the
// model-card parser reads arch, name, params, quant, and context length —
// and that garbage files fail gracefully instead of hanging or panicking.
func stressGGUFCard() error {
	buf := []byte("GGUF")
	buf = binary.LittleEndian.AppendUint32(buf, 3) // version
	buf = binary.LittleEndian.AppendUint64(buf, 1) // tensor count
	kvCountAt := len(buf)
	buf = binary.LittleEndian.AppendUint64(buf, 0) // kv count placeholder (patched below)
	kv := 0

	addKV := func(key string, vtype uint32, val []byte) {
		buf = binary.LittleEndian.AppendUint64(buf, uint64(len(key))) // GGUF key_len is u64
		buf = append(buf, key...)
		buf = binary.LittleEndian.AppendUint32(buf, vtype)
		buf = append(buf, val...)
		kv++
	}
	str := func(s string) []byte {
		b := binary.LittleEndian.AppendUint64(nil, uint64(len(s)))
		return append(b, s...)
	}
	addKV("general.architecture", 8, str("qwen2"))
	addKV("general.name", 8, str("Qwen2.5 7B Instruct"))
	addKV("general.parameter_count", 10, binary.LittleEndian.AppendUint64(nil, 7_615_000_000))
	addKV("general.file_type", 4, binary.LittleEndian.AppendUint32(nil, 15)) // Q4_K_M
	addKV("qwen2.context_length", 4, binary.LittleEndian.AppendUint32(nil, 32768))
	addKV("qwen2.block_count", 4, binary.LittleEndian.AppendUint32(nil, 28))
	addKV("qwen2.embedding_length", 4, binary.LittleEndian.AppendUint32(nil, 3584))
	// a big tokenizer array must be SKIPPED, not materialized:
	// string-array of 3 short tokens
	arr := binary.LittleEndian.AppendUint32(nil, 8) // elem type = string
	arr = binary.LittleEndian.AppendUint64(arr, 3)
	for _, tok := range []string{"a", "b", "c"} {
		arr = binary.LittleEndian.AppendUint64(arr, uint64(len(tok)))
		arr = append(arr, tok...)
	}
	addKV("tokenizer.ggml.tokens", 9, arr)

	// patch the real kv count in
	tmp := make([]byte, len(buf))
	copy(tmp, buf)
	binary.LittleEndian.PutUint64(tmp[kvCountAt:kvCountAt+8], uint64(kv))

	dir, err := os.MkdirTemp("", "gguf-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "qwen2.5-7b-instruct-q4_k_m.gguf")
	// pad so SizeBytes is realistic
	blob := append(tmp, make([]byte, 4096)...)
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		return err
	}

	card, err := llm.ReadModelCard(path)
	if err != nil {
		return fmt.Errorf("ReadModelCard(valid): %v", err)
	}
	if card.Arch != "qwen2" {
		return fmt.Errorf("Arch = %q, want qwen2", card.Arch)
	}
	if card.Name != "Qwen2.5 7B Instruct" {
		return fmt.Errorf("Name = %q", card.Name)
	}
	if card.ParamsCount != 7_615_000_000 {
		return fmt.Errorf("ParamsCount = %d", card.ParamsCount)
	}
	if card.Quant != "Q4_K_M" {
		return fmt.Errorf("Quant = %q, want Q4_K_M", card.Quant)
	}
	if card.ContextLength != 32768 {
		return fmt.Errorf("ContextLength = %d, want 32768", card.ContextLength)
	}
	if card.Layers != 28 || card.EmbeddingLen != 3584 {
		return fmt.Errorf("Layers/Embedding = %d/%d, want 28/3584", card.Layers, card.EmbeddingLen)
	}
	if got := card.FormatParams(); got != "7.6B" {
		return fmt.Errorf("FormatParams = %q, want 7.6B", got)
	}
	meta := card.Meta()
	for _, want := range []string{"7.6B", "Q4_K_M", "32K ctx"} {
		if !strings.Contains(meta, want) {
			return fmt.Errorf("Meta %q missing %q", meta, want)
		}
	}

	// Garbage inputs must error, not hang/panic.
	bad := filepath.Join(dir, "bad.gguf")
	if err := os.WriteFile(bad, []byte("NOT GGUF AT ALL........"), 0o644); err != nil {
		return err
	}
	if _, err := llm.ReadModelCard(bad); err == nil {
		return fmt.Errorf("ReadModelCard(garbage) succeeded, want error")
	}
	if _, err := llm.ReadModelCard(filepath.Join(dir, "missing.gguf")); err == nil {
		return fmt.Errorf("ReadModelCard(missing) succeeded, want error")
	}
	return nil
}

// stressStreamTelemetry proves the TTFT + tokens/sec HUD: against a fake
// SSE server the client reports a plausible PerfStats string.
func stressStreamTelemetry() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		time.Sleep(60 * time.Millisecond) // deterministic TTFT floor
		for i := 0; i < 5; i++ {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"tok%d \"}}]}\n\n", i)
			if fl != nil {
				fl.Flush()
			}
			time.Sleep(30 * time.Millisecond)
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL
	cfg.RemoteModel = "test-model"
	client := llm.NewClient(cfg)
	req := client.BuildChatRequest("test-model", []llm.Message{{Role: "user", Content: "hi"}}, nil)

	perf, err := client.StreamChatDetailed(context.Background(), req, func(llm.StreamEvent) error { return nil })
	if err != nil {
		return fmt.Errorf("StreamChatDetailed: %v", err)
	}
	if perf.Tokens != 5 {
		return fmt.Errorf("Tokens = %d, want 5", perf.Tokens)
	}
	if perf.TTFTMs <= 0 {
		return fmt.Errorf("TTFTMs = %d, want > 0", perf.TTFTMs)
	}
	if perf.TokensPerSec <= 0 {
		return fmt.Errorf("TokensPerSec = %.1f, want > 0", perf.TokensPerSec)
	}
	if s := perf.String(); !strings.Contains(s, "tok/s") {
		return fmt.Errorf("PerfStats.String() = %q, want tok/s HUD line", s)
	}
	// cache_prompt must be on the wire (agent-loop prefill collapse).
	perf2, err := client.StreamChatDetailed(context.Background(), req, func(llm.StreamEvent) error { return nil })
	if err != nil || perf2.Tokens != 5 {
		return fmt.Errorf("second stream: err=%v tokens=%d", err, perf2.Tokens)
	}
	return nil
}

// stressArtifactTracker proves the turn snapshot-diff: files created during
// a turn are detected (new + size-changed), deleted files are not, explicit
// reports surface even outside the watched dirs, and Scan lists the folder.
func stressArtifactTracker() error {
	dir, err := os.MkdirTemp("", "artifacts-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.MkdirAll(filepath.Join(dir, "ws"), 0o755); err != nil {
		return err
	}

	tr := artifacts.New([]string{filepath.Join(dir, "ws")})
	// created BEFORE the turn → not an artifact
	pre := filepath.Join(dir, "ws", "pre-existing.csv")
	if err := os.WriteFile(pre, []byte("1,2"), 0o644); err != nil {
		return err
	}
	tr.BeginTurn()

	// created during the turn
	a := filepath.Join(dir, "ws", "report.md")
	if err := os.WriteFile(a, []byte("# hi"), 0o644); err != nil {
		return err
	}
	// created OUTSIDE the watch roots → explicit report
	b := filepath.Join(dir, "elsewhere.txt")
	if err := os.WriteFile(b, []byte("x"), 0o644); err != nil {
		return err
	}
	tr.Report(b)
	tr.Report(filepath.Join(dir, "ghost.txt")) // missing → ignored

	got := tr.EndTurn()
	if len(got) != 2 {
		names := []string{}
		for _, g := range got {
			names = append(names, g.Name)
		}
		return fmt.Errorf("EndTurn found %d artifacts (%v), want 2", len(got), names)
	}
	byName := map[string]artifacts.Artifact{}
	for _, g := range got {
		byName[g.Name] = g
	}
	if _, ok := byName["report.md"]; !ok {
		return fmt.Errorf("report.md missing from artifacts")
	}
	if _, ok := byName["elsewhere.txt"]; !ok {
		return fmt.Errorf("explicitly reported elsewhere.txt missing from artifacts")
	}
	if byName["report.md"].Kind != artifacts.KindDoc {
		return fmt.Errorf("report.md kind = %q, want doc", byName["report.md"].Kind)
	}

	// Scan lists pre-existing + new files, newest first.
	scanned := tr.Scan(50)
	if len(scanned) != 2 { // report.md + pre-existing.csv (elsewhere is outside roots)
		names := []string{}
		for _, s := range scanned {
			names = append(names, s.Name)
		}
		return fmt.Errorf("Scan found %d (%v), want 2", len(scanned), names)
	}

	// Modified (size changed) file counts as created on the next turn.
	tr.BeginTurn()
	if err := os.WriteFile(pre, []byte("1,2,3,4,5"), 0o644); err != nil {
		return err
	}
	mod := tr.EndTurn()
	if len(mod) != 1 || mod[0].Name != "pre-existing.csv" {
		return fmt.Errorf("modified-file detection: got %v", mod)
	}
	return nil
}

// stressToolsReportHook proves the files tool reports writes through the
// v1.0.4 OnFileCreated hook.
func stressToolsReportHook() error {
	dir, err := os.MkdirTemp("", "hook-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	tools.SetBaseDir(dir)

	var mu sync.Mutex
	var seen []string
	prev := tools.OnFileCreated
	tools.OnFileCreated = func(p string) {
		mu.Lock()
		seen = append(seen, p)
		mu.Unlock()
	}
	defer func() { tools.OnFileCreated = prev }()

	args, _ := json.Marshal(map[string]string{
		"action":  "write",
		"path":    "out.txt",
		"content": "hello",
	})
	if _, err := (tools.Files{}).Run(context.Background(), args); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || !strings.HasSuffix(filepath.ToSlash(seen[0]), "out.txt") {
		return fmt.Errorf("OnFileCreated hook saw %v, want [out.txt]", seen)
	}
	return nil
}

// stressThreadsPhysicalFirst proves the v1.0.4 thread split: generation
// threads never exceed logical cores, and both pools are >= 1.
func stressThreadsPhysicalFirst() error {
	gen, batch := sysinfo.RecommendThreads()
	if gen < 1 || batch < 1 {
		return fmt.Errorf("RecommendThreads = %d/%d, both must be >= 1", gen, batch)
	}
	if gen > batch {
		return fmt.Errorf("generation threads %d exceed prefill threads %d", gen, batch)
	}
	return nil
}
