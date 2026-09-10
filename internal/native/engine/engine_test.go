package engine

// engine_test.go — native engine lifecycle tests (v1.1.5Z Phase 1).
//
// The fake host uses the SAME test-binary re-exec pattern as the llama
// tests (internal/llm/llama_test.go): the test executable re-runs itself
// as a native host speaking the real protocol implementation from
// protocol.go, so framing/round-trip correctness is exercised in both
// directions on every run.
//
// Coverage required by the Phase 1 plan:
//   - native lifecycle (start → ready, stop → stopped)
//   - native failure (death → bounded restart → terminal failure)
//   - malformed native requests (garbage host output; connection loss)
//   - cancellation (cancel op round-trip; op timeout)
//   - concurrent lifecycle access (race detector target)
//   - hardware profile (native probe merged with sysinfo)
//   - metrics (only measured values)

import (
        "bufio"
        "context"
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "testing"
        "time"

        "github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
)

// TestMain implements the re-exec fake host.
func TestMain(m *testing.M) {
        if os.Getenv("SHEYTAN_FAKE_NATIVE_HOST") == "1" {
                runFakeNativeHost()
                os.Exit(0)
        }

        os.Exit(m.Run())
}

// runFakeNativeHost speaks the REAL protocol implementation over
// stdin/stdout. Modes (via SHEYTAN_FAKE_NATIVE_MODE):
//
//      ""       well-behaved host
//      "crash"  dies 150 ms after becoming ready (watchdog target)
//      "garbage" writes non-protocol bytes, then idles (broken host)
//      "slow"   delays every response by 3 s (timeout target)
//      "badping" answers ping with the wrong protocol version
func runFakeNativeHost() {
        mode := os.Getenv("SHEYTAN_FAKE_NATIVE_MODE")

        out := bufio.NewWriter(os.Stdout)
        defer out.Flush()

        in := bufio.NewReader(os.Stdin)

        ready := func() {
                if mode == "crash" {
                        go func() {
                                time.Sleep(150 * time.Millisecond)
                                os.Exit(1)
                        }()
                }
        }

        for {
                payload, err := ReadFrame(in)
                if err != nil {
                        return // EOF or malformed frame: exit
                }

                if mode == "garbage" {
                        // Deliberately write non-protocol bytes once.
                        _, _ = out.WriteString("this is not a frame at all\n")
                        _ = out.Flush()

                        // Then idle: the connection is dead but the process lives.
                        select {}
                }

                var req Request
                if err := json.Unmarshal(payload, &req); err != nil || req.Op == "" {
                        _ = EncodeResponse(out, &Response{
                                ID:    0,
                                OK:    false,
                                Error: "malformed request",
                        })
                        _ = out.Flush()
                        continue
                }

                if mode == "slow" {
                        time.Sleep(3 * time.Second)
                }

                resp := &Response{ID: req.ID, OK: true}

                switch req.Op {
                case OpPing:
                        proto := ProtocolVersion
                        if mode == "badping" {
                                proto = ProtocolVersion + 99
                        }

                        result, _ := json.Marshal(PingResult{
                                ProtocolVersion: proto,
                                ABIVersion:       ABIVersionExpected,
                                Engine:           "fake-native-host",
                        })
                        resp.Result = result

                case OpHealth:
                        ready()

                        result, _ := json.Marshal(HealthResult{
                                Healthy: true,
                                State:   "ready",
                                Detail:  "fake host",
                        })
                        resp.Result = result

                case OpHardware:
                        setFakeHW(&resp.Result)

                case OpMetrics:
                        result, _ := json.Marshal(MetricsResult{
                                EngineState:     "ready",
                                UptimeSeconds:   1.5,
                                ProcessRSSBytes: 4096,
                        })
                        resp.Result = result

                case OpCancel:
                        result, _ := json.Marshal(CancelResult{
                                Cancelled: false,
                                Reason:    "no active generation requests (fake host)",
                        })
                        resp.Result = result

                case OpShutdown:
                        _ = EncodeResponse(out, &Response{ID: req.ID, OK: true, Result: json.RawMessage("{}")})
                        _ = out.Flush()
                        return

                default:
                        resp.OK = false
                        resp.Error = fmt.Sprintf("unknown op %q", req.Op)
                }

                _ = EncodeResponse(out, resp)
                _ = out.Flush()
        }
}

