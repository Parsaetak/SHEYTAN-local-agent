package llm

import (
        "archive/zip"
        "compress/gzip"
        "context"
        "encoding/json"
        "fmt"
        "io"
        "net"
        "net/http"
        "os"
        "os/exec"
        "path/filepath"
        "runtime"
        "strings"
        "sync"
        "syscall"
        "time"

        "github.com/sheytan/local-agent/internal/config"
        "github.com/sheytan/local-agent/internal/logging"
        "github.com/sheytan/local-agent/internal/netcheck"
        "github.com/sheytan/local-agent/internal/proc"
        "github.com/sheytan/local-agent/internal/sysinfo"
        "github.com/sheytan/local-agent/internal/updater"
        "github.com/sheytan/local-agent/internal/vision"
)

// LlamaServer manages a llama.cpp server subprocess. This is the standalone
// inference engine that replaces LM Studio's local server.
type LlamaServer struct {
        cfg      *config.Config
        cmd      *exec.Cmd
        mu       sync.Mutex
        state    string // "stopped" | "starting" | "running" | "error"
        stateCh  chan string
        logBuf   *ringBuffer
        errRing  *ringBuffer // v1.0.5: stderr tail — the REAL reason a launch died
        loaded   string      // absolute path of the model currently loaded by the subprocess
        switchMu sync.Mutex

        // mmproj is the multimodal projector (v1.0.6) the engine was launched
        // with — empty when the engine runs text-only. Resolved at start from
        // cfg.VisionEnabled/VisionMMProj + pairing with the selected model.
        mmproj string

        // engineUpdateTried guards the v1.0.5 self-heal: when a model fails to
        // load because the bundled engine predates its architecture, the engine
        // is auto-updated once per app run and the load retried.
        engineUpdateTried bool
}

type ringBuffer struct {
        mu   sync.Mutex
        buf  []string
        head int
        size int
}

func newRing(n int) *ringBuffer { return &ringBuffer{buf: make([]string, n), size: n} }
func (r *ringBuffer) add(s string) {
        r.mu.Lock()
        defer r.mu.Unlock()
        r.buf[r.head] = s
        r.head = (r.head + 1) % r.size
}
func (r *ringBuffer) lines() []string {
        r.mu.Lock()
        defer r.mu.Unlock()
        out := make([]string, 0, r.size)
        for i := 0; i < r.size; i++ {
                idx := (r.head + i) % r.size
                if r.buf[idx] != "" {
                        out = append(out, r.buf[idx])
                }
        }
        return out
}

// reset clears the ring (each launch attempt starts with a clean tail).
func (r *ringBuffer) reset() {
        r.mu.Lock()
        defer r.mu.Unlock()
        r.buf = make([]string, r.size)
}

// NewLlamaServer returns a manager bound to the given config.
func NewLlamaServer(cfg *config.Config) *LlamaServer {
        return &LlamaServer{
                cfg:     cfg,
                stateCh: make(chan string, 16),
                logBuf:  newRing(500),
                errRing: newRing(64),
        }
}

// State returns the current server state.
func (s *LlamaServer) State() string {
        s.mu.Lock()
        defer s.mu.Unlock()
        return s.state
}

// Logs returns the most recent log lines from the subprocess.
func (s *LlamaServer) Logs() []string {
        return s.logBuf.lines()
}

// IsRunning returns true if the subprocess is alive and listening.
func (s *LlamaServer) IsRunning() bool {
        s.mu.Lock()
        defer s.mu.Unlock()
        return s.state == "running"
}

// ensureBinary makes sure the llama.cpp server binary exists. v1.0.3: the
// engine is BUNDLED with the app (bin/ ships inside the portable folder),
// so this normally just finds it. For legacy installs without bin/ it
// downloads a prebuilt binary from the official release URL.
func (s *LlamaServer) ensureBinary() error {
        if s.cfg.LlamaBinPath == "" {
                s.cfg.LlamaBinPath = filepath.Join(s.cfg.DataDir, "bin", llamaBinaryName())
        }
        if _, err := os.Stat(s.cfg.LlamaBinPath); err == nil {
                // v1.0.3: record the bundled build's lineage once so the updater can
                // compare against upstream releases from the very first run.
                if updater.InstalledEngineTag(s.cfg) == "" {
                        updater.RecordEngineTag(s.cfg, updater.DefaultEngineTag)
                }
                return nil
        }
        // Offline fast-fail: never attempt (and wait on) a download when
        // there is no connectivity — tell the user exactly what to do.
        if netcheck.IsOffline() {
                return fmt.Errorf("llama.cpp server binary missing and you appear to be OFFLINE. "+
                        "Reconnect once so the server can be downloaded automatically, or place a prebuilt "+
                        "llama-server(.exe) into %s and retry", filepath.Dir(s.cfg.LlamaBinPath))
        }
        // Need to download
        dir := filepath.Dir(s.cfg.LlamaBinPath)
        if err := os.MkdirAll(dir, 0o755); err != nil {
                return err
        }
        s.setState("downloading")
        url, err := llamaDownloadURL()
        if err != nil {
                return err
        }
        s.logf("Downloading llama.cpp server from %s", url)
        if err := downloadAndExtract(url, dir); err != nil {
                return fmt.Errorf("download llama.cpp: %w", err)
        }
        if _, err := os.Stat(s.cfg.LlamaBinPath); err != nil {
                return fmt.Errorf("llama.cpp binary not found at %s after extraction", s.cfg.LlamaBinPath)
        }
        _ = os.Chmod(s.cfg.LlamaBinPath, 0o755)
        // Record the build lineage so the scheduled updater can compare against
        // upstream releases (v1.0.0).
        updater.RecordEngineTag(s.cfg, updater.DefaultEngineTag)
        return nil
}

