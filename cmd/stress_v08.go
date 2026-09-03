package cmd

// stress_v08.go — chaos tests added in v0.8 for the log catcher, browser
// automation, remote provider client, and the SHEYTAN™ brand surfaces.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing" //nolint — used for assertions only via helper
	"time"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/agent"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/brand"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/browser"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/logging"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/tools"
)

// --- logging tests ---

func stressLoggingRotation() error {
	dir, _ := os.MkdirTemp("", "sheytan-log-*")
	defer os.RemoveAll(dir)
	m, err := logging.New(dir)
	if err != nil {
		return err
	}
	defer m.Close()
	// ~3 MB of lines with a 2 MB rotation cap → must rotate at least once.
	for i := 0; i < 30_000; i++ {
		m.Info("test", "%04d %s", i, strings.Repeat("x", 90))
	}
	if _, err := os.Stat(filepath.Join(dir, "app-1.log")); err != nil {
		return fmt.Errorf("app.log did not rotate: %w", err)
	}
	// Recent() ring must be bounded.
	if got := len(m.Recent(1000)); got > 512 {
		return fmt.Errorf("recent ring grew unbounded: %d lines", got)
	}
	return nil
}

func stressLoggingStructuredRecords() error {
	dir, _ := os.MkdirTemp("", "sheytan-log-*")
	defer os.RemoveAll(dir)
	m, err := logging.New(dir)
	if err != nil {
		return err
	}
	defer m.Close()
	m.ToolCall(logging.ToolCallRecord{TS: time.Now(), Tool: "files", Args: `{"action":"list"}`, Result: "a\nb", DurationMs: 12})
	m.ToolCall(logging.ToolCallRecord{TS: time.Now(), Tool: "shell", Error: "boom", DurationMs: 5})
	m.LLMCall(logging.LLMCallRecord{TS: time.Now(), Provider: "remote", Model: "glm-4.6", PromptMsgs: 3, CompletionChars: 120, DurationMs: 800})

	toolsData, _ := os.ReadFile(filepath.Join(dir, "tools.jsonl"))
	if !strings.Contains(string(toolsData), `"tool":"files"`) || !strings.Contains(string(toolsData), `"error":"boom"`) {
		return fmt.Errorf("tools.jsonl missing records: %s", toolsData)
	}
	llmData, _ := os.ReadFile(filepath.Join(dir, "llm.jsonl"))
	if !strings.Contains(string(llmData), `"provider":"remote"`) {
		return fmt.Errorf("llm.jsonl missing record: %s", llmData)
	}

	st := m.ComputeStats()
	if st.ToolCalls != 2 || st.ToolErrors != 1 || st.LLMCalls != 1 {
		return fmt.Errorf("stats wrong: %+v", st)
	}
	if st.CallsPerTool["files"] != 1 || st.ErrorsPerTool["shell"] != 1 {
		return fmt.Errorf("per-tool stats wrong: %+v", st)
	}
	return nil
}

func stressLoggingCrashReport() error {
	dir, _ := os.MkdirTemp("", "sheytan-log-*")
	defer os.RemoveAll(dir)
	m, _ := logging.New(dir)
	defer m.Close()
	logging.SetVersion("0.8.0-test")
	path := m.Crash("synthetic panic: index out of range", []byte("goroutine 1 [running]:\nmain.main()"))
	if path == "" {
		return fmt.Errorf("crash file not written")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "0.8.0-test") || !strings.Contains(string(data), "goroutine 1") {
		return fmt.Errorf("crash file missing version/stack: %s", data)
	}
	return nil
}

