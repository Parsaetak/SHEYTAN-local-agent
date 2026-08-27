package cmd

// v1.0.3 stress tests: bundled-engine bookkeeping, the fixed engine updater
// (latest tag with REAL prebuilt assets, forced checks), the multi-strategy
// connectivity probe, GPU auto-offload gating, and the no-phantom-model
// config defaults.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sheytan/local-agent/internal/aicontext"
	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/llm"
	"github.com/sheytan/local-agent/internal/netcheck"
	"github.com/sheytan/local-agent/internal/updater"
)

// stressV103Defaults locks in the v1.0.3 config surface: NO phantom default
// model (the "gemma-4" entry that never matched a real file), GPU auto-
// offload on by default, and DisplayModel stripping the .gguf suffix.
func stressV103Defaults() error {
	cfg := config.Default()
	if cfg.Model != "" {
		return fmt.Errorf("Default().Model = %q, want \"\" (no phantom model)", cfg.Model)
	}
	if !cfg.GPUAutoOffload {
		return fmt.Errorf("Default().GPUAutoOffload = false, want true")
	}
	cfg.Model = "qwen2.5-7b-instruct-q5_k_m.gguf"
	if got := cfg.DisplayModel(); got != "qwen2.5-7b-instruct-q5_k_m" {
		return fmt.Errorf("DisplayModel() = %q, want qwen2.5-7b-instruct-q5_k_m", got)
	}
	cfg.Model = ""
	if got := cfg.DisplayModel(); got != "" {
		return fmt.Errorf("DisplayModel() on empty = %q, want \"\"", got)
	}
	// v1.0.4: the version lock moved to stressV104Defaults; the v1.0.3
	// behavioral contract (no phantom model, GPU auto-offload) stays here.
	return nil
}

// stressUpdaterPicksAssetBearingTag proves the v1.0.3 updater fix: when the
// newest GitHub release carries NO prebuilt binaries (milestone tags like
// v0.3.0), the updater walks down the list to the newest release that
// actually ships one instead of failing on a 404.
func stressUpdaterPicksAssetBearingTag() error {
	asset := func(tag string) string { return updater.AssetName(tag) }
	got := updater.FirstWithAsset([]updater.ReleaseInfo{
		{TagName: "v0.3.0", Assets: []updater.AssetInfo{{Name: "source.zip"}}},
		{TagName: "b10642", Assets: []updater.AssetInfo{{Name: asset("b10642")}, {Name: "source.tar.gz"}}},
		{TagName: "b10640", Assets: []updater.AssetInfo{{Name: asset("b10640")}}},
	})
	if got != "b10642" {
		return fmt.Errorf("FirstWithAsset = %q, want b10642 (v0.3.0 has no binaries)", got)
	}
	// Every release binary-less → empty.
	if got := updater.FirstWithAsset([]updater.ReleaseInfo{
		{TagName: "v0.4.0", Assets: nil},
	}); got != "" {
		return fmt.Errorf("FirstWithAsset on binary-less list = %q, want \"\"", got)
	}
	// Asset URLs stay well-formed for the download path.
	u := updater.AssetURL("b10642")
	if u == "" || !strings.Contains(u, "releases/download/b10642/") {
		return fmt.Errorf("AssetURL(b10642) = %q, malformed", u)
	}
	return nil
}

// stressUpdaterForcedBypassesGate proves the "Check now" fix: a forced check
// ignores the last-check timestamp (before v1.0.3 the button hit the due
// gate and did nothing).
func stressUpdaterForcedBypassesGate() error {
	cfg := config.Default()
	cfg.UpdateSchedule = "daily"
	cfg.LastUpdateCheck = time.Now().UTC().Format(time.RFC3339) // checked seconds ago

	netcheck.SetProbe(func() bool { return false }) // offline → no network calls
	defer netcheck.SetProbe(nil)

	// Non-forced: gated off by the fresh last-check.
	msg, _, err := updater.CheckAndApply(context.Background(), cfg, nil)
	if err != nil || !strings.Contains(msg, "up to date") {
		return fmt.Errorf("scheduled pass with fresh check = (%q, %v), want gated", msg, err)
	}
	// Forced: bypasses the gate and actually checks (offline here).
	msg, _, err = updater.CheckAndApplyForced(context.Background(), cfg, nil)
	if err != nil {
		return fmt.Errorf("forced check errored: %v", err)
	}
	if !strings.Contains(msg, "offline") {
		return fmt.Errorf("forced check msg = %q, want it to have actually checked (offline)", msg)
	}
	return nil
}

// stressNetcheckMultiProbe exercises the multi-strategy probe state machine:
// Force bypasses the cache, the TTL cache works, and state flips both ways
// (the stuck-OFFLINE bug).
func stressNetcheckMultiProbe() error {
	state := false
	netcheck.SetProbe(func() bool { return state })

	if netcheck.Online() {
		return fmt.Errorf("Online() with offline probe = true")
	}
	// Flip to online: Force must see it immediately (the v1.0.2 bug kept
	// the pill OFFLINE after the machine reconnected).
	state = true
	if !netcheck.Force() {
		return fmt.Errorf("Force() after reconnect = false, want true")
	}
	if !netcheck.Online() {
		return fmt.Errorf("Online() after reconnect = false, want true")
	}
	if netcheck.IsOffline() {
		return fmt.Errorf("IsOffline() after reconnect = true")
	}
	// And back offline.
	state = false
	if netcheck.Force() {
		return fmt.Errorf("Force() after disconnect = true, want false")
	}
	if netcheck.Note() == "" {
		return fmt.Errorf("Note() offline = empty, want guidance")
	}
	netcheck.SetProbe(nil)
	return nil
}

