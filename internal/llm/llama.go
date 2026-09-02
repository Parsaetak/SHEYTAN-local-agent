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

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/logging"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/netcheck"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/proc"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sysinfo"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/updater"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/vision"
)

// LlamaServer manages a llama.cpp server subprocess. This is the standalone
// inference engine that replaces LM Studio's local server.
type LlamaServer struct {
	cfg     *config.Config
	cmd     *exec.Cmd
	mu      sync.Mutex
	state   string // "stopped" | "starting" | "running" | "error"
	stateCh chan string
	logBuf  *ringBuffer
	errRing *ringBuffer

	loaded   string // absolute path of the model currently loaded by the subprocess
	switchMu sync.Mutex

	// mmproj is the multimodal projector the engine was launched with.
	// Empty when the engine runs text-only.
	mmproj string

	// engineUpdateTried prevents repeated engine self-updates during one
	// application run.
	engineUpdateTried bool
}

type ringBuffer struct {
	mu   sync.Mutex
	buf  []string
	head int
	size int
}

func newRing(n int) *ringBuffer {
	return &ringBuffer{
		buf:  make([]string, n),
		size: n,
	}
}

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

func (r *ringBuffer) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buf = make([]string, r.size)
	r.head = 0
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

// ensureBinary makes sure the llama.cpp server binary exists.
func (s *LlamaServer) ensureBinary() error {
	if s.cfg.LlamaBinPath == "" {
		s.cfg.LlamaBinPath = filepath.Join(
			s.cfg.DataDir,
			"bin",
			llamaBinaryName(),
		)
	}

	if _, err := os.Stat(s.cfg.LlamaBinPath); err == nil {
		if updater.InstalledEngineTag(s.cfg) == "" {
			updater.RecordEngineTag(s.cfg, updater.DefaultEngineTag)
		}

		return nil
	}

	if netcheck.IsOffline() {
		return fmt.Errorf(
			"llama.cpp server binary missing and you appear to be OFFLINE. "+
				"Reconnect once so the server can be downloaded automatically, "+
				"or place a prebuilt llama-server(.exe) into %s and retry",
			filepath.Dir(s.cfg.LlamaBinPath),
		)
	}

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
		return fmt.Errorf(
			"llama.cpp binary not found at %s after extraction",
			s.cfg.LlamaBinPath,
		)
	}

	_ = os.Chmod(s.cfg.LlamaBinPath, 0o755)
	updater.RecordEngineTag(s.cfg, updater.DefaultEngineTag)

	return nil
}

const modelLoadTimeout = 180 * time.Second
const engineCompatMax = 3

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

// Start boots the llama.cpp server as a subprocess if not already running.
// Serialized on switchMu so a prewarm and a first-send can never race.
func (s *LlamaServer) Start() error {
	s.switchMu.Lock()
	defer s.switchMu.Unlock()

	return s.startLocked()
}

