// v1.0.1 stress tests: the AI context file (AI-CONTEXT.md) lifecycle and
// system-prompt injection, plus the performance hardening cycle — streaming
// coalescing, deterministic tool order (llama.cpp prompt-cache friendly),
// and compact session persistence.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/agent"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/aicontext"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sessions"
)

// --- AI-CONTEXT.md lifecycle ---

func stressAIContextFileLifecycle() error {
	dir := tTempDir("aictx")
	defer os.RemoveAll(dir)

	// 1. Fresh install: file is materialized with marker + instructions.
	p, err := aicontext.EnsureFile(dir)
	if err != nil {
		return fmt.Errorf("EnsureFile: %w", err)
	}
	if p != filepath.Join(dir, aicontext.FileName) {
		return fmt.Errorf("path: %s", p)
	}
	body, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if !strings.Contains(string(body), aicontext.HeaderSentinel) {
		return fmt.Errorf("written file missing instruction header")
	}
	if v, ok := fileVersionOf(string(body)); !ok || v != aicontext.ContextVersion {
		return fmt.Errorf("written marker = (%d, %v), want %d", v, ok, aicontext.ContextVersion)
	}

	// 2. User edits at the same version: EnsureFile must NOT clobber them.
	edited := string(body) + "\n## USER CUSTOM SECTION\nmy own rule\n"
	if err := os.WriteFile(p, []byte(edited), 0o644); err != nil {
		return err
	}
	if _, err := aicontext.EnsureFile(dir); err != nil {
		return err
	}
	after, _ := os.ReadFile(p)
	if !strings.Contains(string(after), "USER CUSTOM SECTION") {
		return fmt.Errorf("user edits were clobbered at the same marker version")
	}

	// 3. Marker-less (user-authored) file: never touched.
	if err := os.WriteFile(p, []byte("# my own context file\nno marker here\n"), 0o644); err != nil {
		return err
	}
	if _, err := aicontext.EnsureFile(dir); err != nil {
		return err
	}
	after, _ = os.ReadFile(p)
	if !strings.HasPrefix(string(after), "# my own context file") {
		return fmt.Errorf("marker-less user file was overwritten")
	}
	return nil
}

