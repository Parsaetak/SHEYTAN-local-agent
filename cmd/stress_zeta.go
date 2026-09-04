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

// --- v1.1.2Z (ZETA) release-surface scenarios ---
//
// v1.1.1Z was the CI-repair + legacy-cleanup release (GTK4/WebKitGTK-6.0
// Linux deps, versioned stress/e2e retirement). v1.1.2Z completes the
// contract: the workflow lost the setup-node pin, the gen-syso Windows
// resource step, the -H=windowsgui subsystem flag and the .bat launcher
// during a refactor — the stress gate now runs in CI so that class of
// regression fails the build instead of shipping.
//
// These scenarios pin the v1.1.2Z surface: the repaired Linux CI
// dependencies, the pinned Node 24 + Go 1.26 toolchain, the Windows
// resource/icon pipeline, the packaged .bat launcher, portable ZIP
// packaging, collision-proof memory IDs, and Windows-safe log rotation.

func stressZetaReleaseSurface() error {
        // Zeta floor: 1.1.2. This is a minimum rather than an exact pin so
        // future compatible point releases cannot silently downgrade the
        // release surface.
        if versionLessThan(config.AppVersion, "1.1.2") {
                return fmt.Errorf(
                        "AppVersion = %q, want >= 1.1.2 (Zeta)",
                        config.AppVersion,
                )
        }

        if brand.SignedBy != "Parsa Tak" {
                return fmt.Errorf(
                        "SignedBy = %q, want %q",
                        brand.SignedBy,
                        "Parsa Tak",
                )
        }

        if aicontext.ContextVersion < 10 {
                return fmt.Errorf(
                        "ContextVersion = %d, want >= 10",
                        aicontext.ContextVersion,
                )
        }

        // The repository-only checks apply only when running from a source
        // checkout. A packaged end-user binary does not normally contain these
        // source-control files beside it.
        if _, err := os.Stat(".gitignore"); err != nil {
                return nil
        }

        // 1. Critical source packages must physically exist in the repository.
        for _, p := range []string{
                "internal/sessions/sessions.go",
                "internal/sandbox/sandbox.go",
                "internal/releasegate/releasegate_test.go",
                "internal/desktop/desktop.go",
        } {
                if _, err := os.Stat(p); err != nil {
                        return fmt.Errorf(
                                "critical source %s missing from the tree: %v",
                                p,
                                err,
                        )
                }
        }

        // 2. Runtime directories in .gitignore must be root-anchored.
        //
        // An unanchored "sessions/" pattern also matches internal/sessions/,
        // which was the release-integrity regression this gate was created to
        // catch.
        gi, err := os.ReadFile(".gitignore")
        if err != nil {
                return fmt.Errorf("read .gitignore: %v", err)
        }

        runtimeDirs := map[string]bool{
                "sessions/":        true,
                "sandbox/":         true,
                "data/":            true,
                "logs/":            true,
                "models/":          true,
                "workspace/":       true,
                "charts/":          true,
                "browser-profile/": true,
                "build/":           true,
                "dist-stage/":      true,
        }

        for i, raw := range strings.Split(string(gi), "\n") {
                line := strings.TrimSpace(raw)

                if line == "" || strings.HasPrefix(line, "#") {
                        continue
                }

                if runtimeDirs[line] {
                        return fmt.Errorf(
                                ".gitignore line %d: pattern %q is UNANCHORED — it can match source directories such as internal/%s. Prefix it with '/'",
                                i+1,
                                line,
                                strings.TrimSuffix(line, "/"),
                        )
                }
        }

        // 3. Current desktop CI release contract.
        workflowsDir := filepath.Join(".github", "workflows")

        workflowCandidates := []string{
                filepath.Join(workflowsDir, "build-desktop.yml"),
                filepath.Join(workflowsDir, "build-desktop.yaml"),
        }

        var workflowPath string
        for _, candidate := range workflowCandidates {
                if _, err := os.Stat(candidate); err == nil {
                        workflowPath = candidate
                        break
                }
        }

        if workflowPath == "" {
                matches, globErr := filepath.Glob(filepath.Join(workflowsDir, "build-*.*ml"))
                if globErr != nil {
                        return fmt.Errorf("discover desktop workflow: %v", globErr)
                }

                if len(matches) == 1 {
                        workflowPath = matches[0]
                }
        }

        if workflowPath == "" {
                return fmt.Errorf(
                        "desktop workflow missing: expected .github/workflows/build-desktop.yml",
                )
        }

        wf, err := os.ReadFile(workflowPath)
        if err != nil {
                return fmt.Errorf("read workflow %q: %v", workflowPath, err)
        }

        w := string(wf)

        // 3a. The Zeta workflow must pin Go instead of floating on "stable".
        if strings.Contains(w, "go-version: 'stable'") ||
                strings.Contains(w, `go-version: stable`) {
                return fmt.Errorf(
                        "CI toolchain floats on 'stable' — pin it to 1.26",
                )
        }

        // Both jobs consume the shared GO_VERSION env, so the pin itself
        // lives in the workflow-level env block.
        if !strings.Contains(w, `GO_VERSION: "1.26"`) {
                return fmt.Errorf(
                        "CI toolchain not pinned to 1.26 (expected GO_VERSION: \"1.26\" in the workflow env block)",
                )
        }

        // 3b. Both target platforms must remain part of the desktop release.
        if !strings.Contains(w, "name: Windows x64") {
                return fmt.Errorf("Windows x64 build job is missing")
        }

        if !strings.Contains(w, "name: Linux x64") {
                return fmt.Errorf("Linux x64 build job is missing")
        }

        if !strings.Contains(w, "runs-on: windows-latest") {
                return fmt.Errorf("Windows build lost its windows-latest runner")
        }

        if !strings.Contains(w, "runs-on: ubuntu-24.04") {
                return fmt.Errorf("Linux build lost its ubuntu-24.04 runner")
        }

        // 3c. Zeta builds from main and supports manual execution.
        if !strings.Contains(w, "branches:") ||
                !strings.Contains(w, "- main") {
                return fmt.Errorf(
                        "CI main branch trigger is missing",
                )
        }

        if !strings.Contains(w, "workflow_dispatch:") {
                return fmt.Errorf(
                        "workflow_dispatch trigger is missing",
                )
        }

        if strings.Contains(w, "- master") {
                return fmt.Errorf(
                        "stale master branch trigger found in Zeta workflow",
                )
        }

        // 3d. Validate the action versions used by the current workflow.
        for _, action := range []string{
                "actions/checkout@v4",
                "actions/setup-go@v5",
                "actions/setup-node@v4",
                "actions/upload-artifact@v4",
                "softprops/action-gh-release@v2",
        } {
                if !strings.Contains(w, action) {
                        return fmt.Errorf(
                                "workflow missing required action %s",
                                action,
                        )
                }
        }

        // 3e. The Node toolchain is PINNED (v1.1.1Z): the new React/Vite UI
        // must never depend on whatever Node version the hosted runner
        // happens to ship.
        if !strings.Contains(w, `node-version: "24"`) {
                return fmt.Errorf(
                        "CI does not pin Node 24 for the frontend build",
                )
        }

        // 3f. THE v1.1.1Z FIX: the Linux job must install the GTK4 /
        // WebKitGTK-6.0 development packages that Wails v3 actually links
        // through cgo + pkg-config. The v1.1.0 workflow installed the Wails
        // v2 era packages (libgtk-3-dev / libwebkit2gtk-4.1-dev) and every
        // Linux build failed at "Go tests" with "Package gtk4 was not
        // found".
        for _, linuxDep := range []string{
                "libgtk-4-dev",
                "libwebkitgtk-6.0-dev",
        } {
                if !strings.Contains(w, linuxDep) {
                        return fmt.Errorf(
                                "Linux CI dependencies missing %q — Wails v3 needs GTK4 + WebKitGTK 6.0 (the exact cause of the v1.1.0 CI failures)",
                                linuxDep,
                        )
                }
        }

        for _, staleDep := range []string{
                "libgtk-3-dev",
                "libwebkit2gtk-4.1-dev",
        } {
                if strings.Contains(w, staleDep) {
                        return fmt.Errorf(
                                "stale Wails v2 era dependency %q still installed by Linux CI",
                                staleDep,
                        )
                }
        }

        // 3g. The Windows build must carry the brand resources: gen-syso
        // (icon + Parsa Tak signature + DPI-aware manifest) and the
        // windowsgui subsystem flag (no console flash on double-click).
        if !strings.Contains(w, "go run ./scripts/gen-syso") {
                return fmt.Errorf(
                        "Windows build lost the gen-syso resource step (icon + version info + DPI manifest)",
                )
        }

        if !strings.Contains(w, "-H=windowsgui") &&
                !strings.Contains(w, "-H windowsgui") {
                return fmt.Errorf(
                        "Windows build lost the -H=windowsgui subsystem flag",
                )
        }

        // 3h. The workflow must build and validate the portable ZIPs.
        requiredWorkflowFragments := []string{
                "SHEYTAN-Local-Agent-Windows-x64-v${env:APP_VERSION}Z.zip",
                "SHEYTAN-Local-Agent-Linux-x64-v${APP_VERSION}Z.zip",
                "Create Windows ZIP",
                "Create Linux ZIP",
                "Verify Windows ZIP",
                "Verify Linux ZIP",
                "Upload Windows ZIP",
                "Upload Linux ZIP",
        }

        for _, fragment := range requiredWorkflowFragments {
                if !strings.Contains(w, fragment) {
                        return fmt.Errorf(
                                "desktop workflow missing release-package contract %q",
                                fragment,
                        )
                }
        }

        // 3i. ZIPs must preserve the portable application folder structure.
        for _, fragment := range []string{
                "SHEYTAN-Local-Agent/SHEYTAN-Local-Agent.exe",
                "SHEYTAN-Local-Agent/SHEYTAN-Local-Agent",
                "SHEYTAN-Local-Agent/models/",
                "SHEYTAN-Local-Agent/workspace/",
        } {
                if !strings.Contains(w, fragment) {
                        return fmt.Errorf(
                                "portable ZIP contract missing %q",
                                fragment,
                        )
                }
        }

        // 3j. The workflow must perform frontend verification before the Go
        // build, and the Windows package must ship the .bat launcher.
        for _, fragment := range []string{
                "npm run typecheck",
                "npm run lint",
                "npm run build",
                "web/static/index.html",
                "sheytan-local-agent.bat",
        } {
                if !strings.Contains(w, fragment) {
                        return fmt.Errorf(
                                "frontend/package release gate missing %q",
                                fragment,
                        )
                }
        }

        // 3k. The current workflow intentionally does not use the broken,
        // repository-wide Prettier gate that previously stopped CI before the
        // actual build.
        if strings.Contains(w, "npm run format:check") {
                return fmt.Errorf(
                        "workflow still contains the stale repository-wide format gate",
                )
        }

        return nil
}

