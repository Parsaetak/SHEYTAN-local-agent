// Package installer ensures the local environment is set up: llama.cpp binary,
// models directory, etc. Auto-installs what it can; reports what's missing.
package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/sheytan/local-agent/internal/config"
)

// State is the persisted snapshot of installed components.
type State struct {
	AppVersion string               `json:"appVersion"`
	LastRunAt  time.Time            `json:"lastRunAt"`
	Components map[string]Component `json:"components"`
}

type Component struct {
	Version    string            `json:"version,omitempty"`
	Status     string            `json:"status"` // "installed" | "missing" | "running" | "error"
	ObservedAt time.Time         `json:"observedAt"`
	Meta       map[string]string `json:"meta,omitempty"`
}

// Manager handles installation and update checking.
type Manager struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

// EnsureRun runs the full install+check pipeline. Returns the current state
// and a list of changes (diff) since last run.
func (m *Manager) EnsureRun(force bool) (*State, []Change, error) {
	prev, _ := m.LoadState()
	if force || prev == nil {
		curr := m.detect()
		curr.AppVersion = config.AppVersion
		curr.LastRunAt = time.Now().UTC()
		_ = m.saveState(curr)
		return curr, diff(prev, curr), nil
	}
	// Update-check path
	curr := m.detect()
	curr.AppVersion = config.AppVersion
	curr.LastRunAt = time.Now().UTC()
	changes := diff(prev, curr)
	_ = m.saveState(curr)
	return curr, changes, nil
}

// Change is one entry in the per-launch diff.
type Change struct {
	Component string `json:"component"`
	Kind      string `json:"kind"` // "added" | "removed" | "changed"
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
}

func diff(prev, curr *State) []Change {
	if prev == nil {
		var out []Change
		for name, c := range curr.Components {
			out = append(out, Change{Component: name, Kind: "added", To: c.Status + " " + c.Version})
		}
		return out
	}
	var out []Change
	for name, c := range curr.Components {
		old, ok := prev.Components[name]
		if !ok {
			out = append(out, Change{Component: name, Kind: "added", To: c.Status + " " + c.Version})
			continue
		}
		if old.Status != c.Status || old.Version != c.Version {
			out = append(out, Change{
				Component: name,
				Kind:      "changed",
				From:      old.Status + " " + old.Version,
				To:        c.Status + " " + c.Version,
			})
		}
	}
	for name, c := range prev.Components {
		if _, ok := curr.Components[name]; !ok {
			out = append(out, Change{Component: name, Kind: "removed", From: c.Status + " " + c.Version})
		}
	}
	return out
}

// detect probes the system for every component.
func (m *Manager) detect() *State {
	s := &State{Components: map[string]Component{}}
	now := time.Now().UTC()

	// Go runtime
	s.Components["goRuntime"] = Component{
		Version:    runtime.Version(),
		Status:     "installed",
		ObservedAt: now,
	}

	// Models dir
	if finfo, err := os.Stat(m.cfg.ModelsDir); err == nil && finfo.IsDir() {
		entries, _ := os.ReadDir(m.cfg.ModelsDir)
		count := 0
		for _, e := range entries {
			if !e.IsDir() && len(e.Name()) > 5 && e.Name()[len(e.Name())-5:] == ".gguf" {
				count++
			}
		}
		s.Components["modelsDir"] = Component{
			Status:     "installed",
			Version:    fmt.Sprintf("%d model(s)", count),
			ObservedAt: now,
			Meta:       map[string]string{"path": m.cfg.ModelsDir},
		}
	} else {
		s.Components["modelsDir"] = Component{
			Status:     "missing",
			ObservedAt: now,
			Meta:       map[string]string{"path": m.cfg.ModelsDir},
		}
	}

	// llama.cpp server binary
	if m.cfg.LlamaBinPath != "" {
		if _, err := os.Stat(m.cfg.LlamaBinPath); err == nil {
			s.Components["llamaServer"] = Component{
				Status:     "installed",
				ObservedAt: now,
				Meta:       map[string]string{"path": m.cfg.LlamaBinPath},
			}
		} else {
			s.Components["llamaServer"] = Component{
				Status:     "missing",
				ObservedAt: now,
				Meta:       map[string]string{"hint": "auto-downloaded on first run"},
			}
		}
	}

	// Sessions dir
	if _, err := os.Stat(m.cfg.SessionsDir); err == nil {
		s.Components["sessionsDir"] = Component{
			Status:     "installed",
			ObservedAt: now,
			Meta:       map[string]string{"path": m.cfg.SessionsDir},
		}
	}

	// Docker (optional)
	if dockerPath, err := lookPath("docker"); err == nil {
		s.Components["docker"] = Component{
			Status:     "installed",
			ObservedAt: now,
			Meta:       map[string]string{"path": dockerPath},
		}
	} else {
		s.Components["docker"] = Component{
			Status:     "missing",
			ObservedAt: now,
			Meta:       map[string]string{"hint": "optional — only needed for sandboxed code execution"},
		}
	}

	return s
}

func lookPath(name string) (string, error) {
	return lookPathImpl(name)
}

// LoadState reads the persisted state file.
func (m *Manager) LoadState() (*State, error) {
	path := m.cfg.StatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// saveState writes the state to disk.
func (m *Manager) saveState(s *State) error {
	if err := os.MkdirAll(m.cfg.DataDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.cfg.StatePath(), data, 0o644)
}

// FormatState pretty-prints the state for CLI/UI display.
func FormatState(s *State) string {
	if s == nil {
		return "(no state — first run)"
	}
	out := fmt.Sprintf("SHEYTAN-Local-Agent v%s state:\n", s.AppVersion)
	out += fmt.Sprintf("  last run: %s\n\n", s.LastRunAt.Format(time.RFC3339))
	out += "Components:\n"
	for name, c := range s.Components {
		mark := "✓"
		if c.Status != "installed" {
			mark = "✗"
		}
		ver := c.Version
		if ver == "" {
			ver = c.Status
		}
		out += fmt.Sprintf("  %s %-15s %s\n", mark, name, ver)
	}
	return out
}