// startLocked is the real boot path; the caller holds switchMu.
//
// The compatibility ladder retries progressively safer launch profiles.
// When the engine reports an unsupported model architecture, the bundled
// llama.cpp engine may be updated once and the ladder retried.
//
// adoptExisting must never be called while s.mu is held. The previous
// implementation could deadlock by attempting to lock s.mu again after
// adopting an existing engine.
func (s *LlamaServer) startLocked() error {
	s.mu.Lock()

	if s.state == "running" || s.state == "starting" {
		s.mu.Unlock()
		return nil
	}

	portInUse := PortInUse(
		s.cfg.LlamaHost,
		s.cfg.LlamaPort,
	)

	s.mu.Unlock()

	if portInUse {
		if s.adoptExisting() {
			s.setState("running")
			return nil
		}

		s.setState("error")

		return fmt.Errorf(
			"port %d is already in use by another program — "+
				"change LlamaPort in config.json or close the other app",
			s.cfg.LlamaPort,
		)
	}

	s.setState("starting")

	if err := s.ensureBinary(); err != nil {
		s.setState("error")
		return err
	}

	modelPath, err := ResolveModelPath(
		s.cfg.ModelsDir,
		s.cfg.Model,
	)
	if err != nil {
		s.setState("error")
		return err
	}

	mmproj := ""

	if s.cfg.VisionEnabled {
		if p := vision.FindProjector(
			s.cfg.ModelsDir,
			modelPath,
			s.cfg.VisionMMProj,
		); p != "" {
			mmproj = p
			s.logf(
				"vision projector paired: %s",
				filepath.Base(p),
			)
		}
	}

	s.mu.Lock()
	s.mmproj = mmproj
	s.mu.Unlock()

	visionRetryDone := false

	for {
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
						_ = config.Save(
							s.cfg.ConfigPath(),
							s.cfg,
						)
					}

					if level > 0 {
						s.logf(
							"engine started in compatibility mode %d (%s) — some speed flags disabled",
							level,
							compatLevelName(level),
						)

						logging.Default().Warn(
							"engine",
							"started in compatibility mode %d (%s) for %s",
							level,
							compatLevelName(level),
							filepath.Base(modelPath),
						)
					}

					s.setState("running")
					return nil
				}

				lastErr = err

				logging.Default().Error(
					"engine",
					"startup attempt failed (compat %d, %s): %v",
					level,
					compatLevelName(level),
					err,
				)

				if _, died := err.(*exitFailure); !died {
					s.setState("error")
					return err
				}

				time.Sleep(250 * time.Millisecond)
			}

			if pass == 0 &&
				needsNewerEngine(lastErr) &&
				s.updateEngineForModel() {
				startLevel = 0
				s.setState("starting")
				continue
			}

			break
		}

		if mmproj != "" && !visionRetryDone {
			visionRetryDone = true

			projectName := filepath.Base(mmproj)
			mmproj = ""

			s.mu.Lock()
			s.mmproj = ""
			s.mu.Unlock()

			s.logf(
				"all profiles failed with the vision projector — retrying text-only",
			)

			logging.Default().Warn(
				"engine",
				"vision projector %s failed with every profile; restarting without vision",
				projectName,
			)

			s.setState("starting")
			continue
		}

		s.setState("error")
		return lastErr
	}
}

// VisionActive reports whether the running engine carries a multimodal
// projector.
func (s *LlamaServer) VisionActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.state == "running" && s.mmproj != ""
}

// ProjectorPath returns the active projector file.
func (s *LlamaServer) ProjectorPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.mmproj
}

// Pid returns the engine subprocess pid.
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
//	0 — everything: Speed Pack flags, GPU offload, user extra args
//	1 — everything + --jinja
//	2 — --jinja + GPU, but no speed flags
//	3 — bare: model, host/port, context, threads
func (s *LlamaServer) buildArgs(
	modelPath string,
	level int,
) []string {
	base := []string{
		"--model",
		modelPath,
		"--host",
		s.cfg.LlamaHost,
		"--port",
		fmt.Sprintf("%d", s.cfg.LlamaPort),
		"--ctx-size",
		fmt.Sprintf("%d", s.cfg.LLM.NumCtx),
		"--batch-size",
		fmt.Sprintf("%d", s.cfg.LLM.NumBatch),
		"--threads",
		fmt.Sprintf("%d", threadsFor(s.cfg)),
		"--temp",
		fmt.Sprintf("%v", s.cfg.LLM.Temperature),
		"--top-p",
		fmt.Sprintf("%v", s.cfg.LLM.TopP),
		"--top-k",
		fmt.Sprintf("%d", s.cfg.LLM.TopK),
		"--repeat-penalty",
		fmt.Sprintf("%v", s.cfg.LLM.RepeatPenalty),
	}

	s.mu.Lock()
	mmproj := s.mmproj
	s.mu.Unlock()

	if mmproj != "" {
		base = append(base, "--mmproj", mmproj)
	}

	if s.cfg.LLM.Mirostat > 0 {
		base = append(
			base,
			"--mirostat",
			fmt.Sprintf("%d", s.cfg.LLM.Mirostat),
		)
	}

	if s.cfg.LLM.Seed != 0 {
		base = append(
			base,
			"--seed",
			fmt.Sprintf("%d", s.cfg.LLM.Seed),
		)
	}

	if level == engineCompatMax {
		return base
	}

	gpuLayers := s.cfg.LLM.NumGPU

	if gpuLayers <= 0 && s.autoGPUOffload() {
		gpuLayers = 99
	}

	if gpuLayers > 0 {
		base = append(
			base,
			"--n-gpu-layers",
			fmt.Sprintf("%d", gpuLayers),
		)
	}

	if level <= 1 {
		base = append(base, SpeedArgs(s.cfg)...)
	} else {
		base = append(base, "--no-webui")
	}

	if level >= 1 {
		base = append(base, "--jinja")
	}

	if level <= 2 &&
		strings.TrimSpace(s.cfg.LlamaExtraArgs) != "" {
		base = append(
			base,
			strings.Fields(s.cfg.LlamaExtraArgs)...,
		)
	}

	return base
}