func stressLoggingDiagnosticsRedacts() error {
	dir, _ := os.MkdirTemp("", "sheytan-log-*")
	defer os.RemoveAll(dir)
	m, err := logging.New(dir)
	if err != nil {
		return err
	}
	defer m.Close()
	m.Info("test", "hello")
	m.ToolCall(logging.ToolCallRecord{TS: time.Now(), Tool: "browser", Args: "{}", Result: "ok", DurationMs: 9})

	// A config that contains a secret.
	cfgPath := filepath.Join(dir, "config.json")
	secret := "sk-super-secret-key-123"
	os.WriteFile(cfgPath, []byte(fmt.Sprintf(`{"remoteApiKey":"%s","model":"x"}`, secret)), 0o644)

	zipPath := filepath.Join(dir, "diag.zip")
	if _, err := m.Diagnostics(zipPath, cfgPath, nil); err != nil {
		return err
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	var foundStats, foundRedact bool
	for _, f := range zr.File {
		if f.Name == "stats.json" {
			foundStats = true
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		if strings.Contains(string(data), secret) {
			return fmt.Errorf("SECRET LEAKED in %s", f.Name)
		}
		if f.Name == "config.redacted.json" && strings.Contains(string(data), "[REDACTED]") {
			foundRedact = true
		}
	}
	if !foundStats {
		return fmt.Errorf("stats.json missing from diagnostics zip")
	}
	if !foundRedact {
		return fmt.Errorf("config was not redacted")
	}
	return nil
}

// --- browser tests ---

func stressBrowserDiscovery() error {
	// 1) explicit override that exists (the Playwright chromium on this box,
	//    or any chromium) must be accepted.
	if path, err := findTestChromium(); err == nil {
		got, err := browser.FindChrome(path)
		if err != nil || got != path {
			return fmt.Errorf("override not honored: %v", err)
		}
	}
	// 2) bogus override → clean error, no panic.
	if _, err := browser.FindChrome("/definitely/not/a/browser"); err == nil {
		return fmt.Errorf("bogus override should fail")
	}
	return nil
}

func findTestChromium() (string, error) {
	for _, c := range []string{
		"/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser",
	} {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	home, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(home, ".cache", "ms-playwright", "chromium-*", "chrome-linux*", "chrome"))
	if len(matches) > 0 {
		return matches[len(matches)-1], nil
	}
	return "", fmt.Errorf("no chromium on this box")
}

func stressBrowserToolArgs() error {
	cfg := config.Default()
	bt := tools.NewBrowserTool(cfg)
	defer bt.Close()

	// missing action → friendly error
	if _, err := bt.Run(context.Background(), json.RawMessage(`{}`)); err == nil {
		return fmt.Errorf("expected error for missing action")
	}
	// garbage json → friendly error
	if _, err := bt.Run(context.Background(), json.RawMessage(`not json`)); err == nil {
		return fmt.Errorf("expected error for garbage args")
	}
	// unknown action → friendly error (no browser boot needed)
	if _, err := bt.Run(context.Background(), json.RawMessage(`{"action":"definitely-not-real"}`)); err == nil {
		return fmt.Errorf("expected error for unknown action")
	}
	// navigate without url → friendly error BEFORE any browser boots
	if _, err := bt.Run(context.Background(), json.RawMessage(`{"action":"navigate"}`)); err == nil {
		return fmt.Errorf("expected error for navigate without url")
	}
	// close without a session → graceful
	if _, err := bt.Run(context.Background(), json.RawMessage(`{"action":"close"}`)); err != nil {
		return fmt.Errorf("close should be graceful: %v", err)
	}
	return nil
}

// --- remote provider client tests ---

// sseChunk builds one OpenAI-style streaming chunk.
func sseChunk(payload string) string {
	return "data: " + payload + "\n\n"
}

func stressRemoteToolCallAssembly() error {
	// Interleaved tool-call fragments (two calls, alternating by index) —
	// the exact pattern that broke the old arrival-order assembler.
	payloads := []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"files","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"shell","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"action\":\"list\",\"path\":\"/tmp\"}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"command\":\"echo hi\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, p := range payloads {
			fmt.Fprint(w, sseChunk(p))
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "fake"
	client := llm.NewClient(cfg)

	var lastToolCalls []llm.ToolCall
	var sawFinish bool
	err := client.StreamChat(context.Background(), client.BuildChatRequest("fake", nil, nil), func(ev llm.StreamEvent) error {
		if len(ev.ToolCalls) > 0 {
			lastToolCalls = ev.ToolCalls
		}
		if ev.FinishReason != "" {
			sawFinish = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !sawFinish {
		return fmt.Errorf("finish reason never arrived")
	}
	if len(lastToolCalls) != 2 {
		return fmt.Errorf("expected 2 assembled tool calls, got %d: %+v", len(lastToolCalls), lastToolCalls)
	}
	if lastToolCalls[0].ID != "call_1" || lastToolCalls[0].Function.Name != "files" ||
		lastToolCalls[0].Function.Arguments != `{"action":"list","path":"/tmp"}` {
		return fmt.Errorf("call_1 mis-assembled: %+v", lastToolCalls[0])
	}
	if lastToolCalls[1].ID != "call_2" || lastToolCalls[1].Function.Name != "shell" ||
		lastToolCalls[1].Function.Arguments != `{"command":"echo hi"}` {
		return fmt.Errorf("call_2 mis-assembled: %+v", lastToolCalls[1])
	}
	return nil
}

func stressRemoteJSONFallback() error {
	// Server ignores stream:true and returns plain JSON — client must cope.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"plain json answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "fake"
	client := llm.NewClient(cfg)

	var content string
	err := client.StreamChat(context.Background(), client.BuildChatRequest("fake", nil, nil), func(ev llm.StreamEvent) error {
		if ev.Content != "" {
			content = ev.Content
		}
		return nil
	})
	if err != nil {
		return err
	}
	if content != "plain json answer" {
		return fmt.Errorf("JSON fallback lost content: %q", content)
	}
	return nil
}

func stressRemoteErrorSurface() error {
	// HTTP 500 with a JSON error body must surface a useful error, not a hang.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"message":"model overloaded"}}`)
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	client := llm.NewClient(cfg)
	_, err := client.Chat(context.Background(), client.BuildChatRequest("fake", nil, nil))
	if err == nil || !strings.Contains(err.Error(), "model overloaded") {
		return fmt.Errorf("expected 500 error with body, got: %v", err)
	}
	return nil
}

// --- orchestrator end-to-end against a fake streaming LLM ---

// stubTool records invocations for the e2e test.
type stubTool struct {
	calls atomic.Int32
}

func (s *stubTool) Name() string        { return "files" }
func (s *stubTool) Description() string { return "stub files tool for e2e" }
func (s *stubTool) Parameters() any {
	return struct {
		Action string `json:"action"`
		Path   string `json:"path"`
	}{}
}
func (s *stubTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	s.calls.Add(1)
	var p struct {
		Action string `json:"action"`
		Path   string `json:"path"`
	}
	_ = json.Unmarshal(args, &p)
	return fmt.Sprintf("stub did %s on %s", p.Action, p.Path), nil
}

func stressOrchestratorE2E() error {
	var llmCalls atomic.Int32
	stub := &stubTool{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := llmCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			// 1st call: stream one tool call in 3 fragments (id, args, args).
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"files","arguments":""}}]}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"action\":\"write\","}}]}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"path\":\"/tmp/e2e.txt\",\"content\":\"hi\"}"}}]}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
		} else {
			// 2nd call: final answer
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"role":"assistant","content":"File written successfully."}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "fake-e2e"
	cfg.MaxIterations = 5
	client := llm.NewClient(cfg)
	orch := agent.New(cfg, client)
	orch.Register(stub)

	var captions []string
	final, err := orch.Run(context.Background(),
		[]llm.Message{{Role: "user", Content: "write hi to /tmp/e2e.txt"}},
		func(a agent.Activity) { captions = append(captions, a.Type+":"+a.Caption) },
	)
	if err != nil {
		return fmt.Errorf("orchestrator run: %v (activity: %v)", err, captions)
	}
	if final != "File written successfully." {
		return fmt.Errorf("unexpected final answer: %q", final)
	}
	if got := stub.calls.Load(); got != 1 {
		return fmt.Errorf("stub tool invoked %d times, want 1", got)
	}
	if got := llmCalls.Load(); got != 2 {
		return fmt.Errorf("LLM called %d times, want 2", got)
	}
	return nil
}

