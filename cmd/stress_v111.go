package cmd

import (
        "fmt"
        "os"
        "path/filepath"
        "strings"

        "github.com/Parsaetak/SHEYTAN-local-agent/internal/aicontext"
        "github.com/Parsaetak/SHEYTAN-local-agent/internal/brand"
        "github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
        "github.com/Parsaetak/SHEYTAN-local-agent/internal/memory"
        "github.com/Parsaetak/SHEYTAN-local-agent/internal/resources"
)

// --- v1.0.11 (GRANITE) stress scenarios, carried into Zeta ---
//
// GRANITE was the build-integrity release. v1.0.10 shipped with two source
// packages (internal/sessions, internal/sandbox) silently swallowed by an
// UNANCHORED .gitignore pattern, a timestamp-based memory ID that collided
// on Windows clock granularity, and a log rotator that renamed over a file
// it still held open. These scenarios pin all of those fixes plus the CI
// workflow hygiene (pinned toolchain, repaired branch triggers, Node-24
// actions) so none of them can quietly regress. Zeta (1.1.0) raises the
// version floor and keeps every guard active.

// stressV111ReleaseSurface: version floor, signature, and — when running
// inside a source checkout — the repo hygiene that keeps GitHub builds
// green (anchored .gitignore, pinned CI toolchain, fixed branch triggers).
// On an end-user machine the repo files simply don't exist next to the exe,
// so those checks no-op there.
func stressV111ReleaseSurface() error {
        // Zeta floor: 1.1.0. A minimum (not an exact pin) so future point
        // releases cannot silently downgrade the release surface.
        if versionLessThan(config.AppVersion, "1.1.0") {
                return fmt.Errorf("AppVersion = %q, want >= 1.1.0 (Zeta)", config.AppVersion)
        }
        if brand.SignedBy != "Parsa Tak" {
                return fmt.Errorf("SignedBy = %q, want \"Parsa Tak\"", brand.SignedBy)
        }
        if aicontext.ContextVersion < 10 {
                return fmt.Errorf("ContextVersion = %d, want >= 10", aicontext.ContextVersion)
        }

        // The rest only applies inside a source checkout (dev box / CI). A
        // shipped exe has no .gitignore or workflows next to it.
        if _, err := os.Stat(".gitignore"); err != nil {
                return nil
        }

        // 1. Critical packages physically exist in the tree.
        for _, p := range []string{
                "internal/sessions/sessions.go",
                "internal/sandbox/sandbox.go",
                "internal/releasegate/releasegate_test.go",
        } {
                if _, err := os.Stat(p); err != nil {
                        return fmt.Errorf("critical source %s missing from the tree: %v", p, err)
                }
        }

        // 2. .gitignore runtime-dir patterns MUST be root-anchored. An
        // unanchored `sessions/` matches internal/sessions/ at ANY depth and is
        // the exact bug that broke the v1.0.9 and v1.0.10 releases.
        gi, err := os.ReadFile(".gitignore")
        if err != nil {
                return fmt.Errorf("read .gitignore: %v", err)
        }
        runtimeDirs := map[string]bool{
                "sessions/": true, "sandbox/": true, "data/": true, "logs/": true,
                "models/": true, "workspace/": true, "charts/": true, "browser-profile/": true,
                "build/": true, "dist-stage/": true,
        }
        for i, raw := range strings.Split(string(gi), "\n") {
                line := strings.TrimSpace(raw)
                if line == "" || strings.HasPrefix(line, "#") {
                        continue
                }
                if runtimeDirs[line] {
                        return fmt.Errorf(".gitignore line %d: pattern %q is UNANCHORED — it matches source dirs like internal/%s and silently drops packages from commits (the v1.0.9/v1.0.10 bug). Prefix it with '/'", i+1, line, strings.TrimSuffix(line, "/"))
                }
        }

        // 3. CI workflow: pinned toolchain, repaired branch list, Node-24
        // actions — matching the build-windows.yml layout (block-list branch
        // triggers, double-quoted Go pin).
        wf, err := os.ReadFile(filepath.Join(".github", "workflows", "build-windows.yml"))
        if err != nil {
                return fmt.Errorf("read workflow: %v", err)
        }
        w := string(wf)
        if strings.Contains(w, "go-version: 'stable'") || strings.Contains(w, `go-version: stable`) {
                return fmt.Errorf("CI toolchain floats on 'stable' — pin it to the go.mod minor line")
        }
        if !strings.Contains(w, `go-version: "1.26"`) {
                return fmt.Errorf("CI toolchain not pinned to 1.26 (go.mod line)")
        }
        if !strings.Contains(w, "runs-on: windows-latest") {
                return fmt.Errorf("CI build job lost its Windows runner")
        }
        if !strings.Contains(w, "- main") || !strings.Contains(w, "- master") || !strings.Contains(w, "workflow_dispatch:") {
                return fmt.Errorf("CI branch triggers malformed — branch pushes would never build")
        }
        for _, action := range []string{
                "actions/checkout@v5", "actions/setup-go@v6",
                "actions/setup-node@v5", "actions/upload-artifact@v5",
        } {
                if !strings.Contains(w, action) {
                        return fmt.Errorf("workflow missing %s (Node-24 runtime)", action)
                }
        }
        return nil
}

