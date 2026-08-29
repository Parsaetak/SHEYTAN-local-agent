// Package aicontext ships SHEYTAN's AI instruction file (AI-CONTEXT.md) and
// builds the environment briefing that is prepended, as the first system
// message, to EVERY conversation the app runs — local GGUF models and remote
// providers alike.
//
// Design:
//   - The canonical file is embedded in the binary and written to the app
//     folder on first run, so the user can read AND edit it. It carries a
//     version marker; an app upgrade that ships a newer instruction version
//     regenerates the file (files without a marker are treated as
//     user-authored and are never touched).
//   - The live environment block (OS, hardware, provider/model,
//     connectivity, date) is appended AFTER the stable instruction text so
//     the prompt prefix stays byte-identical across turns — llama.cpp
//     reuses its KV/prompt cache only when the prefix is unchanged.
package aicontext

import (
        _ "embed"
        "fmt"
        "os"
        "path/filepath"
        "runtime"
        "strings"
        "sync"
        "time"

        "github.com/sheytan/local-agent/internal/config"
        "github.com/sheytan/local-agent/internal/netcheck"
        "github.com/sheytan/local-agent/internal/sysinfo"
        "github.com/sheytan/local-agent/internal/vision"
)

// FileName is the instruction file's name inside the app folder.
const FileName = "AI-CONTEXT.md"

// ContextVersion is the instruction-file version. Bump it whenever the
// embedded AI-CONTEXT.md changes in a release; existing installs with an
// older marker get the fresh file on next boot.
const ContextVersion = 10

//go:embed AI-CONTEXT.md
var embedded string

// markerLine is the machine-readable version marker inside the file.
const markerLine = "<!-- sheytan-context-version:"

// Path returns the instruction file's location for a data dir (app folder).
func Path(dataDir string) string { return filepath.Join(dataDir, FileName) }

// Embedded returns the canonical embedded instruction text.
func Embedded() string { return embedded }

// EnsureFile materializes AI-CONTEXT.md in the app folder:
//   - missing                       → write the embedded canonical file
//   - marker version < ContextVersion → overwrite with the new canonical
//   - marker version >= current     → keep (the user may have edited it)
//   - no marker (user-authored file)→ keep, untouched
//
// It returns the path of the effective file.
func EnsureFile(dataDir string) (string, error) {
        p := Path(dataDir)
        data, err := os.ReadFile(p)
        switch {
        case err == nil:
                v, hasMarker := fileVersion(string(data))
                if hasMarker && v >= ContextVersion {
                        return p, nil // up to date (possibly user-edited) — keep
                }
                if !hasMarker {
                        return p, nil // user-authored file without marker — leave alone
                }
                // outdated marker → regenerate below
        case os.IsNotExist(err):
                // missing → write below
        default:
                return p, err
        }
        if err := os.MkdirAll(dataDir, 0o755); err != nil {
                return p, err
        }
        if err := os.WriteFile(p, []byte(embedded), 0o644); err != nil {
                return p, err
        }
        return p, nil
}

// fileVersion extracts the marker version from an AI-CONTEXT.md body.
func fileVersion(body string) (int, bool) {
        idx := strings.Index(body, markerLine)
        if idx < 0 {
                return 0, false
        }
        rest := body[idx+len(markerLine):]
        end := strings.IndexAny(rest, ">-\n")
        if end < 0 {
                end = len(rest)
        }
        v := 0
        for _, ch := range strings.TrimSpace(rest[:end]) {
                if ch < '0' || ch > '9' {
                        break
                }
                v = v*10 + int(ch-'0')
        }
        if v <= 0 {
                return 0, false
        }
        return v, true
}

// Load returns the effective instruction text: the file in the app folder
// when present (it may be user-customized), otherwise the embedded copy.
func Load(dataDir string) string {
        if data, err := os.ReadFile(Path(dataDir)); err == nil && len(data) > 0 {
                return string(data)
        }
        return embedded
}

// --- live environment briefing ---

var (
        probeOnce sync.Once
        probeInfo *sysinfo.SysInfo
)

// cachedProbe runs the hardware probe once per process — probing GPU
// drivers and disk stats is far too expensive to repeat per turn.
func cachedProbe() *sysinfo.SysInfo {
        probeOnce.Do(func() { probeInfo = sysinfo.Probe() })
        return probeInfo
}

// ResetProbeCache drops the cached hardware snapshot (tests).
func ResetProbeCache() {
        probeOnce = sync.Once{}
        probeInfo = nil
}

// toolNames lists the built-in tools in canonical order. The orchestrator
// registers exactly these names (codeExec may be the sandboxed variant on
// Windows — same name, stronger isolation).
var toolNames = []string{
        "files", "shell", "codeExec", "dataAnalysis", "webSearch", "browser",
        "git", "memory", "screenshot", "linux",
        "json", "archive", "fetch", "diff",
}