// --- brand / license tests ---

func stressBrandLicense() error {
	if !strings.Contains(brand.Trademark, "™") {
		return fmt.Errorf("trademark missing ™ mark")
	}
	// v0.9: the copyright holder is Parsaetak; SHEYTAN™ is the trademark.
	if !strings.Contains(brand.Copyright(), "Parsaetak") || !strings.Contains(brand.Copyright(), "All rights reserved") {
		return fmt.Errorf("copyright line wrong: %s", brand.Copyright())
	}
	if !strings.Contains(brand.CopyrightYears(), "2024") {
		return fmt.Errorf("copyright years must start at 2024: %s", brand.CopyrightYears())
	}
	for _, want := range []string{"TRADEMARK", "LICENSE", "SHEYTAN", "END-USER LICENSE AGREEMENT"} {
		if !strings.Contains(strings.ToUpper(brand.LicenseText), want) {
			return fmt.Errorf("license text missing %q", want)
		}
	}
	return nil
}

func stressConfigProviderSwitch() error {
	cfg := config.Default()
	cfg.Provider = "local"
	cfg.LLMBaseURL = "http://127.0.0.1:8080/v1"
	cfg.Model = "gemma"
	cfg.RemoteBaseURL = "https://api.example.com/v1"
	cfg.RemoteAPIKey = "sk-test"
	cfg.RemoteModel = "glm-4.6"

	if cfg.IsRemote() {
		return fmt.Errorf("provider=local must not be remote")
	}
	if cfg.EffectiveBaseURL() != "http://127.0.0.1:8080/v1" || cfg.EffectiveModel() != "gemma" || cfg.EffectiveAPIKey() != "no-key" {
		return fmt.Errorf("local effective values wrong: %s %s", cfg.EffectiveBaseURL(), cfg.EffectiveModel())
	}

	cfg.Provider = "remote"
	if !cfg.IsRemote() {
		return fmt.Errorf("provider=remote must be remote")
	}
	if cfg.EffectiveBaseURL() != "https://api.example.com/v1" {
		return fmt.Errorf("remote base URL wrong: %s", cfg.EffectiveBaseURL())
	}
	if cfg.EffectiveAPIKey() != "sk-test" {
		return fmt.Errorf("remote API key wrong")
	}
	if cfg.EffectiveModel() != "glm-4.6" {
		return fmt.Errorf("remote model wrong: %s", cfg.EffectiveModel())
	}

	// BuildChatRequest must omit llama-only knobs for remote.
	client := llm.NewClient(cfg)
	req := client.BuildChatRequest("glm-4.6", nil, nil)
	var raw map[string]any
	b, _ := json.Marshal(req)
	json.Unmarshal(b, &raw)
	if _, ok := raw["top_k"]; ok {
		return fmt.Errorf("top_k must NOT be sent to remote providers")
	}
	if _, ok := raw["n_ctx"]; ok {
		return fmt.Errorf("n_ctx must NOT be sent to remote providers")
	}
	if _, ok := raw["temperature"]; !ok {
		return fmt.Errorf("temperature must still be sent to remote providers")
	}

	// Remote request must carry the API key header.
	var gotAuth atomic.Value
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer authSrv.Close()
	cfg.RemoteBaseURL = authSrv.URL + "/v1"
	_, err := llm.NewClient(cfg).Chat(context.Background(), llm.NewClient(cfg).BuildChatRequest("m", nil, nil))
	if err != nil {
		return err
	}
	if gotAuth.Load() != "Bearer sk-test" {
		return fmt.Errorf("Authorization header wrong: %v", gotAuth.Load())
	}
	_ = testing.Short // keep testing import meaningful
	return nil
}

// helper for byte-slices
func mustBytes(r io.Reader) []byte {
	b, _ := io.ReadAll(r)
	return bytes.Clone(b)
}
