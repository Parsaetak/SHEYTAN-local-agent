package cmd

// v1.0.0 stress tests: model-path resolution (the "picked model does
// nothing" fix), engine model switching, the scheduled updater, and the new
// config fields.
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/llm"
	"github.com/sheytan/local-agent/internal/updater"
)

func stressResolveModelPath() error {
	dir := tTempDir("models")
	defer os.RemoveAll(dir)
	mk := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("gguf"), 0o644); err != nil {
			panic(err)
		}
	}
	mk("gemma-3-4b-it-q4_k_m.gguf")
	mk("qwen2.5-7b-instruct-q5.gguf")

	// 1. exact bare filename resolves into the models dir (THE v0.9 bug:
	//    bare filenames were passed to --model where the subprocess could
	//    not resolve them).
	p, err := llm.ResolveModelPath(dir, "gemma-3-4b-it-q4_k_m.gguf")
	if err != nil {
		return fmt.Errorf("exact filename: %v", err)
	}
	if filepath.Base(p) != "gemma-3-4b-it-q4_k_m.gguf" || !filepath.IsAbs(p) {
		return fmt.Errorf("exact filename resolved to %q", p)
	}

	// 2. substring match, case-insensitive.
	p, err = llm.ResolveModelPath(dir, "QWEN 7B")
	if err != nil || !strings.Contains(filepath.Base(p), "qwen2.5-7b") {
		return fmt.Errorf("substring match: %v %q", err, p)
	}

	// 3. empty name picks the first model instead of erroring.
	if p, err = llm.ResolveModelPath(dir, ""); err != nil || !strings.HasSuffix(p, ".gguf") {
		return fmt.Errorf("empty name: %v %q", err, p)
	}

	// 4. an existing absolute path is honored as-is.
	abs := filepath.Join(dir, "qwen2.5-7b-instruct-q5.gguf")
	if p, err = llm.ResolveModelPath(dir, abs); err != nil || p != abs {
		return fmt.Errorf("absolute path: %v %q", err, p)
	}

	// 5. no match -> actionable error naming what IS available.
	_, err = llm.ResolveModelPath(dir, "llama-99.gguf")
	if err == nil || !strings.Contains(err.Error(), "gemma-3-4b") {
		return fmt.Errorf("expected actionable no-match error, got %v", err)
	}

	// 6. empty models dir -> teaching error.
	empty := tTempDir("models-empty")
	defer os.RemoveAll(empty)
	if _, err := llm.ResolveModelPath(empty, ""); err == nil || !strings.Contains(err.Error(), "drop a model") {
		return fmt.Errorf("expected teaching error for empty dir, got %v", err)
	}
	return nil
}

func stressSwitchModelStopped() error {
	dir := tTempDir("switch")
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.ModelsDir = filepath.Join(dir, "models")
	_ = os.MkdirAll(cfg.ModelsDir, 0o755)
	_ = os.WriteFile(filepath.Join(cfg.ModelsDir, "m-a.gguf"), []byte("gguf"), 0o644)
	_ = os.WriteFile(filepath.Join(cfg.ModelsDir, "m-b.gguf"), []byte("gguf"), 0o644)

	srv := llm.NewLlamaServer(cfg)
	// Engine stopped: SwitchModel only records the choice (no boot attempt).
	if err := srv.SwitchModel("m-b.gguf"); err != nil {
		return fmt.Errorf("switch while stopped: %v", err)
	}
	if cfg.Model != "m-b.gguf" {
		return fmt.Errorf("cfg.Model = %q after switch", cfg.Model)
	}
	if srv.LoadedModel() != "" {
		return fmt.Errorf("LoadedModel should be empty while stopped")
	}
	return nil
}