// versionLessThan reports whether semantic version a < b.
//
// The comparison intentionally uses three numeric dot-separated components.
// Missing components count as zero, and a leading "v" is ignored.
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

// tTempDir creates a scratch directory for stress scenarios.
func tTempDir(name string) string {
        dir, err := os.MkdirTemp("", "sheytan-"+name+"-*")
        if err != nil {
                return name
        }
        return dir
}

// stressZetaMemoryUniqueIDs: rapid Appends must produce distinct IDs even
// when the OS clock returns the same instant for consecutive calls (Windows
// granularity), and DeleteByID must remove EXACTLY one entry.
//
// The old timestamp-only scheme made two entries share an ID so one
// DeleteByID wiped both.
func stressZetaMemoryUniqueIDs() error {
        dir := tTempDir("mem-zeta")
        defer os.RemoveAll(dir)

        st := memory.New(filepath.Join(dir, "mem.jsonl"))

        const n = 300

        for i := 0; i < n; i++ {
                if err := st.Append(
                        []string{"zeta"},
                        fmt.Sprintf("rapid entry %d", i),
                        "stress",
                ); err != nil {
                        return fmt.Errorf("append %d: %v", i, err)
                }
        }

        all, err := st.All()
        if err != nil {
                return err
        }

        if len(all) != n {
                return fmt.Errorf(
                        "got %d entries, want %d",
                        len(all),
                        n,
                )
        }

        seen := make(map[string]bool, n)

        for _, e := range all {
                if seen[e.ID] {
                        return fmt.Errorf(
                                "duplicate memory ID %q after %d rapid appends",
                                e.ID,
                                n,
                        )
                }

                seen[e.ID] = true
        }

        if err := st.DeleteByID(all[0].ID); err != nil {
                return err
        }

        if got := st.Count(); got != n-1 {
                return fmt.Errorf(
                        "delete removed %d entries, want exactly 1",
                        n-got,
                )
        }

        return nil
}