// setFakeHW replaces the hardware result payload with the fake values.
func setFakeHW(dst *json.RawMessage) {
        type fakeCPU struct {
                Name          string `json:"name"`
                PhysicalCores int    `json:"physicalCores"`
                LogicalCores  int    `json:"logicalCores"`
        }

        type fakeRAM struct {
                TotalBytes     uint64 `json:"totalBytes"`
                AvailableBytes uint64 `json:"availableBytes"`
        }

        payload := struct {
                Architecture string  `json:"architecture"`
                CPU          fakeCPU `json:"cpu"`
                RAM          fakeRAM `json:"ram"`
        }{
                Architecture: "x86_64",
                CPU:          fakeCPU{Name: "Fake CPU", PhysicalCores: 4, LogicalCores: 8},
                RAM:          fakeRAM{TotalBytes: 16 << 30, AvailableBytes: 8 << 30},
        }

        data, _ := json.Marshal(payload)
        *dst = data
}

// fakeHostPath returns the re-exec path (this test binary) with the fake
// host environment prepared. The returned cleanup restores the env.
func fakeHostPath(t *testing.T, mode string) (string, func()) {
        t.Helper()

        exe, err := os.Executable()
        if err != nil {
                t.Fatalf("test binary path: %v", err)
        }

        dir := t.TempDir()

        // The host binary must live somewhere; the Engine spawns the path
        // directly, so the test binary itself is fine.
        if mode != "" {
                t.Setenv("SHEYTAN_FAKE_NATIVE_MODE", mode)
        } else {
                t.Setenv("SHEYTAN_FAKE_NATIVE_MODE", "")
        }

        script := exe

        _ = dir

        return script, func() {}
}

func newFakeEngine(t *testing.T, mode string) *Engine {
        t.Helper()

        path, cleanup := fakeHostPath(t, mode)
        t.Cleanup(cleanup)

        t.Setenv("SHEYTAN_FAKE_NATIVE_HOST", "1")

        return New(path)
}

// --- protocol tests ---------------------------------------------------------

func TestProtocolFramingRoundTrip(t *testing.T) {
        var sink strings.Builder

        req := &Request{ID: 42, Op: OpHealth}

        if err := EncodeRequest(&sink, req); err != nil {
                t.Fatalf("encode: %v", err)
        }

        payload, err := ReadFrame(strings.NewReader(sink.String()))
        if err != nil {
                t.Fatalf("read frame: %v", err)
        }

        decoded, err := DecodeRequest(payload)
        if err != nil {
                t.Fatalf("decode: %v", err)
        }

        if decoded.ID != 42 || decoded.Op != OpHealth {
                t.Fatalf("round trip mismatch: %+v", decoded)
        }
}

func TestProtocolRejectsOversizedFrame(t *testing.T) {
        header := []byte{0xFF, 0xFF, 0xFF, 0xFF} // > 1 MiB

        if _, err := ReadFrame(strings.NewReader(string(header))); err != ErrFrameTooLarge {
                t.Fatalf("expected ErrFrameTooLarge, got %v", err)
        }
}

func TestProtocolRejectsTruncatedFrame(t *testing.T) {
        frame := []byte{10, 0, 0, 0, 's', 'h'} // header says 10 bytes, body 2

        if _, err := ReadFrame(strings.NewReader(string(frame))); err == nil {
                t.Fatal("expected truncated frame error")
        }
}

func TestDecodeRequestValidation(t *testing.T) {
        cases := []struct {
                name    string
                payload string
                errPart string
        }{
                {"malformed json", "{not json", "malformed"},
                {"missing op", `{"id":1}`, "missing op"},
                {"unknown op", `{"id":1,"op":"explode"}`, "unknown op"},
                {"empty", "", "malformed"},
        }

        for _, tc := range cases {
                t.Run(tc.name, func(t *testing.T) {
                        _, err := DecodeRequest([]byte(tc.payload))
                        if err == nil {
                                t.Fatalf("expected error for %q", tc.payload)
                        }

                        if !strings.Contains(err.Error(), tc.errPart) {
                                t.Fatalf("error %q does not mention %q", err, tc.errPart)
                        }
                })
        }
}