// modelLoadTimeout bounds how long the subprocess may take to become HTTP-
// ready. v1.0.3: raised from 60s to 180s — a 7–8B Q4 model on a slow CPU
// or HDD genuinely needs minutes for the first cold start, and the old
// 60s cap aborted perfectly healthy boots.
const modelLoadTimeout = 180 * time.Second

// engineCompatMax is the last (most conservative) launch profile index.
const engineCompatMax = 3

// compatLevelName renders a launch-profile label for logs and the UI.
func compatLevelName(level int) string {
        switch level {
        case 1:
                return "template compat (--jinja)"
        case 2:
                return "no speed flags"
        case 3:
                return "safe mode (CPU)"
        default:
                return "full speed"
        }
}

// Start boots the llama.cpp server as a subprocess (if not already running).
// Serialized on switchMu so a prewarm and a first-send can never race.
func (s *LlamaServer) Start() error {
        s.switchMu.Lock()
        defer s.switchMu.Unlock()
        return s.startLocked()
}

// startLocked is the real boot path; the caller holds switchMu.
//
// v1.0.5 — the compatibility ladder. A model can make llama.cpp exit with
// code 1 during startup for a handful of reasons that each need a DIFFERENT
// fix (a chat template the default parser rejects, a flag the model's
// attention type disagrees with, a GPU backend the model crashes on, or an
// architecture the bundled engine predates). Instead of surfacing a bare
// "exit status 1", the launcher now retries with progressively safer flag
// profiles and reports the engine's own stderr — the REAL reason — in the
// error text. When even the bare profile dies with "unknown architecture",
// the engine self-updates once and tries again (brand-new model families
// become usable without waiting for the next app release).
func (s *LlamaServer) startLocked() error {
        s.mu.Lock()
        if s.state == "running" || s.state == "starting" {
                s.mu.Unlock()
                return nil
        }
        // v1.0.3: fail fast (with a clear message) when the configured port is
        // already occupied by something that is NOT our subprocess — llama.cpp
        // would otherwise die instantly with a confusing bind error.
        if PortInUse(s.cfg.LlamaHost, s.cfg.LlamaPort) {
                // Is it OUR health endpoint? Then an orphan from a previous run is
                // already serving — adopt it instead of failing.
                if s.adoptExisting() {
                        s.mu.Lock()
                        s.state = "running"
                        s.mu.Unlock()
                        return nil
                }
                s.mu.Unlock()
                s.setState("error")
                return fmt.Errorf("port %d is already in use by another program — change LlamaPort in config.json or close the other app", s.cfg.LlamaPort)
        }
        s.state = "starting"
        s.mu.Unlock()

        if err := s.ensureBinary(); err != nil {
                s.setState("error")
                return err
        }

        // Pick a model file. v1.0.0: the model name may be a bare filename
        // ("qwen.gguf"), a substring the user typed, or an absolute path —
        // resolveModelPath turns ANY of those into an existing absolute file.
        modelPath, err := ResolveModelPath(s.cfg.ModelsDir, s.cfg.Model)
        if err != nil {
                s.setState("error")
                return err
        }

        // v1.0.6 VISION: resolve the multimodal projector that pairs with the
        // model. The engine launches with --mmproj <file> so images and
        // screenshots reach the vision encoder. When every profile dies WITH
        // the projector (corrupt or mismatched mmproj), the whole ladder runs
        // once more without it — a text-only engine beats no engine at all.
        mmproj := ""
        if s.cfg.VisionEnabled {
                if p := vision.FindProjector(s.cfg.ModelsDir, modelPath, s.cfg.VisionMMProj); p != "" {
                        mmproj = p
                        s.logf("vision projector paired: %s", filepath.Base(p))
                }
        }
        s.mu.Lock()
        s.mmproj = mmproj
        s.mu.Unlock()
        visionRetryDone := false

        for {
                // Begin at the profile that worked last time (a model that needed safe
                // mode keeps booting in safe mode), and walk upward only.
                startLevel := s.cfg.EngineCompat
                if startLevel < 0 || startLevel > engineCompatMax {
                        startLevel = 0
                }

                var lastErr error
                for pass := 0; pass < 2; pass++ {
                        for level := startLevel; level <= engineCompatMax; level++ {
                                err := s.launchOnce(modelPath, level)
                                if err == nil {
                                        if s.cfg.EngineCompat != level {
                                                s.cfg.EngineCompat = level
                                                _ = config.Save(s.cfg.ConfigPath(), s.cfg)
                                        }
                                        if level > 0 {
                                                s.logf("engine started in compatibility mode %d (%s) — some speed flags disabled", level, compatLevelName(level))
                                                logging.Default().Warn("engine", "started in compatibility mode %d (%s) for %s",
                                                        level, compatLevelName(level), filepath.Base(modelPath))
                                        }
                                        s.setState("running")
                                        return nil
                                }
                                lastErr = err
                                logging.Default().Error("engine", "startup attempt failed (compat %d, %s): %v",
                                        level, compatLevelName(level), err)
                                // Only retry when the subprocess actually DIED — a timeout with
                                // a live process (slow disk, huge model) will not improve with
                                // different flags.
                                if _, died := err.(*exitFailure); !died {
                                        s.setState("error")
                                        return err
                                }
                                // the port needs a beat to be released after a kill
                                time.Sleep(250 * time.Millisecond)
                        }
                        // Every profile died. When the engine itself says it does not know
                        // the model's architecture, a newer llama.cpp is the only real fix —
                        // fetch it once and run the whole ladder again on the fresh build.
                        if pass == 0 && needsNewerEngine(lastErr) && s.updateEngineForModel() {
                                startLevel = 0
                                s.setState("starting")
                                continue
                        }
                        break
                }

                // v1.0.6: every profile (including after an engine self-update)
                // died with the projector attached → one final full ladder run
                // WITHOUT vision. Text-only is strictly better than a dead app.
                if mmproj != "" && !visionRetryDone {
                        visionRetryDone = true
                        projectName := filepath.Base(mmproj)
                        mmproj = ""
                        s.mu.Lock()
                        s.mmproj = ""
                        s.mu.Unlock()
                        s.logf("all profiles failed with the vision projector — retrying text-only")
                        logging.Default().Warn("engine", "vision projector %s failed with every profile; restarting without vision", projectName)
                        s.setState("starting")
                        continue
                }
                s.setState("error")
                return lastErr
        }
}

