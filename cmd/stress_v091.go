package cmd

// v0.9.1 stress tests: offline mode (netcheck), offline fast-fail of the web
// tools and remote LLM client, the llama.cpp download hint, and the
// orchestrator's offline environment note.
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
	"time"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/agent"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/netcheck"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/tools"
)

// withFakeProbe installs a deterministic connectivity probe for the duration
// of fn and restores real probing afterwards.
func withFakeProbe(online bool, fn func()) {
	netcheck.SetProbe(func() bool { return online })
	defer netcheck.SetProbe(nil)
	fn()
}

func stressNetcheckProbe() error {
	withFakeProbe(false, func() {
		if !netcheck.IsOffline() {
			fmt.Println("    (probe says online while faked offline?)")
		}
	})
	// Use Force with the fake probe still installed for determinism.
	netcheck.SetProbe(func() bool { return false })
	if netcheck.Force() {
		return fmt.Errorf("Force with offline probe returned online=true")
	}
	if netcheck.State() != "offline" {
		return fmt.Errorf("State() = %q, want offline", netcheck.State())
	}
	if !strings.Contains(netcheck.Note(), "OFFLINE") {
		return fmt.Errorf("offline Note() must warn the LLM: %q", netcheck.Note())
	}
	if !strings.Contains(netcheck.Note(), "webSearch") {
		return fmt.Errorf("offline Note() must name the disabled tools")
	}

	netcheck.SetProbe(func() bool { return true })
	if !netcheck.Force() {
		return fmt.Errorf("Force with online probe returned online=false")
	}
	if netcheck.State() != "online" {
		return fmt.Errorf("State() = %q, want online", netcheck.State())
	}
	if netcheck.Note() != "" {
		return fmt.Errorf("online Note() must be empty, got %q", netcheck.Note())
	}
	netcheck.SetProbe(nil) // restore real probing
	return nil
}

func stressWebSearchOfflineFastFail() error {
	netcheck.SetProbe(func() bool { return false })
	defer netcheck.SetProbe(nil)

	start := time.Now()
	_, err := tools.WebSearch{}.Run(context.Background(), json.RawMessage(`{"query":"latest go release"}`))
	dur := time.Since(start)
	if err == nil {
		return fmt.Errorf("webSearch must fail offline")
	}
	if !strings.Contains(err.Error(), "offline") && !strings.Contains(err.Error(), "no internet") {
		return fmt.Errorf("offline webSearch error must explain the cause: %v", err)
	}
	if dur > 2*time.Second {
		return fmt.Errorf("offline webSearch took %v — must fail fast, not crawl engines", dur)
	}
	return nil
}

func stressBrowserOfflineGuard() error {
	netcheck.SetProbe(func() bool { return false })
	defer netcheck.SetProbe(nil)

	cfg := config.Default()
	cfg.DataDir = tTempDir("browser-offline")
	_ = cfg.EnsureDirs()
	bt := tools.NewBrowserTool(cfg)
	defer bt.Close()

	_, err := bt.Run(context.Background(), json.RawMessage(`{"action":"navigate","url":"https://example.com"}`))
	if err == nil {
		return fmt.Errorf("remote navigate must be refused offline")
	}
	if !strings.Contains(err.Error(), "no internet") {
		return fmt.Errorf("offline browser error must mention offline: %v", err)
	}

	// file:// pages stay allowed offline — the guard must NOT fire; the
	// action may still fail later (no browser), but not with the offline message.
	_, err = bt.Run(context.Background(), json.RawMessage(`{"action":"navigate","url":"file:///C:/tmp/index.html"}`))
	if err != nil && strings.Contains(err.Error(), "no internet") {
		return fmt.Errorf("file:// navigate must not be blocked by the offline guard: %v", err)
	}
	return nil
}