func stressUpdaterSchedule() error {
	now := time.Now()
	cases := []struct {
		schedule string
		last     time.Time
		want     bool
	}{
		{"daily", now.Add(-2 * time.Hour), false},
		{"daily", now.Add(-25 * time.Hour), true},
		{"weekly", now.Add(-2 * 24 * time.Hour), false},
		{"weekly", now.Add(-8 * 24 * time.Hour), true},
		{"monthly", now.Add(-20 * 24 * time.Hour), false},
		{"monthly", now.Add(-31 * 24 * time.Hour), true},
		{"off", time.Time{}, false},
		{"off", now.Add(-365 * 24 * time.Hour), false},
		{"", now.Add(-25 * time.Hour), true},            // default = daily
		{"weird-input", now.Add(-25 * time.Hour), true}, // unknown -> default
	}
	for _, c := range cases {
		if got := updater.CheckDue(c.schedule, c.last); got != c.want {
			return fmt.Errorf("CheckDue(%q, %v ago) = %v, want %v", c.schedule, time.Since(c.last).Round(time.Hour), got, c.want)
		}
	}
	if updater.NormalizeSchedule("WEEKLY") != "weekly" || updater.NormalizeSchedule("nope") != updater.DefaultSchedule {
		return fmt.Errorf("NormalizeSchedule mismatch")
	}
	return nil
}

func stressUpdaterStateRoundtrip() error {
	dir := tTempDir("updater-state")
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	_ = os.MkdirAll(dir, 0o755)

	if t := updater.InstalledEngineTag(cfg); t != "" {
		return fmt.Errorf("fresh install should have empty tag, got %q", t)
	}
	updater.RecordEngineTag(cfg, "b4800")
	if t := updater.InstalledEngineTag(cfg); t != "b4800" {
		return fmt.Errorf("recorded tag = %q, want b4800", t)
	}
	// Re-record overwrites.
	updater.RecordEngineTag(cfg, "b4900")
	if t := updater.InstalledEngineTag(cfg); t != "b4900" {
		return fmt.Errorf("overwritten tag = %q, want b4900", t)
	}
	return nil
}

func stressUpdaterAssetURL() error {
	url := updater.AssetURL("b4640")
	// The v0.9 bootstrap URL builder produced a double-b ("llama-bb4640-…");
	// the shared v1.0.0 builder must produce the canonical asset name.
	if !strings.Contains(url, "llama-b4640-bin-") || strings.Contains(url, "bb4") {
		return fmt.Errorf("asset URL not canonical: %s", url)
	}
	if !strings.HasPrefix(url, "https://github.com/ggml-org/llama.cpp/releases/download/b4640/") {
		return fmt.Errorf("asset URL prefix wrong: %s", url)
	}
	return nil
}

func stressConfigV1Fields() error {
	dir := tTempDir("cfg-v1")
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.ProMode = true
	cfg.UpdateSchedule = "monthly"
	cfg.LastUpdateCheck = time.Now().UTC().Format(time.RFC3339)
	path := filepath.Join(dir, "config.json")
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	got, err := config.Load(path)
	if err != nil {
		return err
	}
	if !got.ProMode || got.UpdateSchedule != "monthly" || got.LastUpdateCheck == "" {
		return fmt.Errorf("v1.0.0 fields lost in roundtrip: pro=%v schedule=%q last=%q",
			got.ProMode, got.UpdateSchedule, got.LastUpdateCheck)
	}
	// v1.0.4: the live version lock lives in stressV104Defaults; the
	// v1.0.0 contract checked here is the config-field roundtrip above.
	return nil
}

func stressCheckAndApplySkipsWhenFresh() error {
	dir := tTempDir("upd-fresh")
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.UpdateSchedule = "daily"
	// Just checked -> not due -> no network touched.
	updater.MarkChecked(cfg)
	msg, updated, err := updater.CheckAndApply(context.Background(), cfg, nil)
	if err != nil || updated {
		return fmt.Errorf("fresh check should be a no-op: %q updated=%v err=%v", msg, updated, err)
	}
	if !strings.Contains(msg, "up to date") {
		return fmt.Errorf("fresh-check message: %q", msg)
	}
	return nil
}