// VisionActive reports whether the running engine carries a multimodal
// projector (v1.0.6) — i.e. the model can see images and screenshots.
func (s *LlamaServer) VisionActive() bool {
        s.mu.Lock()
        defer s.mu.Unlock()
        return s.state == "running" && s.mmproj != ""
}

// ProjectorPath returns the active projector file ("" when text-only).
func (s *LlamaServer) ProjectorPath() string {
        s.mu.Lock()
        defer s.mu.Unlock()
        return s.mmproj
}

// Pid returns the engine subprocess pid (0 when not running) — the
// Resources view probes its live memory through it.
func (s *LlamaServer) Pid() int {
        s.mu.Lock()
        defer s.mu.Unlock()
        if s.cmd != nil && s.cmd.Process != nil {
                return s.cmd.Process.Pid
        }
        return 0
}

// buildArgs assembles the llama.cpp server command line for a compatibility
// level:
//
//      0 — everything: Speed Pack flags, GPU offload, user extra args
//      1 — everything + --jinja (chat templates that need a real Jinja parser)
//      2 — --jinja + GPU, but no speed flags (flash-attn / cache-reuse /
//          kv-quant / batch tuning are the historically crashy ones)
//      3 — bare: model, host/port, context, threads. No jinja, no GPU, no
//          extras. The profile that runs on anything.
func (s *LlamaServer) buildArgs(modelPath string, level int) []string {
        base := []string{
                "--model", modelPath,
                "--host", s.cfg.LlamaHost,
                "--port", fmt.Sprintf("%d", s.cfg.LlamaPort),
                "--ctx-size", fmt.Sprintf("%d", s.cfg.LLM.NumCtx),
                "--batch-size", fmt.Sprintf("%d", s.cfg.LLM.NumBatch),
                "--threads", fmt.Sprintf("%d", threadsFor(s.cfg)),
                "--temp", fmt.Sprintf("%v", s.cfg.LLM.Temperature),
                "--top-p", fmt.Sprintf("%v", s.cfg.LLM.TopP),
                "--top-k", fmt.Sprintf("%d", s.cfg.LLM.TopK),
                "--repeat-penalty", fmt.Sprintf("%v", s.cfg.LLM.RepeatPenalty),
        }
        // v1.0.6 VISION: the multimodal projector rides along at every
        // compatibility level — the vision-retry pass in startLocked strips
        // it for the final ladder run when it is the crash cause.
        s.mu.Lock()
        mmproj := s.mmproj
        s.mu.Unlock()
        if mmproj != "" {
                base = append(base, "--mmproj", mmproj)
        }
        if s.cfg.LLM.Mirostat > 0 {
                base = append(base, "--mirostat", fmt.Sprintf("%d", s.cfg.LLM.Mirostat))
        }
        if s.cfg.LLM.Seed != 0 {
                base = append(base, "--seed", fmt.Sprintf("%d", s.cfg.LLM.Seed))
        }
        if level == engineCompatMax {
                return base // bare profile — nothing else
        }

        // GPU offload: an explicit NumGPU always wins; the auto-offload heuristic
        // (Vulkan backend + detected GPU) applies through level 2. Level 3 is
        // pure CPU by design.
        gpuLayers := s.cfg.LLM.NumGPU
        if gpuLayers <= 0 && s.autoGPUOffload() {
                gpuLayers = 99
        }
        if gpuLayers > 0 {
                base = append(base, "--n-gpu-layers", fmt.Sprintf("%d", gpuLayers))
        }

        // v1.0.4 Speed Pack — research-backed throughput flags (see SpeedArgs
        // for the rationale of each). Dropped from level 2 up.
        if level <= 1 {
                base = append(base, SpeedArgs(s.cfg)...)
        } else {
                base = append(base, "--no-webui")
        }

        // v1.0.5: --jinja lets the server parse GGUF chat templates with a real
        // Jinja engine — the fix for models whose template the built-in common
        // templates reject (a classic silent exit-1 for fresh gemma releases).
        if level >= 1 {
                base = append(base, "--jinja")
        }

        // User-provided extra args survive through level 2 (level 3 must isolate
        // the engine from ANYTHING that could be the crash cause).
        if level <= 2 && strings.TrimSpace(s.cfg.LlamaExtraArgs) != "" {
                base = append(base, strings.Fields(s.cfg.LlamaExtraArgs)...)
        }
        return base
}