type procExit struct {
	done chan struct{}
	err  error
}

// launchOnce spawns the server with one profile's flags and waits until it
// is HTTP-ready.
func (s *LlamaServer) launchOnce(
	modelPath string,
	level int,
) error {
	if level > 0 {
		s.logf(
			"launching with compatibility profile %d (%s)…",
			level,
			compatLevelName(level),
		)
	}

	args := s.buildArgs(modelPath, level)

	cmd := proc.Command(
		s.cfg.LlamaBinPath,
		args...,
	)

	cmd.Dir = s.cfg.DataDir

	cmd.Stdout = newLineWriter(func(line string) {
		s.logf("[llama.cpp] %s", line)
	})

	s.errRing.reset()

	cmd.Stderr = newLineWriter(func(line string) {
		s.logf("[llama.cpp!] %s", line)
		s.errRing.add(line)
	})

	proc.Hide(cmd)

	if err := cmd.Start(); err != nil {
		return &exitFailure{
			err: fmt.Errorf("start llama.cpp: %w", err),
		}
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	exit := &procExit{
		done: make(chan struct{}),
	}

	go func() {
		exit.err = cmd.Wait()

		s.mu.Lock()

		latest := s.cmd == cmd

		if latest {
			s.cmd = nil
		}

		wasRunning :=
			latest &&
				s.loaded == modelPath &&
				s.state == "running"

		if wasRunning {
			s.loaded = ""
		}

		s.mu.Unlock()

		if wasRunning {
			s.setState("stopped")

			logging.Default().Error(
				"engine",
				"engine exited while running: %v",
				exit.err,
			)
		}

		close(exit.done)
	}()

	if err := s.waitReadySignaled(
		modelLoadTimeout,
		exit,
	); err != nil {
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
// the subprocess exits or the timeout passes.
func (s *LlamaServer) waitReadySignaled(
	timeout time.Duration,
	exit *procExit,
) error {
	deadline := time.Now().Add(timeout)

	url := fmt.Sprintf(
		"http://%s:%d/health",
		s.cfg.LlamaHost,
		s.cfg.LlamaPort,
	)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	for time.Now().Before(deadline) {
		if resp, err := client.Get(url); err == nil {
			_ = resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
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

	msg := fmt.Sprintf(
		"llama.cpp did not become ready within %v "+
			"(the model may be too large for this machine)",
		timeout,
	)

	if tail != "" {
		msg += "\n\nEngine output (last lines):\n" + tail
	}

	return fmt.Errorf("%s", msg)
}

func (s *LlamaServer) exitError(err error) error {
	return &exitFailure{
		err:  explainExit(err),
		tail: s.errRing.lines(),
	}
}

type exitFailure struct {
	err  error
	tail []string
}

func explainExit(err error) error {
	if err == nil {
		return fmt.Errorf("llama.cpp exited during startup")
	}

	msg := err.Error()

	if strings.Contains(msg, "3221225781") ||
		strings.Contains(msg, "0xc0000135") {
		return fmt.Errorf(
			"llama.cpp could not start: a required Windows DLL is missing. "+
				"Install the free Microsoft Visual C++ Redistributable (64-bit) "+
				"from https://aka.ms/vs/17/release/vc_redist.x64.exe "+
				"and start again: %v",
			err,
		)
	}

	return fmt.Errorf(
		"llama.cpp exited during startup: %v",
		err,
	)
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

func (e *exitFailure) Unwrap() error {
	return e.err
}

func (s *LlamaServer) tailLines(n int) string {
	return compactLines(s.errRing.lines(), n)
}

func compactLines(lines []string, n int) string {
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	var kept []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if len(line) > 200 {
			line = line[:200] + "…"
		}

		kept = append(kept, line)
	}

	return strings.Join(kept, "\n")
}

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

func (s *LlamaServer) updateEngineForModel() bool {
	if s.engineUpdateTried {
		return false
	}

	s.engineUpdateTried = true

	if netcheck.IsOffline() {
		s.logf(
			"model needs a newer engine but the machine is offline — skipping auto-update",
		)
		return false
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Minute,
	)
	defer cancel()

	latest, err := updater.LatestTag(ctx)
	if err != nil || latest == "" {
		s.logf(
			"could not find a newer engine: %v",
			err,
		)
		return false
	}

	current := updater.InstalledEngineTag(s.cfg)

	if current == "" {
		current = updater.DefaultEngineTag
	}

	if latest == current {
		s.logf(
			"engine %s is already the newest release — "+
				"the model is still not loadable",
			current,
		)
		return false
	}

	s.logf(
		"this model needs a newer llama.cpp — updating %s → %s",
		current,
		latest,
	)

	logging.Default().Info(
		"engine",
		"auto-updating engine for new model architecture: %s → %s",
		current,
		latest,
	)

	s.setState("downloading")

	if _, err := updater.UpdateEngine(
		ctx,
		s.cfg,
		nil,
		latest,
	); err != nil {
		s.logf(
			"engine auto-update failed: %v",
			err,
		)

		logging.Default().Warn(
			"engine",
			"auto-update failed: %v",
			err,
		)

		return false
	}

	s.cfg.EngineCompat = 0
	_ = config.Save(
		s.cfg.ConfigPath(),
		s.cfg,
	)

	return true
}

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

func (s *LlamaServer) hasVulkanBackend() bool {
	bin := s.cfg.LlamaBinPath

	if bin == "" {
		bin = filepath.Join(
			s.cfg.DataDir,
			"bin",
			llamaBinaryName(),
		)
	}

	if _, err := os.Stat(
		filepath.Join(
			filepath.Dir(bin),
			"ggml-vulkan.dll",
		),
	); err == nil {
		return true
	}

	if runtime.GOOS != "windows" {
		if _, err := os.Stat(
			filepath.Join(
				filepath.Dir(bin),
				"libggml-vulkan.so",
			),
		); err == nil {
			return true
		}
	}

	return false
}

func (s *LlamaServer) HasVulkanBackendForTest() bool {
	return s.hasVulkanBackend()
}

func (s *LlamaServer) AutoGPUOffloadForTest() bool {
	return s.autoGPUOffload()
}

func (s *LlamaServer) BuildArgsForTest(
	modelPath string,
	level int,
) []string {
	return s.buildArgs(modelPath, level)
}

func (s *LlamaServer) SetProjectorForTest(path string) {
	s.mu.Lock()
	s.mmproj = path
	s.mu.Unlock()
}

func (s *LlamaServer) ProjectorPathForTest() string {
	return s.ProjectorPath()
}

func MakeExitFailureForTest(
	err error,
	tail []string,
) error {
	return &exitFailure{
		err:  explainExit(err),
		tail: tail,
	}
}

func IsExitFailureForTest(err error) bool {
	_, ok := err.(*exitFailure)
	return ok
}

func NeedsNewerEngineForTest(err error) bool {
	return needsNewerEngine(err)
}

func CompactLinesForTest(
	lines []string,
	n int,
) string {
	return compactLines(lines, n)
}

// adoptExisting reports whether the port already answers our llama.cpp
// /health endpoint.
func (s *LlamaServer) adoptExisting() bool {
	url := fmt.Sprintf(
		"http://%s:%d/health",
		s.cfg.LlamaHost,
		s.cfg.LlamaPort,
	)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return false
	}

	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
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

// EnsureRunning boots the engine if it is not already running.
func (s *LlamaServer) EnsureRunning() error {
	if s.IsRunning() {
		return nil
	}

	return s.Start()
}

// Restart stops the server and boots it again.
func (s *LlamaServer) Restart() error {
	s.switchMu.Lock()
	defer s.switchMu.Unlock()

	if err := s.Stop(); err != nil {
		return err
	}

	time.Sleep(300 * time.Millisecond)

	return s.startLocked()
}

// SwitchModel points the engine at a new GGUF and reloads it if running.
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

// LoadOrStartWithModel remembers the chosen model and starts/reloads it.
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

// LoadedModel returns the absolute path of the model currently served.
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

func (s *LlamaServer) logf(
	format string,
	args ...interface{},
) {
	line := fmt.Sprintf(format, args...)
	s.logBuf.add(line)
}

// safeArchivePath validates an archive member name and returns a path
// guaranteed to remain inside dir.
//
// Archive entry names are normalized to forward slashes first so Windows
// backslash traversal is treated exactly like Unix-style traversal.
//
// Examples rejected:
//   - ../../outside.exe
//   - ../outside.exe
//   - /absolute/path
//   - \\server\share\file
//   - C:\outside.exe
//   - C:/outside.exe
//
// Examples accepted:
//   - llama-server.exe
//   - bin/llama-server.exe
func safeArchivePath(dir, name string) (string, error) {
	name = strings.TrimSpace(name)

	if name == "" {
		return "", fmt.Errorf("archive contains an empty path")
	}

	normalized := strings.ReplaceAll(name, "\\", "/")
	cleanName := filepath.Clean(filepath.FromSlash(normalized))

	if cleanName == "." ||
		cleanName == string(filepath.Separator) ||
		cleanName == "" {
		return "", fmt.Errorf(
			"archive contains invalid path %q",
			name,
		)
	}

	if filepath.IsAbs(cleanName) ||
		filepath.VolumeName(cleanName) != "" {
		return "", fmt.Errorf(
			"archive path %q is absolute or contains a volume",
			name,
		)
	}

	base, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf(
			"resolve archive destination: %w",
			err,
		)
	}

	target := filepath.Join(base, cleanName)

	relative, err := filepath.Rel(base, target)
	if err != nil {
		return "", fmt.Errorf(
			"validate archive path %q: %w",
			name,
			err,
		)
	}

	if relative == ".." ||
		strings.HasPrefix(
			relative,
			".."+string(filepath.Separator),
		) {
		return "", fmt.Errorf(
			"archive path %q escapes extraction directory",
			name,
		)
	}

	return target, nil
}

// downloadAndExtract downloads url and extracts it into dir.
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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"download %s: HTTP %d",
			url,
			resp.StatusCode,
		)
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return err
	}

	if _, err := tmp.Seek(0, 0); err != nil {
		return err
	}

	switch {
	case strings.HasSuffix(url, ".zip"):
		stat, err := tmp.Stat()
		if err != nil {
			return err
		}

		zr, err := zip.NewReader(tmp, stat.Size())
		if err != nil {
			return err
		}

		for _, f := range zr.File {
			out, err := safeArchivePath(
				dir,
				f.Name,
			)
			if err != nil {
				return err
			}

			if f.FileInfo().IsDir() {
				if err := os.MkdirAll(
					out,
					0o755,
				); err != nil {
					return err
				}

				continue
			}

			if err := os.MkdirAll(
				filepath.Dir(out),
				0o755,
			); err != nil {
				return err
			}

			rc, err := f.Open()
			if err != nil {
				return err
			}

			outFile, err := os.Create(out)
			if err != nil {
				_ = rc.Close()
				return err
			}

			_, copyErr := io.Copy(
				outFile,
				rc,
			)

			closeErr := outFile.Close()
			rcErr := rc.Close()

			if copyErr != nil {
				return copyErr
			}

			if closeErr != nil {
				return closeErr
			}

			if rcErr != nil {
				return rcErr
			}
		}

	case strings.HasSuffix(url, ".tar.gz"),
		strings.HasSuffix(url, ".tgz"):
		if _, err := tmp.Seek(0, 0); err != nil {
			return err
		}

		gz, err := gzip.NewReader(tmp)
		if err != nil {
			return err
		}

		defer gz.Close()

		cmd := proc.Command(
			"tar",
			"-xzf",
			"-",
			"-C",
			dir,
		)

		cmd.Stdin = gz

		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf(
				"tar -xzf: %w: %s",
				err,
				out,
			)
		}

	default:
		if _, err := tmp.Seek(0, 0); err != nil {
			return err
		}

		out, err := os.Create(
			filepath.Join(
				dir,
				filepath.Base(url),
			),
		)
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
// GGUF path.
func ResolveModelPath(
	modelsDir,
	name string,
) (string, error) {
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
			return "",
				fmt.Errorf(
					"models dir %s is empty or missing "+
						"(drop a .gguf file inside it)",
					modelsDir,
				)
		}

		return "",
			fmt.Errorf(
				"cannot read models dir %s: %w",
				modelsDir,
				err,
			)
	}

	var files []string

	for _, e := range entries {
		if e.IsDir() ||
			!strings.HasSuffix(
				strings.ToLower(e.Name()),
				".gguf",
			) {
			continue
		}

		files = append(files, e.Name())
	}

	if len(files) == 0 {
		return "",
			fmt.Errorf(
				"no .gguf files in %s — drop a model file inside it and try again",
				modelsDir,
			)
	}

	if name == "" {
		return filepath.Join(modelsDir, files[0]), nil
	}

	for _, f := range files {
		if strings.EqualFold(f, name) {
			return filepath.Join(modelsDir, f), nil
		}
	}

	lower := strings.ToLower(name)

	for _, f := range files {
		if strings.Contains(
			strings.ToLower(f),
			lower,
		) {
			return filepath.Join(modelsDir, f), nil
		}
	}

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

	return "",
		fmt.Errorf(
			"no .gguf in %s matches %q (have: %s)",
			modelsDir,
			name,
			strings.Join(files, ", "),
		)
}