// fileVersionOf re-implements the marker parse for the stress suite (the
// real one is unexported).
func fileVersionOf(body string) (int, bool) {
	idx := strings.Index(body, "<!-- sheytan-context-version:")
	if idx < 0 {
		return 0, false
	}
	rest := body[idx+len("<!-- sheytan-context-version:"):]
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

func stressAIContextLoadFallback() error {
	dir := tTempDir("aictx-fallback")
	defer os.RemoveAll(dir)

	// No file on disk → embedded canonical text.
	if got := aicontext.Load(dir); !strings.Contains(got, aicontext.HeaderSentinel) {
		return fmt.Errorf("embedded fallback missing instruction header")
	}

	// Disk copy wins when present.
	p := aicontext.Path(dir)
	custom := "# CUSTOM CONTEXT\nbe terse\n"
	if err := os.WriteFile(p, []byte(custom), 0o644); err != nil {
		return err
	}
	if got := aicontext.Load(dir); got != custom {
		return fmt.Errorf("disk copy must win over embedded")
	}
	return nil
}

func stressAIContextSystemMessage() error {
	cfg := config.Default()
	cfg.DataDir = tTempDir("aictx-sysmsg")
	defer os.RemoveAll(cfg.DataDir)
	aicontext.ResetProbeCache()

	msg := aicontext.SystemMessage(cfg)
	for _, want := range []string{
		aicontext.HeaderSentinel,         // the instruction file
		"LIVE ENVIRONMENT",               // the live env block
		"Tools available:",               // tool enumeration
		"Working directory (app folder)", // where the AI runs
		"dataAnalysis", "webSearch",      // tool names taught
	} {
		if !strings.Contains(msg, want) {
			return fmt.Errorf("SystemMessage missing %q", want)
		}
	}
	// The instruction text must teach the flat-JSON calling convention.
	if !strings.Contains(msg, `"action":"write"`) {
		return fmt.Errorf("SystemMessage missing the flat-JSON call example")
	}
	return nil
}

// --- orchestrator wiring ---

// captureLLM runs orch once against a stub server and returns the full
// request body the "LLM" received.
func captureLLM(tweak func(*http.Request) error, respond func(w http.ResponseWriter)) (string, error) {
	var mu sync.Mutex
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBody = string(data)
		mu.Unlock()
		if tweak != nil {
			_ = tweak(r)
		}
		respond(w)
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.DataDir = tTempDir("aictx-orch")
	defer os.RemoveAll(cfg.DataDir)
	aicontext.ResetProbeCache()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL
	cfg.RemoteModel = "fake"
	cfg.MaxIterations = 3
	client := llm.NewClient(cfg)
	orch := agent.New(cfg, client)

	_, err := orch.Run(context.Background(),
		[]llm.Message{{Role: "user", Content: "hello there"}},
		func(a agent.Activity) {})
	if err != nil {
		return "", err
	}
	mu.Lock()
	defer mu.Unlock()
	return gotBody, nil
}

func jsonReply(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
}

func stressOrchestratorPrependsContext() error {
	body, err := captureLLM(nil, jsonReply)
	if err != nil {
		return err
	}
	var req struct {
		Messages []llm.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		return err
	}
	if len(req.Messages) < 2 {
		return fmt.Errorf("want >=2 messages (context + user), got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		return fmt.Errorf("messages[0].role = %q, want system", req.Messages[0].Role)
	}
	for _, want := range []string{
		aicontext.HeaderSentinel, "LIVE ENVIRONMENT", "hello there",
	} {
		if !strings.Contains(body, want) {
			return fmt.Errorf("request missing %q", want)
		}
	}
	if req.Messages[len(req.Messages)-1].Content != "hello there" {
		return fmt.Errorf("user message not last")
	}
	return nil
}

func stressOrchestratorNoDoubleContext() error {
	// Caller pre-assembles the context message; the orchestrator must not
	// prepend a second one.
	var mu sync.Mutex
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		mu.Lock()
		body = string(data)
		mu.Unlock()
		jsonReply(w)
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.DataDir = tTempDir("aictx-dbl")
	defer os.RemoveAll(cfg.DataDir)
	aicontext.ResetProbeCache()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL
	cfg.RemoteModel = "fake"
	cfg.MaxIterations = 3
	orch := agent.New(cfg, llm.NewClient(cfg))

	_, err := orch.Run(context.Background(),
		[]llm.Message{
			{Role: "system", Content: aicontext.SystemMessage(cfg)},
			{Role: "user", Content: "hi"},
		}, func(a agent.Activity) {})
	if err != nil {
		return err
	}
	if n := strings.Count(body, aicontext.HeaderSentinel); n != 1 {
		return fmt.Errorf("context header appears %d times, want exactly 1", n)
	}
	return nil
}

// stressResponseCoalesced locks in the v1.0.1 streaming coalescer: 300 rapid
// content deltas must produce far fewer "response" activities (and the final
// text must still be complete).
func stressResponseCoalesced() error {
	const deltas = 300
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < deltas; i++ {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"x"}}]}`))
		}
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.DataDir = tTempDir("coalesce")
	defer os.RemoveAll(cfg.DataDir)
	aicontext.ResetProbeCache()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL
	cfg.RemoteModel = "fake"
	cfg.MaxIterations = 2
	orch := agent.New(cfg, llm.NewClient(cfg))

	var responses int32
	final, err := orch.Run(context.Background(),
		[]llm.Message{{Role: "user", Content: "stream test"}},
		func(a agent.Activity) {
			if a.Type == "response" {
				atomic.AddInt32(&responses, 1)
			}
		})
	if err != nil {
		return err
	}
	n := atomic.LoadInt32(&responses)
	if n > 10 {
		return fmt.Errorf("%d deltas produced %d response activities — coalescer not working", deltas, n)
	}
	if final != strings.Repeat("x", deltas) {
		return fmt.Errorf("final text corrupted: %d chars, want %d", len(final), deltas)
	}
	return nil
}

// stressToolSpecsSorted locks deterministic tool ordering: two separate runs
// must send the tools array in the SAME (sorted) order so llama.cpp can
// reuse its prompt prefix cache between turns.
func stressToolSpecsSorted() error {
	var mu sync.Mutex
	var orders []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var req struct {
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		_ = json.Unmarshal(data, &req)
		var names []string
		for _, t := range req.Tools {
			names = append(names, t.Function.Name)
		}
		mu.Lock()
		orders = append(orders, strings.Join(names, ","))
		mu.Unlock()
		jsonReply(w)
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.DataDir = tTempDir("specorder")
	defer os.RemoveAll(cfg.DataDir)
	aicontext.ResetProbeCache()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL
	cfg.RemoteModel = "fake"
	cfg.MaxIterations = 2
	orch := agent.New(cfg, llm.NewClient(cfg))
	orch.Register(stubShell{})
	orch.Register(stubFiles{})
	orch.Register(stubWeb{})

	for i := 0; i < 2; i++ {
		if _, err := orch.Run(context.Background(),
			[]llm.Message{{Role: "user", Content: "x"}}, func(a agent.Activity) {}); err != nil {
			return err
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(orders) != 2 {
		return fmt.Errorf("captured %d requests, want 2", len(orders))
	}
	if orders[0] != orders[1] {
		return fmt.Errorf("tool order unstable between runs:\n  %s\n  %s", orders[0], orders[1])
	}
	want := "afiles,ashell,aweb"
	if orders[0] != want {
		return fmt.Errorf("tool order %q, want sorted %q", orders[0], want)
	}
	return nil
}

// stub tools with sortable names for the ordering test.
type stubShell struct{}
type stubFiles struct{}
type stubWeb struct{}

func (stubShell) Name() string        { return "ashell" }
func (stubShell) Description() string { return "stub shell" }
func (stubShell) Parameters() any {
	return struct {
		Command string `json:"command"`
	}{}
}
func (stubShell) Run(ctx context.Context, args json.RawMessage) (string, error) {
	return "ok", nil
}

func (stubFiles) Name() string        { return "afiles" }
func (stubFiles) Description() string { return "stub files" }
func (stubFiles) Parameters() any {
	return struct {
		Action string `json:"action"`
	}{}
}
func (stubFiles) Run(ctx context.Context, args json.RawMessage) (string, error) {
	return "ok", nil
}

func (stubWeb) Name() string        { return "aweb" }
func (stubWeb) Description() string { return "stub web" }
func (stubWeb) Parameters() any {
	return struct {
		Query string `json:"query"`
	}{}
}
func (stubWeb) Run(ctx context.Context, args json.RawMessage) (string, error) {
	return "ok", nil
}

// stressSessionSaveCompact: session JSON is written compact (one line) —
// half the bytes and marshal cost of the old indented format.
func stressSessionSaveCompact() error {
	dir := tTempDir("sess-compact")
	defer os.RemoveAll(dir)
	store := sessions.New(dir)
	sess := store.Create()
	for i := 0; i < 5; i++ {
		if _, err := store.AppendMessage(sess.ID, llm.Message{
			Role: "user", Content: strings.Repeat("message body ", 20),
		}); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, sess.ID+".json"))
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "\n  ") {
		return fmt.Errorf("session file still indented (%d bytes) — compact save not active", len(data))
	}
	if strings.Count(string(data), "\n") > 1 {
		return fmt.Errorf("session file has %d lines, want 1", strings.Count(string(data), "\n"))
	}
	// And it must still round-trip.
	loaded, err := store.Get(sess.ID)
	if err != nil {
		return err
	}
	if len(loaded.Messages) != 5 {
		return fmt.Errorf("roundtrip lost messages: %d", len(loaded.Messages))
	}
	return nil
}

// stressContextCLI: the `sheytan context` command materializes the file and
// exits 0 (exercised in-process via the same code path).
func stressContextCLI() error {
	dir := tTempDir("aictx-cli")
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	if _, err := aicontext.EnsureFile(cfg.DataDir); err != nil {
		return err
	}
	if _, err := os.Stat(aicontext.Path(dir)); err != nil {
		return fmt.Errorf("context file missing after EnsureFile: %w", err)
	}
	// --reset path: deleting + EnsureFile regenerates.
	if err := os.Remove(aicontext.Path(dir)); err != nil {
		return err
	}
	if _, err := aicontext.EnsureFile(cfg.DataDir); err != nil {
		return err
	}
	if _, err := os.Stat(aicontext.Path(dir)); err != nil {
		return fmt.Errorf("context file not regenerated: %w", err)
	}
	return nil
}