// procExit carries one subprocess's death: done is CLOSED exactly once by
// the monitor goroutine (a closed channel can be observed by any number of
// readers — unlike a consumed-once value channel, which stalled the retry
// ladder when two readers both wanted the exit signal).
type procExit struct {
        done chan struct{}
        err  error // valid after done is closed
}

// launchOnce spawns the server with one profile's flags and waits until it
// is HTTP-ready. Returns nil on success; a *exitFailure when the process
// died (retry-worthy); any other error means "not retry-worthy".
func (s *LlamaServer) launchOnce(modelPath string, level int) error {
        if level > 0 {
                s.logf("launching with compatibility profile %d (%s)…", level, compatLevelName(level))
        }
        args := s.buildArgs(modelPath, level)

        cmd := proc.Command(s.cfg.LlamaBinPath, args...)
        cmd.Dir = s.cfg.DataDir // pin cwd to the app folder — never inherit an arbitrary launcher cwd
        cmd.Stdout = newLineWriter(func(line string) { s.logf("[llama.cpp] %s", line) })
        s.errRing.reset()
        cmd.Stderr = newLineWriter(func(line string) {
                s.logf("[llama.cpp!] %s", line)
                s.errRing.add(line)
        })
        // v1.0.4: CREATE_NO_WINDOW — the engine is a console app; without
        // this flag Windows opened a terminal that stayed up for the whole
        // session (the "extra terminal" bug).
        proc.Hide(cmd)

        if err := cmd.Start(); err != nil {
                return &exitFailure{err: fmt.Errorf("start llama.cpp: %w", err)}
        }

        s.mu.Lock()
        s.cmd = cmd
        s.mu.Unlock()

        // Monitor + exit signal: the moment the subprocess dies we know it
        // (v1.0.3) — a missing DLL or a bad flag fails in seconds with a REAL
        // error instead of burning the full 3-minute health timeout.
        exit := &procExit{done: make(chan struct{})}
        go func() {
                exit.err = cmd.Wait()
                s.mu.Lock()
                latest := s.cmd == cmd
                if latest {
                        s.cmd = nil
                }
                wasRunning := latest && s.loaded == modelPath && s.state == "running"
                if wasRunning {
                        s.loaded = ""
                }
                s.mu.Unlock()
                if wasRunning {
                        // the engine died mid-session (not during a launch attempt)
                        s.setState("stopped")
                        logging.Default().Error("engine", "engine exited while running: %v", exit.err)
                }
                close(exit.done)
        }()

        if err := s.waitReadySignaled(modelLoadTimeout, exit); err != nil {
                // Make sure the failed attempt is fully gone before any retry so the
                // port is free and no zombie lingers. When the monitor already
                // reaped the process (it cleared s.cmd), only wait for its signal.
                s.mu.Lock()
                c := s.cmd
                if s.cmd == cmd {
                        s.cmd = nil
                }
                s.mu.Unlock()
                if c != nil && c.Process != nil {
                        _ = c.Process.Kill()
                }
                select {
                case <-exit.done:
                case <-time.After(3 * time.Second):
                }
                return err
        }

        // The health check passed a moment ago — make sure the process did not
        // die in the tiny window between the check and now (the monitor clears
        // s.cmd on death, which is how we can tell).
        s.mu.Lock()
        if s.cmd != cmd {
                s.mu.Unlock()
                select {
                case <-exit.done:
                case <-time.After(time.Second):
                }
                return &exitFailure{
                        err:  fmt.Errorf("llama.cpp died immediately after becoming ready"),
                        tail: s.errRing.lines(),
                }
        }
        s.loaded = modelPath
        s.mu.Unlock()
        return nil
}