func TestDecodeResultRejectsErrorResponses(t *testing.T) {
        resp := &Response{ID: 1, OK: false, Error: "engine exploded"}

        var out PingResult

        if err := DecodeResult(resp, &out); err == nil {
                t.Fatal("expected error for failed response")
        }

        respOK := &Response{ID: 1, OK: true, Result: json.RawMessage(`{"healthy":true}`)}

        var health HealthResult
        if err := DecodeResult(respOK, &health); err != nil {
                t.Fatalf("healthy result: %v", err)
        }

        if !health.Healthy {
                t.Fatal("healthy flag lost")
        }
}

// --- lifecycle tests ---------------------------------------------------------

func TestEngineStartReachesReady(t *testing.T) {
        e := newFakeEngine(t, "")

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        if err := e.Start(ctx); err != nil {
                t.Fatalf("start: %v", err)
        }

        defer e.Stop(context.Background())

        if e.State() != llm.StateReady {
                t.Fatalf("state = %q, want ready", e.State())
        }

        if !e.IsAlive() {
                t.Fatal("engine not alive after start")
        }

        if e.Pid() == 0 {
                t.Fatal("pid not reported")
        }

        if e.Restarts() != 0 {
                t.Fatalf("restarts = %d, want 0", e.Restarts())
        }
}

func TestEngineStartFailsWithMissingBinary(t *testing.T) {
        e := New(filepath.Join(t.TempDir(), "does-not-exist"))

        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        err := e.Start(ctx)
        if err == nil {
                t.Fatal("expected error for missing binary")
        }

        if !strings.Contains(err.Error(), "not found") {
                t.Fatalf("unexpected error: %v", err)
        }

        if e.State() != llm.StateFailed {
                t.Fatalf("state = %q, want failed", e.State())
        }
}

func TestEngineHealthRoundTrip(t *testing.T) {
        e := newFakeEngine(t, "")

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        if err := e.Start(ctx); err != nil {
                t.Fatalf("start: %v", err)
        }

        defer e.Stop(context.Background())

        report, err := e.Health(ctx)
        if err != nil {
                t.Fatalf("health: %v", err)
        }

        if !report.Alive || report.State != llm.StateReady {
                t.Fatalf("health report: %+v", report)
        }
}

func TestEngineHealthWhenDown(t *testing.T) {
        e := newFakeEngine(t, "")

        if _, err := e.Health(context.Background()); err == nil {
                t.Fatal("expected error when engine is down")
        }
}

func TestEngineStopWalksStoppingToStopped(t *testing.T) {
        e := newFakeEngine(t, "")

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        if err := e.Start(ctx); err != nil {
                t.Fatalf("start: %v", err)
        }

        events, unsubscribe := e.SubscribeEvents()
        defer unsubscribe()

        if err := e.Stop(context.Background()); err != nil {
                t.Fatalf("stop: %v", err)
        }

        if e.State() != llm.StateStopped {
                t.Fatalf("state = %q, want stopped", e.State())
        }

        // The event stream must contain the stopping → stopped walk.
        var sawStopping, sawStopped bool

        for {
                select {
                case ev, ok := <-events:
                        if !ok {
                                goto done
                        }

                        if ev.State == llm.StateStopping {
                                sawStopping = true
                        }

                        if ev.State == llm.StateStopped {
                                sawStopped = true
                        }

                case <-time.After(2 * time.Second):
                        goto done
                }
        }

done:
        if !sawStopping || !sawStopped {
                t.Fatalf("event walk incomplete: stopping=%t stopped=%t", sawStopping, sawStopped)
        }
}

func TestEngineEventsArePublished(t *testing.T) {
        e := newFakeEngine(t, "")

        events, unsubscribe := e.SubscribeEvents()
        defer unsubscribe()

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        if err := e.Start(ctx); err != nil {
                t.Fatalf("start: %v", err)
        }

        defer e.Stop(context.Background())

        var states []string

        for {
                select {
                case ev, ok := <-events:
                        if !ok {
                                goto done2
                        }

                        states = append(states, ev.State)

                        if ev.State == llm.StateReady {
                                goto done2
                        }

                case <-time.After(5 * time.Second):
                        goto done2
                }
        }

done2:
        joined := strings.Join(states, ",")

        if !strings.Contains(joined, llm.StateStarting) {
                t.Fatalf("missing starting transition in %q", joined)
        }

        if !strings.Contains(joined, llm.StateReady) {
                t.Fatalf("missing ready transition in %q", joined)
        }
}

