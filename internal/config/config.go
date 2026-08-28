// Package config holds runtime configuration for SHEYTAN-Local-Agent.
// All fields can be overridden via a YAML config file, environment variables,
// or the in-UI settings panel.
package config

import (
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "runtime"
        "strconv"
        "strings"
        "time"
)

const (
        AppName    = "SHEYTAN-Local-Agent"
        AppVersion = "1.0.9"
)

// Provider kinds: "local" (bundled llama.cpp) or "remote" (any
// OpenAI-compatible endpoint — z.ai, OpenAI, OpenRouter, vLLM, ...).
const (
        ProviderLocal  = "local"
        ProviderRemote = "remote"
)

// LLMOptions mirrors the full set of LM Studio-style sampling/runtime knobs.
// Anything not set falls back to llama.cpp server defaults.
type LLMOptions struct {
        Temperature      float64 `json:"temperature" yaml:"temperature"`
        TopP             float64 `json:"top_p" yaml:"top_p"`
        TopK             int     `json:"top_k" yaml:"top_k"`
        MinP             float64 `json:"min_p" yaml:"min_p"`
        MaxTokens        int     `json:"max_tokens" yaml:"max_tokens"`
        Stop             string  `json:"stop" yaml:"stop"`
        Seed             int     `json:"seed" yaml:"seed"`
        RepeatPenalty    float64 `json:"repeat_penalty" yaml:"repeat_penalty"`
        RepeatLastN      int     `json:"repeat_last_n" yaml:"repeat_last_n"`
        PresencePenalty  float64 `json:"presence_penalty" yaml:"presence_penalty"`
        FrequencyPenalty float64 `json:"frequency_penalty" yaml:"frequency_penalty"`
        Mirostat         int     `json:"mirostat" yaml:"mirostat"`
        MirostatTau      float64 `json:"mirostat_tau" yaml:"mirostat_tau"`
        MirostatEta      float64 `json:"mirostat_eta" yaml:"mirostat_eta"`
        NumCtx           int     `json:"num_ctx" yaml:"num_ctx"`
        NumBatch         int     `json:"num_batch" yaml:"num_batch"`
        NumGPU           int     `json:"num_gpu" yaml:"num_gpu"`
        NumThread        int     `json:"num_thread" yaml:"num_thread"`
        Stream           bool    `json:"stream" yaml:"stream"`
        Preset           string  `json:"preset" yaml:"preset"`
}