// waitReadySignaled polls /health until 200, but aborts immediately when
// the subprocess exits (its stderr tail explains why) or the timeout passes.
func (s *LlamaServer) waitReadySignaled(timeout time.Duration, exit *procExit) error {
        deadline := time.Now().Add(timeout)
        url := fmt.Sprintf("http://%s:%d/health", s.cfg.LlamaHost, s.cfg.LlamaPort)
        client := &http.Client{Timeout: 2 * time.Second}
        for time.Now().Before(deadline) {
                if resp, err := client.Get(url); err == nil {
                        _ = resp.Body.Close()
                        if resp.StatusCode == 200 {
                                return nil
                        }
                }
                select {
                case <-exit.done:
                        return s.exitError(exit.err)
                case <-time.After(400 * time.Millisecond):
                }
        }
        select {
        case <-exit.done:
                return s.exitError(exit.err)
        default:
        }
        tail := s.tailLines(6)
        msg := fmt.Sprintf("llama.cpp did not become ready within %v (the model may be too large for this machine)", timeout)
        if tail != "" {
                msg += "\n\nEngine output (last lines):\n" + tail
        }
        return fmt.Errorf("%s", msg)
}

// exitError builds the launch-failure error for a dead subprocess: the
// friendly explanation PLUS the engine's own last stderr lines, so the user
// and the logs see the REAL reason (unknown architecture, template parse
// failure, out-of-memory, …) instead of a bare "exit status 1".
func (s *LlamaServer) exitError(err error) error {
        return &exitFailure{err: explainExit(err), tail: s.errRing.lines()}
}

// exitFailure marks "the subprocess died during startup" — the only class
// of launch error worth retrying with different flags.
type exitFailure struct {
        err  error
        tail []string
}

// explainExit turns a raw subprocess exit error into actionable guidance.
// Windows status 0xC0000135 = DLL not found: the official llama.cpp builds
// link the MSVC runtime, which a small fraction of Windows installs lack.
func explainExit(err error) error {
        if err == nil {
                return fmt.Errorf("llama.cpp exited during startup")
        }
        msg := err.Error()
        if strings.Contains(msg, "3221225781") || strings.Contains(msg, "0xc0000135") {
                return fmt.Errorf("llama.cpp could not start: a required Windows DLL is missing. "+
                        "Install the free Microsoft Visual C++ Redistributable (64-bit) from "+
                        "https://aka.ms/vs/17/release/vc_redist.x64.exe and start again: %v", err)
        }
        return fmt.Errorf("llama.cpp exited during startup: %v", err)
}

func (e *exitFailure) Error() string {
        msg := "llama.cpp exited during startup"
        if e.err != nil {
                msg = e.err.Error()
        }
        if tail := compactLines(e.tail, 12); tail != "" {
                msg += "\n\nEngine output (last lines):\n" + tail
        }
        return msg
}

func (e *exitFailure) Unwrap() error { return e.err }

// tailLines returns the last n stderr lines, clipped ("" when quiet).
func (s *LlamaServer) tailLines(n int) string {
        return compactLines(s.errRing.lines(), n)
}

// compactLines joins the last n non-empty lines, each clipped to 200 chars.
func compactLines(lines []string, n int) string {
        if len(lines) > n {
                lines = lines[len(lines)-n:]
        }
        var kept []string
        for _, l := range lines {
                l = strings.TrimSpace(l)
                if l == "" {
                        continue
                }
                if len(l) > 200 {
                        l = l[:200] + "…"
                }
                kept = append(kept, l)
        }
        return strings.Join(kept, "\n")
}

// needsNewerEngine reports whether an error text says the engine binary
// predates the model's architecture — the signal for the v1.0.5 self-update.
func needsNewerEngine(err error) bool {
        if err == nil {
                return false
        }
        msg := strings.ToLower(err.Error())
        for _, sig := range []string{
                "unknown model architecture",
                "unknown architecture",
                "unsupported architecture",
                "unknown model type",
                "unrecognized",
        } {
                if strings.Contains(msg, sig) {
                        return true
                }
        }
        return false
}

// updateEngineForModel downloads the newest llama.cpp release (once per app
// run) when a model needs a build newer than the bundled one. Returns true
// when a fresh engine was installed and the caller should retry.
func (s *LlamaServer) updateEngineForModel() bool {
        if s.engineUpdateTried {
                return false
        }
        s.engineUpdateTried = true
        if netcheck.IsOffline() {
                s.logf("model needs a newer engine but the machine is offline — skipping auto-update")
                return false
        }
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
        defer cancel()
        latest, err := updater.LatestTag(ctx)
        if err != nil || latest == "" {
                s.logf("could not find a newer engine: %v", err)
                return false
        }
        current := updater.InstalledEngineTag(s.cfg)
        if current == "" {
                current = updater.DefaultEngineTag
        }
        if latest == current {
                s.logf("engine %s is already the newest release — the model is still not loadable", current)
                return false
        }
        s.logf("this model needs a newer llama.cpp — updating %s → %s", current, latest)
        logging.Default().Info("engine", "auto-updating engine for new model architecture: %s → %s", current, latest)
        s.setState("downloading")
        if _, err := updater.UpdateEngine(ctx, s.cfg, nil, latest); err != nil {
                s.logf("engine auto-update failed: %v", err)
                logging.Default().Warn("engine", "auto-update failed: %v", err)
                return false
        }
        // Fresh binary, fresh chance at the fastest profile.
        s.cfg.EngineCompat = 0
        _ = config.Save(s.cfg.ConfigPath(), s.cfg)
        return true
}