// Briefing builds the LIVE ENVIRONMENT block for the active config. It is
// deliberately compact and dated to day granularity so the prompt prefix
// stays stable within a session (see package comment).
func Briefing(cfg *config.Config) string {
        var b strings.Builder
        b.WriteString("\n\n---\n\n## LIVE ENVIRONMENT\n\n")
        fmt.Fprintf(&b, "- App: SHEYTAN-Local-Agent v%s (native desktop, local-first)\n", config.AppVersion)
        fmt.Fprintf(&b, "- Date: %s\n", time.Now().Format("2006-01-02 (MST)"))
        fmt.Fprintf(&b, "- OS: %s/%s", cap(runtime.GOOS), runtime.GOARCH)
        if info := cachedProbe(); info != nil {
                if info.CPU.Name != "" {
                        fmt.Fprintf(&b, " · CPU: %s (%d cores)", info.CPU.Name, info.CPU.LogicalCores)
                }
                if info.RAM.TotalBytes > 0 {
                        fmt.Fprintf(&b, " · RAM: %s", sysinfo.FormatBytes(info.RAM.TotalBytes))
                }
                if len(info.GPU) > 0 && info.GPU[0].Name != "" {
                        fmt.Fprintf(&b, " · GPU: %s", info.GPU[0].Name)
                }
        }
        b.WriteString("\n")
        if cfg != nil {
                fmt.Fprintf(&b, "- Working directory (app folder): %s\n", cfg.DataDir)
                if cfg.IsRemote() {
                        fmt.Fprintf(&b, "- Provider: remote OpenAI-compatible endpoint · model: %s\n", cfg.EffectiveModel())
                } else {
                        fmt.Fprintf(&b, "- Provider: local llama.cpp (bundled) · model: %s\n", cfg.EffectiveModel())
                }
        }
        if netcheck.IsOffline() {
                b.WriteString("- Connectivity: OFFLINE — webSearch and browser are DISABLED; all local tools work\n")
        } else {
                b.WriteString("- Connectivity: online (webSearch and browser available)\n")
        }
        // v1.0.2 tool selection: list only what the user actually enabled so
        // the model never plans around a disabled tool. Empty list = all.
        tools := toolNames
        if cfg != nil {
                tools = cfg.EnabledToolList(toolNames)
        }
        b.WriteString("- Tools available: " + strings.Join(tools, ", ") + "\n")
        if cfg != nil && len(cfg.EnabledTools) > 0 {
                b.WriteString("- Tool selection: the user restricted the toolset — never call a tool outside this list\n")
        }
        // v1.0.6 vision status: tells the model whether attached images and
        // the screenshot tool arrive as REAL image parts (projector paired)
        // or as text notes. Static (config + models folder) so the prefix
        // stays cacheable within a session.
        if cfg != nil && !cfg.IsRemote() && cfg.VisionEnabled {
                if proj := vision.FindProjector(cfg.ModelsDir, cfg.Model, cfg.VisionMMProj); proj != "" {
                        fmt.Fprintf(&b, "- Vision: ENABLED (projector %s) — attached images and screenshot captures arrive as real image parts you can see; analyze what is actually visible\n", filepath.Base(proj))
                }
        }
        // v1.0.7 continuum status: teaches the model that long threads roll
        // into chapters and how to treat the FRAMEWORK briefing block.
        if cfg != nil && cfg.ContinuumEnabled {
                fmt.Fprintf(&b, "- Continuum: ENABLED — long conversations roll into chapters; a [CONTINUUM FRAMEWORK] system block is your distilled memory of earlier chapters (mission, facts, decisions, open threads). Treat it as your own memory: continue seamlessly, never re-ask anything it answers, and pick up its open threads.\n")
        }
        b.WriteString("- Full instructions: see the AI Operating Instructions above and AI-CONTEXT.md in the app folder.\n")
        return b.String()
}

// SystemMessage builds the complete first system message: the instruction
// file (disk copy, user-editable, embedded fallback) + the live environment
// block. Callers should prepend it as message[0] exactly once per
// conversation.
func SystemMessage(cfg *config.Config) string {
        dataDir := ""
        if cfg != nil {
                dataDir = cfg.DataDir
        }
        return Load(dataDir) + Briefing(cfg)
}

// HeaderSentinel is a stable substring identifying an AI-context system
// message, so callers can avoid double-prepending it.
const HeaderSentinel = "SHEYTAN™ Local-Agent — AI Operating Instructions"

func cap(s string) string {
        if s == "" {
                return s
        }
        return strings.ToUpper(s[:1]) + s[1:]
}