// Config is the top-level runtime configuration.
type Config struct {
        // Where data lives
        DataDir     string `json:"dataDir" yaml:"dataDir"`
        ModelsDir   string `json:"modelsDir" yaml:"modelsDir"`
        SessionsDir string `json:"sessionsDir" yaml:"sessionsDir"`

        // HTTP server
        Host string `json:"host" yaml:"host"`
        Port int    `json:"port" yaml:"port"`

        // LLM endpoint (llama.cpp server)
        LLMBaseURL string `json:"llmBaseUrl" yaml:"llmBaseUrl"`
        Model      string `json:"model" yaml:"model"`

        // Remote provider (OpenAI-compatible API). When Provider ==
        // "remote" the agent skips llama.cpp entirely and talks to
        // RemoteBaseURL with RemoteAPIKey and RemoteModel.
        Provider      string `json:"provider" yaml:"provider"` // "local" | "remote"
        RemoteBaseURL string `json:"remoteBaseUrl" yaml:"remoteBaseUrl"`
        RemoteAPIKey  string `json:"remoteApiKey" yaml:"remoteApiKey"`
        RemoteModel   string `json:"remoteModel" yaml:"remoteModel"`

        // llama.cpp server subprocess
        LlamaBinPath   string `json:"llamaBinPath" yaml:"llamaBinPath"`
        LlamaHost      string `json:"llamaHost" yaml:"llamaHost"`
        LlamaPort      int    `json:"llamaPort" yaml:"llamaPort"`
        LlamaAutoStart bool   `json:"llamaAutoStart" yaml:"llamaAutoStart"`
        LlamaExtraArgs string `json:"llamaExtraArgs" yaml:"llamaExtraArgs"`

        // Agent loop
        MaxIterations int  `json:"maxIterations" yaml:"maxIterations"`
        ParallelTools bool `json:"parallelTools" yaml:"parallelTools"`
        VerboseAgent  bool `json:"verboseAgent" yaml:"verboseAgent"`

        // Sandbox
        SandboxEnabled bool   `json:"sandboxEnabled" yaml:"sandboxEnabled"`
        SandboxMemory  string `json:"sandboxMemory" yaml:"sandboxMemory"`
        SandboxCPU     int    `json:"sandboxCPU" yaml:"sandboxCPU"`

        // Sampling
        LLM LLMOptions `json:"llm" yaml:"llm"`

        // Browser tool
        BrowserExecutablePath string `json:"browserExecutablePath" yaml:"browserExecutablePath"`
        BrowserHeadless       bool   `json:"browserHeadless" yaml:"browserHeadless"`
        BrowserSlowMo         int    `json:"browserSlowMoMs" yaml:"browserSlowMoMs"` // extra human-like delay

        // v1.0.0 — experience & maintenance
        // ProMode: when true the UI shows the professional dock (context/params/
        // system/tools tabs, live logs, activity detail); when false it stays
        // minimal with background processing only.
        ProMode bool `json:"proMode" yaml:"proMode"`
        // UpdateSchedule: engine update cadence — "daily" | "weekly" |
        // "monthly" | "off" (default daily).
        UpdateSchedule string `json:"updateSchedule" yaml:"updateSchedule"`
        // LastUpdateCheck: RFC3339 timestamp of the last scheduled engine check.
        LastUpdateCheck string `json:"lastUpdateCheck" yaml:"lastUpdateCheck"`

        // v1.0.2 — attachments, thinking, tools, recall, chunking

        // ThinkingMode: when true the agent reasons step-by-step (<think> tags
        // or native reasoning_content) before answering; the reasoning stream
        // is surfaced separately from the final answer.
        ThinkingMode bool `json:"thinkingMode" yaml:"thinkingMode"`
        // EnabledTools: allow-list of tool names the agent may call. Empty or
        // nil = every registered tool is enabled.
        EnabledTools []string `json:"enabledTools" yaml:"enabledTools"`
        // AttachmentsBudgetKB: total inline byte budget for files attached to
        // one message (default 256; text beyond the budget is head/tail
        // windowed, binaries become metadata notes).
        AttachmentsBudgetKB int `json:"attachmentsBudgetKb" yaml:"attachmentsBudgetKb"`
        // HistoryWindowPct: share of num_ctx (0-100) reserved for conversation
        // history before older messages are compacted (default 60).
        HistoryWindowPct int `json:"historyWindowPct" yaml:"historyWindowPct"`
        // RecallEnabled: auto-recall relevant facts from past conversations
        // into the prompt (default true).
        RecallEnabled bool `json:"recallEnabled" yaml:"recallEnabled"`
        // RecallTopK: how many past-exchange digests to inject max (default 4).
        RecallTopK int `json:"recallTopK" yaml:"recallTopK"`

        // v1.0.3 — engine & experience

        // GPUAutoOffload: when true (default) and the bundled engine ships a
        // Vulkan backend while a GPU is detected, llama.cpp is launched with
        // --n-gpu-layers 99 even though LLM.NumGPU is 0 — the engine uses the
        // GPU when it can and silently stays on CPU when it cannot. Turn off to
        // force pure-CPU inference.
        GPUAutoOffload bool `json:"gpuAutoOffload" yaml:"gpuAutoOffload"`

        // v1.0.5 — engine compatibility ladder.

        // EngineCompat records the llama.cpp launch profile the engine last
        // started successfully with, so the fix that made a difficult model
        // (e.g. a brand-new gemma release whose flags/arch the bundled build
        // partially rejects) is remembered and the next boot skips the
        // levels that already failed:
        //   0 = full speed pack (default)
        //   1 = + --jinja (chat-template compat)
        //   2 = --jinja, no speed flags (attention/cache-flag compat)
        //   3 = bare minimum, CPU-only (safe mode)
        // Reset to 0 automatically whenever the engine binary is updated, so
        // a newer build gets a fresh chance at the fastest path.
        EngineCompat int `json:"engineCompat" yaml:"engineCompat"`

        // v1.0.4 — Speed Pack (the "fastest local AI experience" cycle).
        // Research-backed llama.cpp tuning (Aug 2026): flash attention,
        // prefix cache reuse for agent loops, separate generation/prefill
        // thread pools, optional KV-cache compression, optional draft model
        // for speculative decoding.

        // FlashAttention: launch the engine with --flash-attn (default ON —
        // a straight throughput win on every backend the bundled build
        // ships).
        FlashAttention bool `json:"flashAttention" yaml:"flashAttention"`
        // CacheReuse: minimum chunk size for llama.cpp KV-shift prompt-cache
        // reuse (--cache-reuse). Agent turns share a long stable prefix (AI
        // context + tools), so reuse collapses repeated prefill to near zero
        // after the first turn. 0 disables; default 32.
        CacheReuse int `json:"cacheReuse" yaml:"cacheReuse"`
        // UBatchSize: physical batch for prompt processing (--ubatch-size).
        // Larger batches raise prefill throughput on multicore CPUs;
        // default 512 (llama.cpp's own sweet spot on x86-64).
        UBatchSize int `json:"ubatchSize" yaml:"ubatchSize"`
        // ThreadsBatch: threads for prompt processing (--threads-batch).
        // 0 = logical cores (prefill parallelizes well across SMT).
        ThreadsBatch int `json:"threadsBatch" yaml:"threadsBatch"`
        // KVCacheQuant: quantize the KV cache (--cache-type-kv). "", "q8_0"
        // (halves KV memory, <5% speed cost on modern GPUs), or "q4_0"
        // (quarter memory, quality risk). Off by default — a minority of
        // Vulkan drivers regress on quantized KV.
        KVCacheQuant string `json:"kvCacheQuant" yaml:"kvCacheQuant"`
        // Mlock: pin model pages in RAM (--mlock) so a background app can
        // never evict the weights mid-chat. Default off (needs free RAM
        // beyond the model size).
        Mlock bool `json:"mlock" yaml:"mlock"`
        // DraftModel: optional small GGUF for speculative decoding
        // (--model-draft). A 0.5–1B draft of the same family typically adds
        // 20–50% tokens/sec. Empty disables.
        DraftModel string `json:"draftModel" yaml:"draftModel"`
        // ShowPerfHUD: display live tokens/sec + time-to-first-token in the
        // status line after each turn (default ON in Pro mode).
        ShowPerfHUD bool `json:"showPerfHud" yaml:"showPerfHud"`

        // v1.0.6 — Vision (multimodal) + resources + feedback.

        // VisionEnabled: when true (default) the app looks for a multimodal
        // projector (mmproj-*.gguf) that pairs with the selected chat model
        // and launches llama.cpp with --mmproj so images and screenshots can
        // be understood. Turn off to force text-only.
        VisionEnabled bool `json:"visionEnabled" yaml:"visionEnabled"`
        // VisionMMProj: explicit projector file (name or absolute path).
        // Empty = auto-detect by pairing with the selected model.
        VisionMMProj string `json:"visionMmproj" yaml:"visionMmproj"`

        // MaxWorkspaceMB: soft quota for the agent workspace folder. 0 =
        // unlimited. Surfaced in the Resources view with a usage bar and a
        // one-click cleanup.
        MaxWorkspaceMB int `json:"maxWorkspaceMb" yaml:"maxWorkspaceMb"`
        // MaxSessionsKept: when the Resources cleanup runs, sessions beyond
        // the newest N are archived away. 0 = keep everything.
        MaxSessionsKept int `json:"maxSessionsKept" yaml:"maxSessionsKept"`
        // MaxLogMB: target size for the log folder — the cleanup truncates
        // the biggest log files down to it. 0 = no limit.
        MaxLogMB int `json:"maxLogMb" yaml:"maxLogMb"`

        // MultiAgentDepth: planning depth of the multi-agent pipeline
        // (plan → execute → critique loops). 1-5, default 3. Each extra
        // depth costs one more critic pass — the Resources view exposes it
        // as the per-agent processing budget.
        MultiAgentDepth int `json:"multiAgentDepth" yaml:"multiAgentDepth"`

        // v1.0.7 — Continuum context engine ("almost unlimited context").

        // ContinuumEnabled: when true (default) the app watches context
        // usage of the active session and, at the end of a completed turn
        // near the limit, transparently rolls the conversation into a NEW
        // chapter session seeded with a distilled state Framework — the
        // user experiences one unbroken conversation while each chapter
        // stays small enough to prefill fast.
        ContinuumEnabled bool `json:"continuumEnabled" yaml:"continuumEnabled"`
        // ContinuumThresholdPct: estimated history usage (0-100) of the
        // history token budget at which a chapter rollover is triggered
        // after the turn ends. Default 75; clamped 50-95.
        ContinuumThresholdPct int `json:"continuumThresholdPct" yaml:"continuumThresholdPct"`
        // ContinuumCarryMessages: how many of the most recent messages are
        // carried VERBATIM into the next chapter (the older ones live on in
        // the Framework + recall). Default 4; clamped 0-16.
        ContinuumCarryMessages int `json:"continuumCarryMessages" yaml:"continuumCarryMessages"`
        // ContinuumFrameworkTokens: token budget for the rendered Framework
        // briefing injected at the top of every new chapter. Default 700;
        // clamped 200-2000.
        ContinuumFrameworkTokens int `json:"continuumFrameworkTokens" yaml:"continuumFrameworkTokens"`

        // v1.0.9 — TURBINE streaming & frame pacing.

        // SmoothStream: when true (default) streaming tokens are rendered
        // through the frame-paced pump — UI mutations coalesce to at most
        // one batch per display frame (TargetFPS), so text streams at the
        // monitor's cadence instead of flooding the widget tree.
        SmoothStream bool `json:"smoothStream" yaml:"smoothStream"`
        // TargetFPS: the frame rate the UI pacer aims for when coalescing
        // stream updates (default 120; clamped 30-240). 120 delivers one
        // update every ~8.3ms — as smooth as a 120 Hz display can show.
        TargetFPS int `json:"targetFps" yaml:"targetFps"`
}