func stressLLMRemoteOfflineFastFail() error {
	netcheck.SetProbe(func() bool { return false })
	defer netcheck.SetProbe(nil)

	// A LOCAL endpoint must be exempt from the offline fast-fail: users run
	// Ollama / LM Studio / llama.cpp on 127.0.0.1 and expect them to keep
	// working with no internet.
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"local ok"},"finish_reason":"stop"}]}`))
	}))
	defer local.Close()
	cfgLocal := config.Default()
	cfgLocal.DataDir = tTempDir("llm-offline-local")
	_ = cfgLocal.EnsureDirs()
	cfgLocal.Provider = config.ProviderRemote
	cfgLocal.RemoteBaseURL = local.URL // 127.0.0.1
	cfgLocal.RemoteModel = "m"
	if _, err := llm.NewClient(cfgLocal).Chat(context.Background(), &llm.ChatRequest{
		Model:    "m",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		return fmt.Errorf("local endpoint must work offline: %w", err)
	}

	cfg := config.Default()
	cfg.DataDir = tTempDir("llm-offline")
	_ = cfg.EnsureDirs()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = "https://api.example.com/v1" // unroutable in test
	cfg.RemoteModel = "test-model"
	client := llm.NewClient(cfg)

	start := time.Now()
	_, err := client.Chat(context.Background(), &llm.ChatRequest{
		Model:    cfg.RemoteModel,
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	dur := time.Since(start)
	if err == nil {
		return fmt.Errorf("remote chat must fail offline")
	}
	if !strings.Contains(err.Error(), "offline") && !strings.Contains(err.Error(), "no internet") {
		return fmt.Errorf("remote offline error must explain and suggest local provider: %v", err)
	}
	if dur > 3*time.Second {
		return fmt.Errorf("remote offline fail took %v — must skip the retry ladder", dur)
	}

	// Streaming path too.
	start = time.Now()
	err = client.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    cfg.RemoteModel,
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	}, func(llm.StreamEvent) error { return nil })
	dur = time.Since(start)
	if err == nil || dur > 3*time.Second {
		return fmt.Errorf("stream offline fast-fail failed: err=%v dur=%v", err, dur)
	}
	return nil
}

func stressLlamaOfflineDownloadHint() error {
	netcheck.SetProbe(func() bool { return false })
	defer netcheck.SetProbe(nil)

	cfg := config.Default()
	cfg.DataDir = tTempDir("llama-offline-dl")
	_ = cfg.EnsureDirs()
	cfg.LlamaBinPath = filepath.Join(cfg.DataDir, "bin", "llama-server-definitely-missing")
	srv := llm.NewLlamaServer(cfg)

	start := time.Now()
	err := srv.Start()
	dur := time.Since(start)
	if err == nil {
		return fmt.Errorf("llama.Start must fail when the binary is missing offline")
	}
	if !strings.Contains(err.Error(), "OFFLINE") {
		return fmt.Errorf("offline llama error must explain the offline situation: %v", err)
	}
	if !strings.Contains(err.Error(), "llama-server") {
		return fmt.Errorf("offline llama error must tell where to place the binary: %v", err)
	}
	if dur > 3*time.Second {
		return fmt.Errorf("offline llama.Start took %v — must not attempt a download", dur)
	}
	return nil
}

func stressOrchestratorOfflineNote() error {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// v1.0.1: the request body now carries the full AI-context
		// system message (~5 KB) — a single Read() can return a
		// partial chunk and truncate the tail where the offline
		// note lives. Read the WHOLE body.
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	netcheck.SetProbe(func() bool { return false })
	defer netcheck.SetProbe(nil)

	cfg := config.Default()
	cfg.DataDir = tTempDir("orch-offline")
	_ = cfg.EnsureDirs()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL
	cfg.RemoteModel = "fake"
	client := llm.NewClient(cfg)
	orch := agent.New(cfg, client)

	var captions []string
	_, err := orch.Run(context.Background(),
		[]llm.Message{{Role: "user", Content: "what can you do offline?"}},
		func(a agent.Activity) { captions = append(captions, a.Caption) })
	if err != nil {
		return fmt.Errorf("orchestrator run: %w", err)
	}
	if !strings.Contains(gotBody, "ENVIRONMENT NOTE") || !strings.Contains(gotBody, "OFFLINE") {
		return fmt.Errorf("offline environment note missing from the LLM request")
	}
	found := false
	for _, c := range captions {
		if strings.Contains(c, "Offline mode") {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("offline-mode activity caption missing")
	}
	return nil
}

func tTempDir(name string) string {
	dir, err := os.MkdirTemp("", "sheytan-"+name+"-*")
	if err != nil {
		return name
	}
	return dir
}