func TestEngineDeathTriggersBoundedRestart(t *testing.T) {
        if testing.Short() {
                t.Skip("bounded-restart ladder takes ~8 s")
        }

        // Crash mode: every host instance dies 150 ms after ready.
        e := newFakeEngine(t, "crash")

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        if err := e.Start(ctx); err != nil {
                t.Fatalf("start: %v", err)
        }

        // The watchdog must restart (bounded) and eventually give up with a
        // visible failed state. Backoff 1s+2s+4s + ~150ms lifetime per host.
        deadline := time.Now().Add(30 * time.Second)

        for e.State() != llm.StateFailed {
                if time.Now().After(deadline) {
                        t.Fatalf("engine never reached failed (state %q, restarts %d)", e.State(), e.Restarts())
                }

                time.Sleep(100 * time.Millisecond)
        }

        if e.Restarts() < 3 {
                t.Fatalf("restarts = %d, want >= 3 before giving up", e.Restarts())
        }

        if e.Detail() == "" {
                t.Fatal("expected a visible failure detail")
        }
}

func TestEngineMalformedHostOutputDetected(t *testing.T) {
        // Garbage mode: the host writes non-protocol bytes and idles.
        e := newFakeEngine(t, "garbage")

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        // Start fails during the handshake because the connection breaks.
        err := e.Start(ctx)
        if err == nil {
                defer e.Stop(context.Background())

                // If the handshake raced the garbage write, the next health
                // probe must fail instead.
                if _, herr := e.Health(ctx); herr == nil {
                        t.Fatal("expected health failure against a garbage host")
                }

                return
        }

        if e.State() != llm.StateFailed {
                t.Fatalf("state = %q, want failed after garbage host", e.State())
        }
}

func TestEngineSlowHostHitsOpTimeout(t *testing.T) {
        e := newFakeEngine(t, "slow")

        // The slow host delays every response by 3 s; the caller-bounded
        // boot must give up well before that and tear down deterministically.
        ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
        defer cancel()

        err := e.Start(ctx)

        if err == nil {
                defer e.Stop(context.Background())
                t.Fatal("expected slow host to fail the bounded start")
        }

        if e.State() != llm.StateFailed {
                t.Fatalf("state = %q, want failed after slow host timeout", e.State())
        }
}

func TestEngineConcurrentLifecycle(t *testing.T) {
        e := newFakeEngine(t, "")

        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()

        var wg sync.WaitGroup

        for i := 0; i < 8; i++ {
                wg.Add(1)

                go func() {
                        defer wg.Done()

                        _ = e.Start(ctx)

                        _, _ = e.Health(ctx)

                        _, _ = e.MetricsSnapshot(ctx)
                }()
        }

        wg.Wait()

        if e.State() != llm.StateReady {
                t.Fatalf("state = %q, want ready after concurrent starts", e.State())
        }

        if err := e.Stop(ctx); err != nil {
                t.Fatalf("stop: %v", err)
        }
}

func TestEngineCancelReturnsReason(t *testing.T) {
        e := newFakeEngine(t, "")

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        if err := e.Start(ctx); err != nil {
                t.Fatalf("start: %v", err)
        }

        defer e.Stop(context.Background())

        // Phase 1: cancel is a real round-trip but there are no generation
        // requests, so the engine honestly reports the miss.
        err := e.Cancel(ctx, "req-1")
        if err == nil {
                t.Fatal("expected cancel miss (no active requests in Phase 1)")
        }

        if !strings.Contains(err.Error(), "no active") {
                t.Fatalf("unexpected cancel error: %v", err)
        }
}