// autoGPUOffload reports whether the engine should be launched with GPU
// layer offload even though NumGPU is 0: the user has not disabled the
// behavior, the bundled engine carries the Vulkan backend, and a GPU was
// detected on this machine (v1.0.3).
func (s *LlamaServer) autoGPUOffload() bool {
        if !s.cfg.GPUAutoOffload {
                return false
        }
        if !s.hasVulkanBackend() {
                return false
        }
        info := sysinfo.Probe()
        return len(info.GPU) > 0
}

// hasVulkanBackend reports whether ggml-vulkan.dll sits next to the server
// binary (the v1.0.3 bundled build ships it; CPU-only installs do not).
func (s *LlamaServer) hasVulkanBackend() bool {
        bin := s.cfg.LlamaBinPath
        if bin == "" {
                bin = filepath.Join(s.cfg.DataDir, "bin", llamaBinaryName())
        }
        if _, err := os.Stat(filepath.Join(filepath.Dir(bin), "ggml-vulkan.dll")); err == nil {
                return true
        }
        // darwin/linux builds name the backend differently
        if runtime.GOOS != "windows" {
                if _, err := os.Stat(filepath.Join(filepath.Dir(bin), "libggml-vulkan.so")); err == nil {
                        return true
                }
        }
        return false
}

// HasVulkanBackendForTest is the exported seam for the stress suite.
func (s *LlamaServer) HasVulkanBackendForTest() bool { return s.hasVulkanBackend() }

// AutoGPUOffloadForTest is the exported seam for the stress suite.
func (s *LlamaServer) AutoGPUOffloadForTest() bool { return s.autoGPUOffload() }

// --- v1.0.5 stress-suite seams ---

// BuildArgsForTest exposes buildArgs (the compatibility-ladder flag set).
func (s *LlamaServer) BuildArgsForTest(modelPath string, level int) []string {
        return s.buildArgs(modelPath, level)
}

// SetProjectorForTest pins the active multimodal projector (v1.0.6 vision
// seam) so the stress suite can assert the --mmproj flag contract.
func (s *LlamaServer) SetProjectorForTest(path string) {
        s.mu.Lock()
        s.mmproj = path
        s.mu.Unlock()
}

// ProjectorPathForTest exposes the active projector path.
func (s *LlamaServer) ProjectorPathForTest() string { return s.ProjectorPath() }

// MakeExitFailureForTest builds an exitFailure with a canned stderr tail.
func MakeExitFailureForTest(err error, tail []string) error {
        return &exitFailure{err: explainExit(err), tail: tail}
}

// IsExitFailureForTest reports whether err is the retry-worthy exit kind.
func IsExitFailureForTest(err error) bool {
        _, ok := err.(*exitFailure)
        return ok
}

// NeedsNewerEngineForTest exposes the auto-update signal check.
func NeedsNewerEngineForTest(err error) bool { return needsNewerEngine(err) }

// CompactLinesForTest exposes the stderr-tail clipper.
func CompactLinesForTest(lines []string, n int) string { return compactLines(lines, n) }

// adoptExisting reports whether the port already answers OUR llama.cpp
// /health endpoint (an orphaned subprocess from a previous run). When true
// the caller marks the engine running without spawning a new process
// (v1.0.3 — the previous behavior was a confusing bind-error).
func (s *LlamaServer) adoptExisting() bool {
        url := fmt.Sprintf("http://%s:%d/health", s.cfg.LlamaHost, s.cfg.LlamaPort)
        client := &http.Client{Timeout: 2 * time.Second}
        resp, err := client.Get(url)
        if err != nil {
                return false
        }
        defer resp.Body.Close()
        return resp.StatusCode == 200
}

// Stop terminates the subprocess.
func (s *LlamaServer) Stop() error {
        s.mu.Lock()
        defer s.mu.Unlock()
        if s.cmd == nil || s.cmd.Process == nil {
                s.state = "stopped"
                return nil
        }
        _ = s.cmd.Process.Signal(syscall.SIGTERM)
        time.Sleep(500 * time.Millisecond)
        _ = s.cmd.Process.Kill()
        s.cmd = nil
        s.loaded = ""
        s.state = "stopped"
        return nil
}

// EnsureRunning is the v1.0.3 auto-start primitive: it boots the engine if
// it is not up yet and waits until it is ready to serve, so the caller can
// fire its first chat the moment the user starts typing->sending. Returns
// nil when the engine is already running.
func (s *LlamaServer) EnsureRunning() error {
        if s.IsRunning() {
                return nil
        }
        return s.Start()
}

// Restart stops the server (if running) and boots it again — used when the
// selected model or engine parameters change at runtime.
func (s *LlamaServer) Restart() error {
        s.switchMu.Lock()
        defer s.switchMu.Unlock()
        if err := s.Stop(); err != nil {
                return err
        }
        // Give the OS a beat to release the listening port.
        time.Sleep(300 * time.Millisecond)
        return s.startLocked()
}