// AppRoot resolves the portable application root: the directory that
// contains the running executable (on Windows: the sheytan-local-agent/
// folder the zip extracted). EVERYTHING the app reads or writes — config,
// models, sessions, logs, memory, binaries, browser profile, charts —
// lives inside this single folder, so the app can be moved or copied as
// one unit. Falls back to the working directory if the executable lookup
// fails (e.g. exotic test runners).
func AppRoot() string {
        if exe, err := os.Executable(); err == nil && exe != "" {
                // Resolve symlinks (go run / temp build dirs) so the root is
                // the real install location, then take its directory.
                if resolved, err := filepath.EvalSymlinks(exe); err == nil {
                        exe = resolved
                }
                return filepath.Dir(exe)
        }
        wd, err := os.Getwd()
        if err != nil {
                return "."
        }
        return wd
}

// Default returns the portable default config: the data root IS the app
// folder (next to the .exe). SHEYTAN_DATA_DIR can override it for tests.
func Default() *Config {
        dataDir := AppRoot()
        if v := os.Getenv("SHEYTAN_DATA_DIR"); v != "" {
                dataDir = v
        }
        return &Config{
                DataDir:     dataDir,
                ModelsDir:   filepath.Join(dataDir, "models"),
                SessionsDir: filepath.Join(dataDir, "sessions"),
                Host:        "127.0.0.1",
                Port:        8765,
                LLMBaseURL:  "http://127.0.0.1:8080/v1",
                // v1.0.3: NO fake default model. The model shown and served is
                // always one the user actually has: the picker lists only the .gguf
                // files inside the models folder, and an empty Model resolves to the
                // first local file at engine start. A phantom "gemma-4" entry at the
                // top of the UI (v1.0.2 default) misled users whose folder held a
                // completely different model.
                Model:          "",
                LlamaHost:      "127.0.0.1",
                LlamaPort:      8080,
                LlamaAutoStart: true,
                MaxIterations:  25,
                ParallelTools:  true,
                VerboseAgent:   true,
                SandboxEnabled: false,
                SandboxMemory:  "512m",
                SandboxCPU:     1,
                LLM: LLMOptions{
                        Temperature:   0.7,
                        TopP:          0.95,
                        TopK:          40,
                        MaxTokens:     1024,
                        NumCtx:        8192,
                        NumBatch:      512,
                        NumGPU:        0,
                        NumThread:     runtime.NumCPU(),
                        Stream:        true,
                        Preset:        "balanced",
                        RepeatPenalty: 1.1,
                },
                Provider:        ProviderLocal,
                ProMode:         false,
                UpdateSchedule:  "daily",
                BrowserHeadless: true,
                BrowserSlowMo:   0,

                // v1.0.2 defaults
                ThinkingMode:        false,
                AttachmentsBudgetKB: 256,
                HistoryWindowPct:    60,
                RecallEnabled:       true,
                RecallTopK:          4,

                // v1.0.3 defaults
                GPUAutoOffload: true,

                // v1.0.4 Speed Pack defaults
                FlashAttention: true,
                CacheReuse:     32,
                UBatchSize:     512,
                ThreadsBatch:   0,  // 0 = logical cores at launch time
                KVCacheQuant:   "", // off by default (Vulkan driver variance)
                Mlock:          false,
                DraftModel:     "",
                ShowPerfHUD:    true,

                // v1.0.6 defaults
                VisionEnabled:   true,
                VisionMMProj:    "",
                MaxWorkspaceMB:  512,
                MaxSessionsKept: 100,
                MaxLogMB:        50,
                MultiAgentDepth: 3,

                // v1.0.7 defaults
                ContinuumEnabled:         true,
                ContinuumThresholdPct:    75,
                ContinuumCarryMessages:   4,
                ContinuumFrameworkTokens: 700,

                // v1.0.9 defaults
                SmoothStream: true,
                TargetFPS:    120,
        }
}