func fuzzyTokens(name string) []string {
	var toks []string

	for _, field := range strings.Fields(
		strings.ToLower(name),
	) {
		var cur strings.Builder

		for _, r := range field {
			if (r >= 'a' && r <= 'z') ||
				(r >= '0' && r <= '9') {
				cur.WriteRune(r)
			}
		}

		if cur.Len() > 0 {
			toks = append(toks, cur.String())
		}
	}

	return toks
}

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

func llamaDownloadURL() (string, error) {
	tag := updater.DefaultEngineTag
	url := updater.AssetURL(tag)

	if url == "" {
		return "",
			fmt.Errorf(
				"no prebuilt llama.cpp asset for %s/%s",
				runtime.GOOS,
				runtime.GOARCH,
			)
	}

	return url, nil
}

func ListLocalModels(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var out []string

	for _, e := range entries {
		if e.IsDir() ||
			!strings.HasSuffix(
				strings.ToLower(e.Name()),
				".gguf",
			) {
			continue
		}

		if vision.IsMMProj(e.Name()) {
			continue
		}

		out = append(out, e.Name())
	}

	return out
}

func (s *LlamaServer) ListLoadedModels() ([]string, error) {
	if !s.IsRunning() {
		return nil,
			fmt.Errorf("llama.cpp server not running")
	}

	url := fmt.Sprintf(
		"http://%s:%d/v1/models",
		s.cfg.LlamaHost,
		s.cfg.LlamaPort,
	)

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

	if err := json.NewDecoder(
		resp.Body,
	).Decode(&body); err != nil {
		return nil, err
	}

	var out []string

	for _, model := range body.Data {
		out = append(out, model.ID)
	}

	return out, nil
}

func PortInUse(host string, port int) bool {
	addr := fmt.Sprintf(
		"%s:%d",
		host,
		port,
	)

	l, err := net.Listen(
		"tcp",
		addr,
	)
	if err != nil {
		return true
	}

	_ = l.Close()
	return false
}

type lineWriter struct {
	buf []byte
	cb  func(string)
}

func newLineWriter(
	cb func(string),
) *lineWriter {
	return &lineWriter{
		cb: cb,
	}
}

func (w *lineWriter) Write(
	p []byte,
) (int, error) {
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

		line := strings.TrimRight(
			string(w.buf[:i]),
			"\r",
		)

		w.buf = w.buf[i+1:]
		w.cb(line)
	}

	return len(p), nil
}