// SwitchModel points the engine at a new GGUF and reloads it if the server
// is currently running. If the server is stopped the new model is simply
// picked up on the next Start(). This is what the model picker calls —
// before v1.0.0 picking a model changed nothing because the running server
// kept serving whatever it booted with.
func (s *LlamaServer) SwitchModel(name string) error {
        s.mu.Lock()
        running := s.state == "running"
        s.mu.Unlock()
        s.cfg.Model = name
        if !running {
                return nil
        }
        return s.Restart()
}

// LoadOrStartWithModel is the v1.0.3 "prewarm" path: remember the chosen
// model and, if the engine is already running, reload it with that model;
// otherwise boot the engine now so the FIRST message answers instantly.
func (s *LlamaServer) LoadOrStartWithModel(name string) error {
        s.switchMu.Lock()
        defer s.switchMu.Unlock()
        s.cfg.Model = name
        if s.IsRunning() {
                if err := s.Stop(); err != nil {
                        return err
                }
                time.Sleep(300 * time.Millisecond)
        }
        return s.startLocked()
}

// LoadedModel returns the absolute path of the model the subprocess is
// currently serving ("" when stopped).
func (s *LlamaServer) LoadedModel() string {
        s.mu.Lock()
        defer s.mu.Unlock()
        return s.loaded
}

func (s *LlamaServer) setState(st string) {
        s.mu.Lock()
        s.state = st
        s.mu.Unlock()
        select {
        case s.stateCh <- st:
        default:
        }
}

func (s *LlamaServer) logf(format string, args ...interface{}) {
        line := fmt.Sprintf(format, args...)
        s.logBuf.add(line)
}

// downloadAndExtract downloads `url` and extracts it into `dir`. Supports
// .zip (windows) and .tar.gz (linux/macos).
func downloadAndExtract(url, dir string) error {
        tmp, err := os.CreateTemp("", "llama-*")
        if err != nil {
                return err
        }
        defer os.Remove(tmp.Name())
        defer tmp.Close()

        resp, err := http.Get(url)
        if err != nil {
                return err
        }
        defer resp.Body.Close()
        if resp.StatusCode != 200 {
                return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
        }
        if _, err := io.Copy(tmp, resp.Body); err != nil {
                return err
        }

        if _, err := tmp.Seek(0, 0); err != nil {
                return err
        }

        switch {
        case strings.HasSuffix(url, ".zip"):
                _, err = tmp.Seek(0, 0)
                if err != nil {
                        return err
                }
                stat, err := tmp.Stat()
                if err != nil {
                        return err
                }
                zr, err := zip.NewReader(tmp, stat.Size())
                if err != nil {
                        return err
                }
                for _, f := range zr.File {
                        out := filepath.Join(dir, f.Name)
                        if f.FileInfo().IsDir() {
                                _ = os.MkdirAll(out, 0o755)
                                continue
                        }
                        _ = os.MkdirAll(filepath.Dir(out), 0o755)
                        rc, err := f.Open()
                        if err != nil {
                                return err
                        }
                        f, err := os.Create(out)
                        if err != nil {
                                _ = rc.Close()
                                return err
                        }
                        _, err = io.Copy(f, rc)
                        _ = rc.Close()
                        _ = f.Close()
                        if err != nil {
                                return err
                        }
                }
        case strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz"):
                if _, err := tmp.Seek(0, 0); err != nil {
                        return err
                }
                gz, err := gzip.NewReader(tmp)
                if err != nil {
                        return err
                }
                defer gz.Close()
                // Use tar command to keep deps minimal (hidden console on
                // Windows — v1.0.4).
                cmd := proc.Command("tar", "-xzf", "-", "-C", dir)
                cmd.Stdin = gz
                if out, err := cmd.CombinedOutput(); err != nil {
                        return fmt.Errorf("tar -xzf: %w: %s", err, out)
                }
        default:
                // Plain binary
                if _, err := tmp.Seek(0, 0); err != nil {
                        return err
                }
                out, err := os.Create(filepath.Join(dir, filepath.Base(url)))
                if err != nil {
                        return err
                }
                defer out.Close()
                if _, err := io.Copy(out, tmp); err != nil {
                        return err
                }
        }
        return nil
}