// Load reads config from `path` (JSON). Falls back to defaults if file
// missing. Before falling back it attempts a one-time migration from the
// legacy ~/.sheytan home-directory layout into the portable app folder.
func Load(path string) (*Config, error) {
        cfg := Default()
        data, err := os.ReadFile(path)
        if err != nil {
                if os.IsNotExist(err) {
                        // First run in portable mode — pull in legacy data if any.
                        MigrateLegacy(cfg.DataDir)
                        if migrated, rerr := os.ReadFile(path); rerr == nil {
                                data = migrated
                        } else {
                                // No config file yet — env overrides still apply
                                // (they have higher priority than defaults).
                                applyEnv(cfg)
                                return cfg, nil
                        }
                } else {
                        return nil, err
                }
        }
        if err := json.Unmarshal(data, cfg); err != nil {
                return nil, fmt.Errorf("parse config: %w", err)
        }
        // A config saved by an older release may still point at ~/.sheytan;
        // re-pin the standard sub-dirs to the portable root (custom dataDir
        // in the file still wins).
        if cfg.DataDir == "" {
                cfg.DataDir = Default().DataDir
        }
        if cfg.ModelsDir == "" {
                cfg.ModelsDir = filepath.Join(cfg.DataDir, "models")
        }
        if cfg.SessionsDir == "" {
                cfg.SessionsDir = filepath.Join(cfg.DataDir, "sessions")
        }
        // Apply env overrides (highest priority)
        applyEnv(cfg)
        return cfg, nil
}

