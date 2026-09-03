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
// it still held open.
//
// These scenarios pin those fixes and validate the current Zeta release
// surface: anchored runtime .gitignore rules, the portable desktop workflow,
// pinned Go toolchain, main-branch builds, workflow_dispatch support,
// current GitHub Action majors, portable ZIP packaging, collision-proof
// memory IDs, and Windows-safe log rotation.

func stressV111ReleaseSurface() error {
	// Zeta floor: 1.1.0. This is a minimum rather than an exact pin so
	// future compatible point releases cannot silently downgrade the release
	// surface.
	if versionLessThan(config.AppVersion, "1.1.0") {
		return fmt.Errorf(
			"AppVersion = %q, want >= 1.1.0 (Zeta)",
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
	// which was the release-integrity regression this suite was created to
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
	//
	// The workflow was renamed from build-windows.yml to
	// build-desktop.yml. Do not hard-code the historical filename.
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

	if !strings.Contains(w, `go-version: "1.26"`) {
		return fmt.Errorf(
			"CI toolchain not pinned to 1.26",
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

	// The old stress test required a master trigger. Zeta no longer does;
	// main is the canonical development/release branch.
	if strings.Contains(w, "- master") {
		return fmt.Errorf(
			"stale master branch trigger found in Zeta workflow",
		)
	}

	// 3d. Validate the action versions used by the current workflow.
	//
	// setup-node is deliberately absent from Zeta CI because the hosted
	// runners already provide a sufficiently recent Node.js runtime.
	for _, action := range []string{
		"actions/checkout@v4",
		"actions/setup-go@v5",
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

	if strings.Contains(w, "actions/setup-node@") {
		return fmt.Errorf(
			"stale setup-node action found — Zeta intentionally uses the hosted Node.js runtime",
		)
	}

	// 3e. The workflow must build and validate the portable ZIPs.
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

	// 3f. ZIPs must preserve the portable application folder structure.
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

	// 3g. The workflow must perform frontend verification before Go build.
	for _, fragment := range []string{
		"npm run typecheck",
		"npm run lint",
		"npm run build",
		"web/static/index.html",
	} {
		if !strings.Contains(w, fragment) {
			return fmt.Errorf(
				"frontend release gate missing %q",
				fragment,
			)
		}
	}

	// 3h. The current workflow intentionally does not use the broken,
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

// stressV111MemoryUniqueIDs: rapid Appends must produce distinct IDs even
// when the OS clock returns the same instant for consecutive calls (Windows
// granularity), and DeleteByID must remove EXACTLY one entry.
//
// The old timestamp-only scheme made two entries share an ID so one
// DeleteByID wiped both.
func stressV111MemoryUniqueIDs() error {
	dir := tTempDir("mem111")
	defer os.RemoveAll(dir)

	st := memory.New(filepath.Join(dir, "mem.jsonl"))

	const n = 300

	for i := 0; i < n; i++ {
		if err := st.Append(
			[]string{"v111"},
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

// stressV111TrimLogsRotate: an over-budget log folder must actually shrink
// (bytes freed > 0), the rotated file must end at a line boundary, and no
// .rot temp files may be left behind.
//
// The v1.0.10 rotateTail held the source file open across the rename, which
// Windows refuses — TrimLogs previously swallowed the error and freed
// nothing.
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