// stressZetaTrimLogsRotate: an over-budget log folder must actually shrink
// (bytes freed > 0), the rotated file must end at a line boundary, and no
// .rot temp files may be left behind.
//
// The v1.0.10 rotateTail held the source file open across the rename, which
// Windows refuses — TrimLogs previously swallowed the error and freed
// nothing.
func stressZetaTrimLogsRotate() error {
        dir := tTempDir("logs-zeta")
        defer os.RemoveAll(dir)

        // ~6.7 MB single log (64-byte lines) against a 1 MB budget.
        var b strings.Builder
        line := strings.Repeat("g", 63) + "\n"

        for i := 0; i < 105_000; i++ {
                b.WriteString(line)
        }

        logPath := filepath.Join(dir, "app.log")

        if err := os.WriteFile(
                logPath,
                []byte(b.String()),
                0o644,
        ); err != nil {
                return err
        }

        freed := resources.TrimLogs(dir, 1)

        if freed <= 0 {
                return fmt.Errorf(
                        "TrimLogs freed %d bytes on an over-budget folder, want > 0",
                        freed,
                )
        }

        fi, err := os.Stat(logPath)
        if err != nil {
                return err
        }

        if fi.Size() > 2<<20 {
                return fmt.Errorf(
                        "log still %d bytes after trim",
                        fi.Size(),
                )
        }

        data, err := os.ReadFile(logPath)
        if err != nil {
                return err
        }

        if len(data) > 0 && data[len(data)-1] != '\n' {
                return fmt.Errorf(
                        "rotated log must end at a line boundary",
                )
        }

        if _, err := os.Stat(logPath + ".rot"); err == nil {
                return fmt.Errorf(
                        "rotate left a .rot temp file behind",
                )
        }

        return nil
}