// MigrateLegacy copies a legacy ~/.sheytan data directory into the portable
// app root once (only when the portable root has no config.json yet). It is
// best-effort: any failure is skipped silently — worst case the user starts
// fresh.
func MigrateLegacy(root string) {
        home, err := os.UserHomeDir()
        if err != nil || home == "" {
                return
        }
        legacy := filepath.Join(home, ".sheytan")
        if legacy == root {
                return
        }
        if _, err := os.Stat(filepath.Join(root, "config.json")); err == nil {
                return // portable root already initialized
        }
        fi, err := os.Stat(legacy)
        if err != nil || !fi.IsDir() {
                return // nothing to migrate
        }
        // Files worth carrying over.
        files := []string{"config.json", "memory.jsonl", "installed.json"}
        dirs := []string{"sessions", "logs", "models", "bin"}
        for _, f := range files {
                src := filepath.Join(legacy, f)
                if _, err := os.Stat(src); err == nil {
                        _ = copyFile(src, filepath.Join(root, f))
                }
        }
        for _, d := range dirs {
                src := filepath.Join(legacy, d)
                if fi, err := os.Stat(src); err == nil && fi.IsDir() {
                        _ = copyDir(src, filepath.Join(root, d))
                }
        }
}

func copyFile(src, dst string) error {
        data, err := os.ReadFile(src)
        if err != nil {
                return err
        }
        if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
                return err
        }
        return os.WriteFile(dst, data, 0o644)
}

func copyDir(src, dst string) error {
        return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
                if err != nil {
                        return err
                }
                rel, _ := filepath.Rel(src, p)
                target := filepath.Join(dst, rel)
                if info.IsDir() {
                        return os.MkdirAll(target, 0o755)
                }
                return copyFile(p, target)
        })
}

// Save writes config to `path` as pretty JSON.
func Save(path string, cfg *Config) error {
        if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
                return err
        }
        data, err := json.MarshalIndent(cfg, "", "  ")
        if err != nil {
                return err
        }
        return os.WriteFile(path, data, 0o644)
}