func TestEngineMetricsMeasuredValues(t *testing.T) {
        e := newFakeEngine(t, "")

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        if err := e.Start(ctx); err != nil {
                t.Fatalf("start: %v", err)
        }

        defer e.Stop(context.Background())

        m, err := e.MetricsSnapshot(ctx)
        if err != nil {
                t.Fatalf("metrics: %v", err)
        }

        if m.Backend != "native" {
                t.Fatalf("backend = %q", m.Backend)
        }

        if m.EngineState != llm.StateReady {
                t.Fatalf("engineState = %q", m.EngineState)
        }

        // The fake host reports measured values.
        if m.UptimeSeconds <= 0 {
                t.Fatal("uptime not measured")
        }

        if m.ProcessRSSBytes == 0 {
                t.Fatal("RSS not measured")
        }

        if m.Pid == 0 {
                t.Fatal("pid not measured")
        }

        // No generation exists: decode metrics must stay absent, not zero.
        if m.DecodeTokensPerSecond != 0 {
                t.Fatal("decode speed must not be invented")
        }
}

func TestEngineHardwareMergesSysinfo(t *testing.T) {
        e := newFakeEngine(t, "")

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        if err := e.Start(ctx); err != nil {
                t.Fatalf("start: %v", err)
        }

        defer e.Stop(context.Background())

        hw, err := e.Hardware(ctx)
        if err != nil {
                t.Fatalf("hardware: %v", err)
        }

        if hw.Backend != "native" {
                t.Fatalf("backend = %q", hw.Backend)
        }

        // Fake host contributes the native-side values.
        if hw.CPU.LogicalCores != 8 {
                t.Fatalf("logical cores = %d, want 8 (fake host value)", hw.CPU.LogicalCores)
        }

        if hw.RAM.TotalBytes != 16<<30 {
                t.Fatalf("ram total = %d, want fake value", hw.RAM.TotalBytes)
        }

        // sysinfo merge supplies real values.
        found := false

        for _, src := range hw.DetectedBy {
                if src == "sysinfo" {
                        found = true
                }
        }

        if !found {
                t.Fatalf("sysinfo not merged: %v", hw.DetectedBy)
        }

        if len(hw.GPUs) == 0 && hw.DetectedBy[len(hw.DetectedBy)-1] != "sysinfo" {
                t.Fatal("unexpected detectedBy state")
        }
}

// --- backend contract tests ---------------------------------------------------

func TestBackendImplementsContract(t *testing.T) {
        var _ llm.Backend = NewBackend(nil)
}

func TestBackendGenerationNotImplemented(t *testing.T) {
        b := NewBackend(New("/nonexistent"))

        if b.Name() != "native" {
                t.Fatalf("name = %q", b.Name())
        }

        if b.GenerationCapable() {
                t.Fatal("Phase 1 native backend must not claim generation capability")
        }

        if _, err := b.Generate(context.Background(), &llm.ChatRequest{}); err == nil {
                t.Fatal("Generate must return ErrNotImplemented")
        }

        if _, err := b.StreamGenerate(context.Background(), &llm.ChatRequest{}, nil); err == nil {
                t.Fatal("StreamGenerate must return ErrNotImplemented")
        }

        if _, err := b.ModelInfo(context.Background()); err == nil {
                t.Fatal("ModelInfo must return ErrNotImplemented")
        }

        if err := b.LoadModel(context.Background(), llm.ModelSpec{Path: "/x"}); err == nil {
                t.Fatal("LoadModel must return ErrNotImplemented (after validation)")
        }
}

func TestBackendLoadModelValidatesFirst(t *testing.T) {
        b := NewBackend(New("/nonexistent"))

        // Empty path: validation error, not ErrNotImplemented.
        err := b.LoadModel(context.Background(), llm.ModelSpec{})
        if err == nil || strings.Contains(err.Error(), "not implemented") {
                t.Fatalf("expected validation error for empty path, got %v", err)
        }
}

// --- model concern validation ---------------------------------------------------

func TestValidateModelSpecPathJail(t *testing.T) {
        root := t.TempDir()

        inside := filepath.Join(root, "model.gguf")
        if err := os.WriteFile(inside, []byte("x"), 0o644); err != nil {
                t.Fatal(err)
        }

        if _, err := ValidateModelSpec(root, inside); err != nil {
                t.Fatalf("valid path rejected: %v", err)
        }

        if _, err := ValidateModelSpec(root, ""); err == nil {
                t.Fatal("empty path accepted")
        }

        if _, err := ValidateModelSpec(root, filepath.Join(root, "..", "escape.gguf")); err == nil {
                t.Fatal("path escape accepted")
        }

        if _, err := ValidateModelSpec(root, root); err == nil {
                t.Fatal("directory accepted as model")
        }
}