// versionLessThan reports whether semantic version a < b (three numeric
// dot-separated components; missing components count as 0).
func versionLessThan(a, b string) bool {
        pa, pb := parseVersionParts(a), parseVersionParts(b)
        for i := 0; i < 3; i++ {
                if pa[i] != pb[i] {
                        return pa[i] < pb[i]
                }
        }
        return false
}

func parseVersionParts(v string) [3]int {
        var out [3]int
        v = strings.TrimPrefix(strings.TrimSpace(v), "v")
        for i, part := range strings.SplitN(v, ".", 3) {
                n := 0
                for _, r := range part {
                        if r < '0' || r > '9' {
                                break
                        }
                        n = n*10 + int(r-'0')
                }
                out[i] = n
        }
        return out
}

// stressV111MemoryUniqueIDs: rapid Appends must produce distinct IDs even
// when the OS clock returns the same instant for consecutive calls (Windows
// granularity), and DeleteByID must remove EXACTLY one entry. The old
// timestamp-only scheme made two entries share an ID so one DeleteByID
// wiped both — the exact v1.0.10 CI failure.
func stressV111MemoryUniqueIDs() error {
        dir := tTempDir("mem111")
        defer os.RemoveAll(dir)
        st := memory.New(filepath.Join(dir, "mem.jsonl"))
        const n = 300
        for i := 0; i < n; i++ {
                if err := st.Append([]string{"v111"}, fmt.Sprintf("rapid entry %d", i), "stress"); err != nil {
                        return fmt.Errorf("append %d: %v", i, err)
                }
        }
        all, err := st.All()
        if err != nil {
                return err
        }
        if len(all) != n {
                return fmt.Errorf("got %d entries, want %d", len(all), n)
        }
        seen := make(map[string]bool, n)
        for _, e := range all {
                if seen[e.ID] {
                        return fmt.Errorf("duplicate memory ID %q after %d rapid appends", e.ID, n)
                }
                seen[e.ID] = true
        }
        if err := st.DeleteByID(all[0].ID); err != nil {
                return err
        }
        if got := st.Count(); got != n-1 {
                return fmt.Errorf("delete removed %d entries, want exactly 1", n-got)
        }
        return nil
}

// stressV111TrimLogsRotate: an over-budget log folder must actually shrink
// (bytes freed > 0), the rotated file must end at a line boundary, and no
// .rot temp files may be left behind. The v1.0.10 rotateTail held the
// source file open across the rename, which Windows refuses — TrimLogs
// swallowed the error and freed nothing.
func stressV111TrimLogsRotate() error {
        dir := tTempDir("logs111")
        defer os.RemoveAll(dir)
        // ~6.7 MB single log (64-byte lines) against a 1 MB budget.
        var b strings.Builder
        line := strings.Repeat("g", 63) + "\n"
        for i := 0; i < 105_000; i++ {
                b.WriteString(line)
        }
        logPath := filepath.Join(dir, "app.log")
        if err := os.WriteFile(logPath, []byte(b.String()), 0o644); err != nil {
                return err
        }
        freed := resources.TrimLogs(dir, 1)
        if freed <= 0 {
                return fmt.Errorf("TrimLogs freed %d bytes on an over-budget folder, want > 0", freed)
        }
        fi, err := os.Stat(logPath)
        if err != nil {
                return err
        }
        if fi.Size() > 2<<20 {
                return fmt.Errorf("log still %d bytes after trim", fi.Size())
        }
        data, err := os.ReadFile(logPath)
        if err != nil {
                return err
        }
        if len(data) > 0 && data[len(data)-1] != '\n' {
                return fmt.Errorf("rotated log must end at a line boundary")
        }
        // No temp rotation leftovers.
        if _, err := os.Stat(logPath + ".rot"); err == nil {
                return fmt.Errorf("rotate left a .rot temp file behind")
        }
        return nil
}
