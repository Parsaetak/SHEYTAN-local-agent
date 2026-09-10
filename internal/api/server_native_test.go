package api

// server_native_test.go — v1.1.5Z Phase 1: the engine snapshot must expose
// the backend selection and the native engine status honestly, without
// changing any v1.1.4Z behavior when the native path is disabled (the
// default).

import (
        "encoding/json"
        "io"
        "net/http"
        "net/http/httptest"
        "path/filepath"
        "strings"
        "testing"

        "github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
        "github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
)

func TestEngineSnapshotDefaultHasNoNativeBlock(t *testing.T) {
        server, _ := newTestServer(t) // default: EngineBackend = llama

        resp, err := http.Get(server.URL + "/api/engine")
        if err != nil {
                t.Fatalf("GET /api/engine: %v", err)
        }
        defer resp.Body.Close()

        var snap struct {
                Backend string           `json:"backend"`
                Native  *json.RawMessage `json:"native"`
        }

        if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
                t.Fatalf("decode: %v", err)
        }

        // Default: generation backend is llama and NO native block exists —
        // byte-compatible with the v1.1.4Z payload plus the backend name.
        if snap.Backend != "llama" {
                t.Fatalf("backend = %q, want llama (default)", snap.Backend)
        }

        if snap.Native != nil {
                t.Fatalf("native block must be absent by default, got %s", *snap.Native)
        }
}

func newNativeTestServer(t *testing.T, engineBackend string) *httptest.Server {
        t.Helper()

        cfg := config.Default()
        cfg.DataDir = t.TempDir()
        cfg.ModelsDir = filepath.Join(cfg.DataDir, "models")
        cfg.SessionsDir = filepath.Join(cfg.DataDir, "sessions")
        cfg.Host = "127.0.0.1"
        cfg.Port = 0
        cfg.Provider = "local"
        cfg.LlamaAutoStart = false
        cfg.EngineBackend = engineBackend

        srv, err := New(cfg)
        if err != nil {
                t.Fatalf("api.New: %v", err)
        }

        if err := srv.EnsureSetup(); err != nil {
                t.Fatalf("EnsureSetup: %v", err)
        }

        t.Cleanup(srv.Close)

        server := httptest.NewServer(srv.Handler())
        t.Cleanup(server.Close)

        return server
}

func TestEngineSnapshotNativeSelectedButUnavailable(t *testing.T) {
        // Native selected, host binary NOT built → the block must say so
        // honestly and the generation backend must remain llama.
        server := newNativeTestServer(t, "native")

        resp, err := http.Get(server.URL + "/api/engine")
        if err != nil {
                t.Fatalf("GET /api/engine: %v", err)
        }
        defer resp.Body.Close()

        var snap struct {
                Backend string `json:"backend"`
                Native  *struct {
                        Selected  bool   `json:"selected"`
                        Available bool   `json:"available"`
                        State     string `json:"state"`
                        Detail    string `json:"detail,omitempty"`
                } `json:"native"`
        }

        if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
                t.Fatalf("decode: %v", err)
        }

        if snap.Backend != "llama" {
                t.Fatalf("backend = %q, want llama (native cannot generate in Phase 1)", snap.Backend)
        }

        if snap.Native == nil {
                t.Fatal("native block missing when the native path is selected")
        }

        if !snap.Native.Selected {
                t.Fatal("native.selected must be true")
        }

        if snap.Native.Available {
                t.Fatal("native.available must be false (no host binary built in tests)")
        }

        // No start was attempted (no prewarm with LlamaAutoStart=false): the
        // engine is idle — an honest, reachable state.
        if snap.Native.State != llm.StateIdle {
                t.Fatalf("native state = %q, want idle before any start attempt", snap.Native.State)
        }
}

func TestEngineToggleStartReportsLlamaState(t *testing.T) {
        // The POST /api/llama action contract is unchanged: the response
        // carries the llama engine state; native failures never leak into
        // the response path (they surface in /api/engine).
        server := newNativeTestServer(t, "native")

        body := `{"action":"start"}`
        resp, err := http.Post(server.URL+"/api/llama", "application/json", jsonReader(body))
        if err != nil {
                t.Fatalf("POST /api/llama: %v", err)
        }
        defer resp.Body.Close()

        // No llama binary exists in the test environment: the start must
        // fail with 500 exactly as in v1.1.4Z (no native interference).
        if resp.StatusCode != http.StatusInternalServerError {
                t.Fatalf("status = %d, want 500 (missing engine binary, unchanged behavior)", resp.StatusCode)
        }
}

func jsonReader(s string) io.Reader {
        return strings.NewReader(s)
}