// ResolveModelPath turns a configured model name into an existing absolute
// GGUF path. Accepted inputs, in priority order:
//  1. an existing absolute or cwd-relative file path
//  2. a file inside modelsDir matching the name exactly (case-insensitive)
//  3. a file inside modelsDir whose name CONTAINS the given string
//  4. when name is empty: the first .gguf in modelsDir
//
// v1.0.0 fix: v0.9 passed a bare filename straight to `--model`, which
// llama-server resolved against ITS working directory (often not the app
// folder), so freshly picked models never loaded.
func ResolveModelPath(modelsDir, name string) (string, error) {
        name = strings.TrimSpace(name)
        if name != "" {
                if abs, err := filepath.Abs(name); err == nil {
                        if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
                                return abs, nil
                        }
                }
        }
        entries, err := os.ReadDir(modelsDir)
        if err != nil {
                if name == "" {
                        return "", fmt.Errorf("models dir %s is empty or missing (drop a .gguf file inside it)", modelsDir)
                }
                return "", fmt.Errorf("cannot read models dir %s: %w", modelsDir, err)
        }
        var files []string
        for _, e := range entries {
                if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".gguf") {
                        continue
                }
                files = append(files, e.Name())
        }
        if len(files) == 0 {
                return "", fmt.Errorf("no .gguf files in %s — drop a model file inside it and try again", modelsDir)
        }
        if name == "" {
                return filepath.Join(modelsDir, files[0]), nil
        }
        // exact (case-insensitive) match first
        for _, f := range files {
                if strings.EqualFold(f, name) {
                        return filepath.Join(modelsDir, f), nil
                }
        }
        // then substring match
        lower := strings.ToLower(name)
        for _, f := range files {
                if strings.Contains(strings.ToLower(f), lower) {
                        return filepath.Join(modelsDir, f), nil
                }
        }
        // then fuzzy match: every alphanumeric TOKEN of the query must appear in
        // the filename ("qwen 7b" matches "qwen2.5-7b-instruct-q5.gguf").
        tokens := fuzzyTokens(name)
        if len(tokens) > 0 {
                for _, f := range files {
                        lf := strings.ToLower(f)
                        all := true
                        for _, tok := range tokens {
                                if !strings.Contains(lf, tok) {
                                        all = false
                                        break
                                }
                        }
                        if all {
                                return filepath.Join(modelsDir, f), nil
                        }
                }
        }
        return "", fmt.Errorf("no .gguf in %s matches %q (have: %s)", modelsDir, name, strings.Join(files, ", "))
}

// fuzzyTokens splits a model query into lowercase alphanumeric tokens.
func fuzzyTokens(name string) []string {
        var toks []string
        for _, f := range strings.Fields(strings.ToLower(name)) {
                var cur strings.Builder
                for _, r := range f {
                        if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
                                cur.WriteRune(r)
                        }
                }
                if cur.Len() > 0 {
                        toks = append(toks, cur.String())
                }
        }
        return toks
}

// llamaBinaryName returns the OS-specific llama.cpp server binary name.
func llamaBinaryName() string {
        switch runtime.GOOS {
        case "windows":
                return "llama-server.exe"
        case "darwin":
                return "llama-server"
        default:
                return "llama-server"
        }
}

// llamaDownloadURL returns the official llama.cpp release URL for the
// current OS/arch (pinned bootstrap build; the updater moves to newer tags).
func llamaDownloadURL() (string, error) {
        tag := updater.DefaultEngineTag
        url := updater.AssetURL(tag)
        if url == "" {
                return "", fmt.Errorf("no prebuilt llama.cpp asset for %s/%s", runtime.GOOS, runtime.GOARCH)
        }
        return url, nil
}

// ListLocalModels returns the .gguf files found in the models dir.
// v1.0.5: extension match is case-insensitive — "MODEL.GGUF" is a valid
// download too and must show up in the picker like any other model.
// v1.0.6: multimodal projectors (mmproj-*.gguf) are NOT chat models — they
// pair with a model through the vision layer and would die with exit code 1
// if the user picked one, so they never appear in the model list.
func ListLocalModels(dir string) []string {
        entries, err := os.ReadDir(dir)
        if err != nil {
                return nil
        }
        var out []string
        for _, e := range entries {
                if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".gguf") {
                        if vision.IsMMProj(e.Name()) {
                                continue
                        }
                        out = append(out, e.Name())
                }
        }
        return out
}

// ListLoadedModels queries the running llama.cpp server's /v1/models endpoint.
func (s *LlamaServer) ListLoadedModels() ([]string, error) {
        if !s.IsRunning() {
                return nil, fmt.Errorf("llama.cpp server not running")
        }
        url := fmt.Sprintf("http://%s:%d/v1/models", s.cfg.LlamaHost, s.cfg.LlamaPort)
        resp, err := http.Get(url)
        if err != nil {
                return nil, err
        }
        defer resp.Body.Close()
        var body struct {
                Data []struct {
                        ID string `json:"id"`
                } `json:"data"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
                return nil, err
        }
        var out []string
        for _, m := range body.Data {
                out = append(out, m.ID)
        }
        return out, nil
}

// PortInUse is a small helper for the UI to show whether a port is occupied.
func PortInUse(host string, port int) bool {
        addr := fmt.Sprintf("%s:%d", host, port)
        l, err := net.Listen("tcp", addr)
        if err != nil {
                return true
        }
        _ = l.Close()
        return false
}

// lineWriter collects stdout/stderr line-by-line and calls `cb` for each line.
type lineWriter struct {
        buf []byte
        cb  func(string)
}

func newLineWriter(cb func(string)) *lineWriter { return &lineWriter{cb: cb} }

func (w *lineWriter) Write(p []byte) (int, error) {
        w.buf = append(w.buf, p...)
        for {
                i := -1
                for j, b := range w.buf {
                        if b == '\n' {
                                i = j
                                break
                        }
                }
                if i < 0 {
                        break
                }
                line := strings.TrimRight(string(w.buf[:i]), "\r")
                w.buf = w.buf[i+1:]
                w.cb(line)
        }
        return len(p), nil
}