// applyEnv overlays SHEYTAN_* environment variables on the config.
func applyEnv(cfg *Config) {
        if v := os.Getenv("SHEYTAN_HOST"); v != "" {
                cfg.Host = v
        }
        // v1.0.4: speed env overrides (used by stress tests + power users)
        if v := os.Getenv("SHEYTAN_FLASH_ATTENTION"); v != "" {
                cfg.FlashAttention = v == "1" || strings.EqualFold(v, "true")
        }
        if v := os.Getenv("SHEYTAN_KV_CACHE_QUANT"); v != "" {
                cfg.KVCacheQuant = strings.ToLower(v)
        }
        if v := os.Getenv("SHEYTAN_CACHE_REUSE"); v != "" {
                if n, err := strconv.Atoi(v); err == nil {
                        cfg.CacheReuse = n
                }
        }
        if v := os.Getenv("SHEYTAN_PORT"); v != "" {
                if p, err := strconv.Atoi(v); err == nil {
                        cfg.Port = p
                }
        }
        if v := os.Getenv("SHEYTAN_BASE_URL"); v != "" {
                cfg.LLMBaseURL = v
        }
        if v := os.Getenv("SHEYTAN_MODEL"); v != "" {
                cfg.Model = v
        }
        if v := os.Getenv("SHEYTAN_DATA_DIR"); v != "" {
                cfg.DataDir = v
                cfg.ModelsDir = filepath.Join(v, "models")
                cfg.SessionsDir = filepath.Join(v, "sessions")
        }
        if v := os.Getenv("SHEYTAN_LLM_PRESET"); v != "" {
                cfg.LLM.Preset = strings.ToLower(v)
        }
        if v := os.Getenv("SHEYTAN_PROVIDER"); v != "" {
                cfg.Provider = strings.ToLower(v)
        }
        if v := os.Getenv("SHEYTAN_REMOTE_BASE_URL"); v != "" {
                cfg.RemoteBaseURL = v
                if cfg.Provider == "" {
                        cfg.Provider = ProviderRemote
                }
        }
        if v := os.Getenv("SHEYTAN_REMOTE_API_KEY"); v != "" {
                cfg.RemoteAPIKey = v
        }
        if v := os.Getenv("SHEYTAN_REMOTE_MODEL"); v != "" {
                cfg.RemoteModel = v
        }
        if v := os.Getenv("SHEYTAN_BROWSER_PATH"); v != "" {
                cfg.BrowserExecutablePath = v
        }
}

// EnsureDirs creates every required directory under the portable app root.
func (c *Config) EnsureDirs() error {
        for _, p := range []string{
                c.DataDir, c.ModelsDir, c.SessionsDir, c.LogsDir(),
                c.ScreenshotsDir(), c.ChartsDir(), c.BrowserProfileDir(),
                c.SandboxDir(), c.WorkspaceDir(),
                filepath.Join(c.DataDir, "bin"),
        } {
                if err := os.MkdirAll(p, 0o755); err != nil {
                        return fmt.Errorf("mkdir %s: %w", p, err)
                }
        }
        return nil
}

// LogsDir is where the log catcher writes (app.log, tools.jsonl, llm.jsonl).
func (c *Config) LogsDir() string { return filepath.Join(c.DataDir, "logs") }

// ScreenshotsDir is where browser-tool screenshots are saved.
func (c *Config) ScreenshotsDir() string { return filepath.Join(c.DataDir, "logs", "screenshots") }

// ChartsDir is where the data-analysis tool renders SVG charts.
func (c *Config) ChartsDir() string { return filepath.Join(c.DataDir, "charts") }

// BrowserProfileDir is the persistent Chromium profile used by the browser
// automation tool — inside the app folder so sessions survive moves.
func (c *Config) BrowserProfileDir() string { return filepath.Join(c.DataDir, "browser-profile") }

// SandboxDir is where sandboxed code execution writes its temp workdirs.
func (c *Config) SandboxDir() string { return filepath.Join(c.DataDir, "sandbox") }

// WorkspaceDir is the agent's default scratch space for files it creates.
func (c *Config) WorkspaceDir() string { return filepath.Join(c.DataDir, "workspace") }