// stressVulkanDetect tests the GPU auto-offload gate: it requires BOTH the
// Vulkan backend dll next to the engine binary AND the user's consent.
func stressVulkanDetect() error {
	dir, _ := os.MkdirTemp("", "sheytan-vk-*")
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.ModelsDir = filepath.Join(dir, "models")
	cfg.LlamaBinPath = filepath.Join(dir, "bin", "llama-server")
	_ = os.MkdirAll(filepath.Dir(cfg.LlamaBinPath), 0o755)

	srv := llm.NewLlamaServer(cfg)
	if srv.HasVulkanBackendForTest() {
		return fmt.Errorf("HasVulkanBackend with no dll = true")
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(cfg.LlamaBinPath), "ggml-vulkan.dll"), []byte("x"), 0o644)
	if !srv.HasVulkanBackendForTest() {
		return fmt.Errorf("HasVulkanBackend with dll = false")
	}
	// GPUAutoOffload=false must disable the offload even with the dll.
	cfg.GPUAutoOffload = false
	if srv.AutoGPUOffloadForTest() {
		return fmt.Errorf("autoGPUOffload with consent=false = true")
	}
	cfg.GPUAutoOffload = true
	// (On the Linux CI box sysinfo finds no GPU, so AutoGPUOffloadForTest()
	// may be false here — the gate ordering is what we assert above.)
	return nil
}

// stressEngineMissingModelOffline proves the engine fails FAST and clearly
// when there is no model and no connectivity (no multi-minute hang).
func stressEngineMissingModelOffline() error {
	dir, _ := os.MkdirTemp("", "sheytan-nomodel-*")
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.ModelsDir = filepath.Join(dir, "models")
	cfg.LlamaBinPath = filepath.Join(dir, "bin", "llama-server")
	_ = os.MkdirAll(cfg.ModelsDir, 0o755)

	netcheck.SetProbe(func() bool { return false })
	defer netcheck.SetProbe(nil)

	srv := llm.NewLlamaServer(cfg)
	err := srv.EnsureRunning()
	if err == nil {
		return fmt.Errorf("EnsureRunning with no model + offline = nil, want error")
	}
	if !strings.Contains(err.Error(), "OFFLINE") && !strings.Contains(err.Error(), "models") {
		return fmt.Errorf("error not actionable: %v", err)
	}
	return nil
}

// stressEngineTagRecordedOnBundle proves a bundled engine (binary present)
// gets its build tag recorded in installed.json on first touch so the
// updater knows what it is replacing.
func stressEngineTagRecordedOnBundle() error {
	dir, _ := os.MkdirTemp("", "sheytan-tag-*")
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.ModelsDir = filepath.Join(dir, "models")
	cfg.LlamaBinPath = filepath.Join(dir, "bin", "llama-server.exe")
	_ = os.MkdirAll(filepath.Dir(cfg.LlamaBinPath), 0o755)
	_ = os.WriteFile(cfg.LlamaBinPath, []byte("stub"), 0o755)

	if tag := updater.InstalledEngineTag(cfg); tag != "" {
		return fmt.Errorf("tag before record = %q, want empty", tag)
	}
	// Touch the binary through the manager (ensureBinary path via Start is
	// heavy; call the exported recorder the same way ensureBinary does).
	updater.RecordEngineTag(cfg, updater.DefaultEngineTag)
	if tag := updater.InstalledEngineTag(cfg); tag != updater.DefaultEngineTag {
		return fmt.Errorf("tag after record = %q, want %s", tag, updater.DefaultEngineTag)
	}
	if updater.DefaultEngineTag == "" || !strings.HasPrefix(updater.DefaultEngineTag, "b") {
		return fmt.Errorf("DefaultEngineTag = %q, want a b-tag", updater.DefaultEngineTag)
	}
	return nil
}

// stressAIContextV3 ensures the AI instruction file generation bumped to
// version 3 (bundled engine + auto-start notes). v1.0.4 moved the live
// version lock to stressAIContextV4; this test keeps the v3 upgrade path.
func stressAIContextV3() error {
	// v1.0.5 raised the instruction file to version 5; the contract
	// here is "at least the v4 bundled-engine edition".
	if aicontext.ContextVersion < 4 {
		return fmt.Errorf("ContextVersion = %d, want >= 4", aicontext.ContextVersion)
	}
	dir, _ := os.MkdirTemp("", "sheytan-ctx-*")
	defer os.RemoveAll(dir)
	if _, err := aicontext.EnsureFile(dir); err != nil {
		return fmt.Errorf("EnsureFile: %v", err)
	}
	data, err := os.ReadFile(aicontext.Path(dir))
	if err != nil {
		return fmt.Errorf("read AI-CONTEXT.md: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, fmt.Sprintf("sheytan-context-version: %d", aicontext.ContextVersion)) {
		return fmt.Errorf("marker does not match ContextVersion %d", aicontext.ContextVersion)
	}
	if !strings.Contains(body, "auto-started") {
		return fmt.Errorf("bundled-engine note missing")
	}
	// Regeneration: an outdated v2 file is replaced.
	_ = os.WriteFile(aicontext.Path(dir), []byte("<!-- sheytan-context-version: 1 --> old"), 0o644)
	if _, err := aicontext.EnsureFile(dir); err != nil {
		return fmt.Errorf("EnsureFile upgrade: %v", err)
	}
	data, _ = os.ReadFile(aicontext.Path(dir))
	if !strings.Contains(string(data), fmt.Sprintf("sheytan-context-version: %d", aicontext.ContextVersion)) {
		return fmt.Errorf("outdated marker was not regenerated to ContextVersion %d", aicontext.ContextVersion)
	}
	return nil
}
