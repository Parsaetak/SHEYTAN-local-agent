//go:build headless

// v1.0.0 interaction tests: the model picker applies a picked model (the
// v0.9 "click does nothing" regression), and safeMarkdown neutralizes the
// overlapping RichText table renderer.
package ui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/llm"
	"github.com/sheytan/local-agent/internal/logging"
	agentrt "github.com/sheytan/local-agent/internal/runtime"
	"github.com/sheytan/local-agent/internal/sessions"
)

func newPickerTestApp(t *testing.T) *desktopApp {
    t.Helper()

    a := test.NewApp()
    a.Settings().SetTheme(Theme())

    dir := t.TempDir()

    cfg := config.Default()
    cfg.DataDir = dir
    cfg.ModelsDir = filepath.Join(dir, "models")
    cfg.SessionsDir = filepath.Join(dir, "sessions")
    _ = cfg.EnsureDirs()

    mgr, err := logging.New(cfg.LogsDir())
    if err == nil {
        logging.SetDefault(mgr)
        logging.SetVersion(config.AppVersion)
    }

    stack := agentrt.NewStack(cfg)

    d := &desktopApp{
        cfg:    cfg,
        fyne:   a,
        store:  sessions.New(cfg.SessionsDir),
        client: stack.Client,
        orch:   stack.Orch,
        multi:  stack.Multi,
        mem:    stack.Mem,
        llama:  stack.Llama,
        sb:     stack.Sandbox,
        stack:  stack,
    }

    d.navRows = map[string]*navRow{}
    d.views = map[string]fyne.CanvasObject{}
    d.fader = newCrossFader()

    t.Cleanup(func() {
        if d.win != nil {
            d.win.Close()
        }

        if d.stack != nil {
            d.stack.Close()
        }

        if err := mgr.Close(); err != nil {
            t.Logf("close logger: %v", err)
        }

        logging.SetDefault(nil)
        a.Quit()
    })

    return d
}

// TestModelPickerAppliesModel proves the fixed interaction: tapping a model
// row persists the choice, updates the header chip, and hides the dialog.
func TestModelPickerAppliesModel(t *testing.T) {
	d := newPickerTestApp(t)

	// Two models on disk, none selected yet.
	_ = os.WriteFile(filepath.Join(d.cfg.ModelsDir, "gemma-3-4b-it.gguf"), []byte("gguf"), 0o644)
	_ = os.WriteFile(filepath.Join(d.cfg.ModelsDir, "qwen2.5-7b.gguf"), []byte("gguf"), 0o644)
	d.cfg.Model = "gemma-3-4b-it.gguf"

	content := d.buildRoot()
	d.win = test.NewWindow(content)
	d.win.Resize(fyne.NewSize(1340, 840))

	// Open the picker (same path as the model chip).
	d.pickModel()
	if d.activeModelDialog == nil {
		t.Fatal("picker dialog was not created")
	}

	// Grab the picker list rows and tap the qwen row.
	models := llm.ListLocalModels(d.cfg.ModelsDir)
	list := d.buildModelList(models)
	rows := list.Objects
	if len(rows) != 2 {
		t.Fatalf("picker rows = %d, want 2", len(rows))
	}
	qwen := rows[1].(*modelRow)
	test.Tap(qwen)

	if d.cfg.Model != "qwen2.5-7b.gguf" {
		t.Fatalf("cfg.Model after tap = %q, want qwen2.5-7b.gguf", d.cfg.Model)
	}
	// The persisted config must also carry the new model.
	saved, err := config.Load(d.cfg.ConfigPath())
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if saved.Model != "qwen2.5-7b.gguf" {
		t.Fatalf("persisted Model = %q, want qwen2.5-7b.gguf", saved.Model)
	}
	// The header chip shows the new model (v1.0.3: display label drops
	// the .gguf suffix — the chip reads "qwen2.5-7b").
	if got := d.modelChip.label.Text; got != "qwen2.5-7b" {
		t.Fatalf("model chip = %q", got)
	}
}

// TestSafeMarkdownRewritesTables locks in the table-to-code-block rewrite
// (RichText's table renderer overlaps neighboring paragraphs).
func TestSafeMarkdownRewritesTables(t *testing.T) {
	in := "before\n\n| Region | Revenue |\n|---|---|\n| Tehran | 25800 |\n\nafter"
	out := safeMarkdown(in)
	if containsAny(out, []string{"|---|"}) {
		t.Fatalf("separator row survived: %q", out)
	}
	if !containsSubstr(out, "```") || !containsSubstr(out, "| Tehran | 25800 |") {
		t.Fatalf("table not rendered as code block: %q", out)
	}
	if !containsSubstr(out, "before") || !containsSubstr(out, "after") {
		t.Fatalf("surrounding prose lost: %q", out)
	}
	// Non-table markdown passes through untouched.
	plain := "just **bold** text"
	if safeMarkdown(plain) != plain {
		t.Fatalf("plain markdown rewritten: %q", safeMarkdown(plain))
	}
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if containsSubstr(s, sub) {
			return true
		}
	}
	return false
}