// ProviderKind returns the effective provider (normalized to local/remote).
func (c *Config) ProviderKind() string {
        if strings.EqualFold(c.Provider, ProviderRemote) {
                return ProviderRemote
        }
        return ProviderLocal
}

// IsRemote reports whether the agent should talk to a remote OpenAI-
// compatible endpoint instead of the bundled llama.cpp server.
func (c *Config) IsRemote() bool { return c.ProviderKind() == ProviderRemote }

// EffectiveBaseURL returns the chat-completions base URL for the active
// provider (no trailing slash).
func (c *Config) EffectiveBaseURL() string {
        if c.IsRemote() && c.RemoteBaseURL != "" {
                return strings.TrimRight(c.RemoteBaseURL, "/")
        }
        return strings.TrimRight(c.LLMBaseURL, "/")
}

// EffectiveAPIKey returns the bearer token for the active provider.
func (c *Config) EffectiveAPIKey() string {
        if c.IsRemote() && c.RemoteAPIKey != "" {
                return c.RemoteAPIKey
        }
        return "no-key"
}

// EffectiveModel returns the model id for the active provider.
func (c *Config) EffectiveModel() string {
        if c.IsRemote() && c.RemoteModel != "" {
                return c.RemoteModel
        }
        return c.Model
}

// DisplayModel returns the short, human-friendly model label for the UI
// chip: the local file name without the .gguf extension (v1.0.3 — the chip
// used to render whatever raw string sat in the config, including a phantom
// default that matched no real file).
func (c *Config) DisplayModel() string {
        m := c.EffectiveModel()
        if m == "" {
                return ""
        }
        return strings.TrimSuffix(m, ".gguf")
}

// DefaultPath returns the canonical portable config path (config.json in
// the app root), honoring SHEYTAN_DATA_DIR.
func DefaultPath() string { return Default().ConfigPath() }

// ConfigPath returns the canonical path to the config file under DataDir.
func (c *Config) ConfigPath() string {
        return filepath.Join(c.DataDir, "config.json")
}

// StatePath returns the path to the installed-components state file.
func (c *Config) StatePath() string {
        return filepath.Join(c.DataDir, "installed.json")
}

// --- v1.0.2 helpers: tool allow-list, budgets ---

// ToolEnabled reports whether the named tool may be called. An empty
// EnabledTools list enables everything.
func (c *Config) ToolEnabled(name string) bool {
        if len(c.EnabledTools) == 0 {
                return true
        }
        for _, n := range c.EnabledTools {
                if strings.EqualFold(n, name) {
                        return true
                }
        }
        return false
}

// EnabledToolList filters `all` down to the enabled tools (stable order).
func (c *Config) EnabledToolList(all []string) []string {
        if len(c.EnabledTools) == 0 {
                return all
        }
        out := make([]string, 0, len(all))
        for _, n := range all {
                if c.ToolEnabled(n) {
                        out = append(out, n)
                }
        }
        return out
}

// AttachmentsBudgetBytes returns the attachment budget in bytes (never zero).
func (c *Config) AttachmentsBudgetBytes() int {
        kb := c.AttachmentsBudgetKB
        if kb <= 0 {
                kb = 256
        }
        return kb * 1024
}

// HistoryWindowTokens returns the token budget for conversation history
// derived from num_ctx and HistoryWindowPct (clamped to 10-95%, default 60).
func (c *Config) HistoryWindowTokens() int {
        ctxN := c.LLM.NumCtx
        if ctxN <= 0 {
                ctxN = 8192
        }
        pct := c.HistoryWindowPct
        if pct <= 0 {
                pct = 60
        }
        if pct > 95 {
                pct = 95
        }
        return ctxN * pct / 100
}

// EffectiveRecallTopK returns the recall capsule limit (>= 1).
func (c *Config) EffectiveRecallTopK() int {
        if c.RecallTopK <= 0 {
                return 4
        }
        if c.RecallTopK > 12 {
                return 12
        }
        return c.RecallTopK
}

// --- v1.0.4 Speed Pack helpers ---

// EffectiveKVCacheQuant validates the KV cache quantization string.
// Anything unknown normalizes to "" (off) — a typo must never brick the
// engine launch.
func (c *Config) EffectiveKVCacheQuant() string {
        switch strings.ToLower(strings.TrimSpace(c.KVCacheQuant)) {
        case "q8_0":
                return "q8_0"
        case "q4_0":
                return "q4_0"
        default:
                return ""
        }
}

