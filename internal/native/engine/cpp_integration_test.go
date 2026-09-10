package engine

// cpp_integration_test.go — end-to-end test against the REAL C++ host
// binary (native/engine/build/shtn-engine-host).
//
// Skips when the C++ build artifact is not present (e.g. environments that
// only build the Go side); run `cmake -S native/engine -B
// native/engine/build && cmake --build native/engine/build` to enable it.
// When present, this test proves the actual Go↔C++ boundary: framing,
// handshake, health, hardware, metrics, cancel and shutdown all round-trip
// against the real C++ implementation.

import (
        "context"
        "os"
        "path/filepath"
        "runtime"
        "strings"
        "testing"
        "time"
)

func realHostBinaryPath() string {
        // This file lives at internal/native/engine/; the C++ tree is at
        // <repo>/native/engine/build/.
        _, thisFile, _, _ := runtime.Caller(0)
        repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))

        name := "shtn-engine-host"
        if runtime.GOOS == "windows" {
                name += ".exe"
        }

        return filepath.Join(repoRoot, "native", "engine", "build", name)
}

func TestRealCppHostEndToEnd(t *testing.T) {
        bin := realHostBinaryPath()

        if !fileExists(bin) {
                t.Skipf("C++ host binary not built (%s); build native/engine with CMake to enable", bin)
        }

        e := New(bin)

        if !e.Available() {
                t.Fatalf("host binary reported unavailable: %s", bin)
        }

        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()

        // Boot through the real handshake (protocol + ABI check).
        if err := e.Start(ctx); err != nil {
                t.Fatalf("start against real C++ host: %v", err)
        }

        t.Cleanup(func() {
                stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
                defer stopCancel()
                _ = e.Stop(stopCtx)
        })

        if e.State() != "ready" {
                t.Fatalf("state = %q, want ready", e.State())
        }

        // Active health round-trip.
        report, err := e.Health(ctx)
        if err != nil {
                t.Fatalf("health: %v", err)
        }

        if !report.Alive {
                t.Fatalf("health report: %+v", report)
        }

        // Real hardware facts from the C++ side merged with sysinfo.
        hw, err := e.Hardware(ctx)
        if err != nil {
                t.Fatalf("hardware: %v", err)
        }

        if hw.Architecture == "" {
                t.Fatal("architecture missing from real hwinfo")
        }

        if hw.CPU.LogicalCores <= 0 {
                t.Fatalf("logical cores = %d, want > 0", hw.CPU.LogicalCores)
        }

        if hw.RAM.TotalBytes == 0 {
                t.Fatal("RAM total missing from real hwinfo")
        }

        // Real measured metrics (the C++ engine measures its own RSS).
        m, err := e.MetricsSnapshot(ctx)
        if err != nil {
                t.Fatalf("metrics: %v", err)
        }

        if m.ProcessRSSBytes == 0 {
                t.Fatal("C++ engine did not measure its RSS")
        }

        if m.UptimeSeconds <= 0 {
                t.Fatal("uptime not measured")
        }

        // Cancel is a real round-trip that honestly misses in Phase 1.
        err = e.Cancel(ctx, "req-e2e")
        if err == nil || !strings.Contains(err.Error(), "no active") {
                t.Fatalf("cancel against real host: %v", err)
        }

        // Clean stop path against the real host.
        if err := e.Stop(ctx); err != nil {
                t.Fatalf("stop: %v", err)
        }

        if e.State() != "stopped" {
                t.Fatalf("state = %q, want stopped", e.State())
        }
}

func fileExists(path string) bool {
        _, err := os.Stat(path)
        return err == nil
}