// EffectiveCacheReuse clamps the KV-shift reuse chunk to llama.cpp's
// accepted range (0 = off, else 1..512).
func (c *Config) EffectiveCacheReuse() int {
        if c.CacheReuse <= 0 {
                return 0
        }
        if c.CacheReuse > 512 {
                return 512
        }
        return c.CacheReuse
}

// EffectiveUBatchSize clamps the physical batch size (default 512;
// llama.cpp accepts 1..8192 — huge values only waste memory).
func (c *Config) EffectiveUBatchSize() int {
        if c.UBatchSize <= 0 {
                return 512
        }
        if c.UBatchSize > 8192 {
                return 8192
        }
        return c.UBatchSize
}

// --- v1.0.6 helpers: vision + resources ---

// EffectiveMultiAgentDepth clamps the pipeline planning depth (1-5).
func (c *Config) EffectiveMultiAgentDepth() int {
        if c.MultiAgentDepth <= 0 {
                return 3
        }
        if c.MultiAgentDepth > 5 {
                return 5
        }
        return c.MultiAgentDepth
}

// EffectiveContinuumThreshold clamps the chapter-rollover trigger (50-95%
// of the history budget; 0/negative = default 75).
func (c *Config) EffectiveContinuumThreshold() int {
        if c.ContinuumThresholdPct <= 0 {
                return 75
        }
        if c.ContinuumThresholdPct < 50 {
                return 50
        }
        if c.ContinuumThresholdPct > 95 {
                return 95
        }
        return c.ContinuumThresholdPct
}

// EffectiveContinuumCarry clamps how many recent messages ride into the
// next chapter verbatim (0-16; negative = default 4).
func (c *Config) EffectiveContinuumCarry() int {
        if c.ContinuumCarryMessages < 0 {
                return 4
        }
        if c.ContinuumCarryMessages > 16 {
                return 16
        }
        return c.ContinuumCarryMessages
}

// EffectiveContinuumFrameworkTokens clamps the rendered Framework briefing
// budget (200-2000 tokens; 0 = default 700).
func (c *Config) EffectiveContinuumFrameworkTokens() int {
        if c.ContinuumFrameworkTokens <= 0 {
                return 700
        }
        if c.ContinuumFrameworkTokens < 200 {
                return 200
        }
        if c.ContinuumFrameworkTokens > 2000 {
                return 2000
        }
        return c.ContinuumFrameworkTokens
}

// EffectiveMaxWorkspaceMB returns the workspace soft quota (0 = unlimited,
// negative normalizes to 0).
func (c *Config) EffectiveMaxWorkspaceMB() int {
        if c.MaxWorkspaceMB < 0 {
                return 0
        }
        return c.MaxWorkspaceMB
}

// EffectiveMaxSessionsKept returns the session retention target (0 = keep
// everything; clamped to >= 10 when set).
func (c *Config) EffectiveMaxSessionsKept() int {
        if c.MaxSessionsKept <= 0 {
                return 0
        }
        if c.MaxSessionsKept < 10 {
                return 10
        }
        return c.MaxSessionsKept
}

// EffectiveMaxLogMB returns the log folder size target in MB (0 = no limit).
func (c *Config) EffectiveMaxLogMB() int {
        if c.MaxLogMB < 0 {
                return 0
        }
        return c.MaxLogMB
}

// EffectiveTargetFPS returns the UI frame-pacing target (default 120,
// clamped 30-240). Values below 30 feel choppy; above 240 the coalescing
// overhead outweighs any perceptible smoothness.
func (c *Config) EffectiveTargetFPS() int {
        fps := c.TargetFPS
        if fps == 0 {
                fps = 120
        }
        if fps < 30 {
                fps = 30
        }
        if fps > 240 {
                fps = 240
        }
        return fps
}

// EffectiveStreamEmitInterval returns how often the orchestrator forwards
// a streaming snapshot while tokens arrive. It is derived from the frame
// target (one emit per frame) but never faster than 8ms — that is the
// 120 Hz cadence and the sweet spot for the UI pacer.
func (c *Config) EffectiveStreamEmitInterval() time.Duration {
        interval := time.Second / time.Duration(c.EffectiveTargetFPS())
        if interval < 8*time.Millisecond {
                interval = 8 * time.Millisecond
        }
        if interval > 80*time.Millisecond {
                interval = 80 * time.Millisecond
        }
        return interval
}
