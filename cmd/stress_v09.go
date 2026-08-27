package cmd

// v0.9 stress tests: portable single-folder storage, data-analysis tool,
// Parsaetak brand, cross-tool path interoperability, and chart rendering.
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sheytan/local-agent/internal/brand"
	"github.com/sheytan/local-agent/internal/config"
	agentrt "github.com/sheytan/local-agent/internal/runtime"
	"github.com/sheytan/local-agent/internal/tools"
)

// --- portable storage ---

func stressPortableAppRoot() error {
	root := config.AppRoot()
	if root == "" || root == "." {
		return fmt.Errorf("AppRoot returned %q", root)
	}
	// AppRoot must be a directory that exists.
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return fmt.Errorf("AppRoot %q is not a directory: %v", root, err)
	}
	// Default config must place every standard dir inside the root.
	cfg := config.Default()
	cfg.DataDir = root // simulate
	for _, p := range []string{cfg.ModelsDir, cfg.SessionsDir, cfg.LogsDir(), cfg.ChartsDir(), cfg.BrowserProfileDir(), cfg.SandboxDir(), cfg.WorkspaceDir()} {
		if !strings.HasPrefix(p, root) {
			return fmt.Errorf("path %q escapes app root %q", p, root)
		}
	}
	return nil
}

func stressPortableConfigRoundTrip() error {
	dir, err := os.MkdirTemp("", "sheytan-portable-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	oldDir := os.Getenv("SHEYTAN_DATA_DIR")
	os.Setenv("SHEYTAN_DATA_DIR", dir)
	defer os.Setenv("SHEYTAN_DATA_DIR", oldDir)

	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}
	if cfg.DataDir != dir {
		return fmt.Errorf("DataDir=%q want %q", cfg.DataDir, dir)
	}
	if err := cfg.EnsureDirs(); err != nil {
		return fmt.Errorf("EnsureDirs: %w", err)
	}
	for _, sub := range []string{"models", "sessions", "logs", "charts", "browser-profile", "sandbox", "workspace", "bin"} {
		if fi, err := os.Stat(filepath.Join(dir, sub)); err != nil || !fi.IsDir() {
			return fmt.Errorf("missing portable dir %s: %v", sub, err)
		}
	}
	// Save + reload round-trip keeps the portable root.
	cfg.Provider = config.ProviderLocal
	if err := config.Save(cfg.ConfigPath(), cfg); err != nil {
		return fmt.Errorf("save: %w", err)
	}
	cfg2, err := config.Load(cfg.ConfigPath())
	if err != nil {
		return fmt.Errorf("reload: %w", err)
	}
	if cfg2.DataDir != dir {
		return fmt.Errorf("reloaded DataDir=%q want %q", cfg2.DataDir, dir)
	}
	return nil
}

func stressPortableLegacyMigration() error {
	// Build a fake legacy ~/.sheytan? Can't touch real HOME safely — use a
	// temp HOME.
	home, err := os.MkdirTemp("", "sheytan-home-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	oldHome, oldProfile := os.Getenv("HOME"), os.Getenv("USERPROFILE")
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	defer func() {
		os.Setenv("HOME", oldHome)
		os.Setenv("USERPROFILE", oldProfile)
	}()
	root, err := os.MkdirTemp("", "sheytan-root-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	legacy := filepath.Join(home, ".sheytan")
	if err := os.MkdirAll(filepath.Join(legacy, "sessions"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(legacy, "memory.jsonl"), []byte(`{"id":"1","content":"migrated fact"}`+"\n"), 0o644); err != nil {
		return err
	}

	config.MigrateLegacy(root)

	if _, err := os.Stat(filepath.Join(root, "memory.jsonl")); err != nil {
		return fmt.Errorf("legacy memory.jsonl not migrated: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(root, "sessions")); err != nil || !fi.IsDir() {
		return fmt.Errorf("legacy sessions dir not migrated: %v", err)
	}
	// Second migration must be a no-op (config.json now exists).
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(`{}`), 0o644); err != nil {
		return err
	}
	config.MigrateLegacy(root)
	return nil
}

// --- cross-tool path interoperability ---

func stressToolBaseDirInterop() error {
	dir, err := os.MkdirTemp("", "sheytan-interop-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	tools.SetBaseDir(dir)

	// files.write with relative path → lands in base dir
	files := tools.Files{}
	out, err := files.Run(context.Background(), json.RawMessage(
		`{"action":"write","path":"interop.csv","content":"a,b\n1,2\n3,4\n"}`))
	if err != nil {
		return fmt.Errorf("files.write: %w", err)
	}
	if !strings.Contains(out, filepath.Join(dir, "interop.csv")) {
		return fmt.Errorf("write did not resolve into base dir: %s", out)
	}

	// shell with no cwd → runs in base dir; relative read works
	shell := tools.Shell{}
	shOut, err := shell.Run(context.Background(), json.RawMessage(
		`{"command":"cat interop.csv"}`))
	if err != nil {
		return fmt.Errorf("shell cat: %w (out=%s)", err, shOut)
	}
	if !strings.Contains(shOut, "1,2") {
		return fmt.Errorf("shell did not see files-written interop.csv (cwd mismatch): %q", shOut)
	}

	// dataAnalysis reads the same relative path
	cfg := config.Default()
	cfg.DataDir = dir
	dt := tools.NewDataTool(cfg)
	daOut, err := dt.Run(context.Background(), json.RawMessage(
		`{"action":"profile","path":"interop.csv"}`))
	if err != nil {
		return fmt.Errorf("dataAnalysis.profile: %w", err)
	}
	if !strings.Contains(daOut, "2 rows") {
		return fmt.Errorf("profile did not see the same file: %q", clipStress(daOut, 200))
	}
	return nil
}

func clipStress(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// --- data analysis ---

func stressDataAnalysisSuite() error {
	dir, err := os.MkdirTemp("", "sheytan-data-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	_ = cfg.EnsureDirs()
	tools.SetBaseDir(dir)

	csv := "region,units,price\n" +
		"A,10,5\nB,20,3\nA,30,7\nC,5,2\nB,15,4\nA,,9\n"
	if err := os.WriteFile(filepath.Join(dir, "s.csv"), []byte(csv), 0o644); err != nil {
		return err
	}
	dt := tools.NewDataTool(cfg)
	run := func(args string) (string, error) {
		out, err := dt.Run(context.Background(), json.RawMessage(args))
		if err != nil {
			return "", fmt.Errorf("%s: %w", args, err)
		}
		return out, nil
	}

	// stats: units mean = (10+20+30+5+15)/5 = 16
	out, err := run(`{"action":"stats","path":"s.csv"}`)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "16") {
		return fmt.Errorf("units mean 16 not found in stats output: %s", clipStress(out, 300))
	}
	// missing: units has 1 missing of 6
	out, err = run(`{"action":"missing","path":"s.csv"}`)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "16.7%") {
		return fmt.Errorf("units missing 16.7%% not found: %s", clipStress(out, 300))
	}
	// groupby sum: A = 40, B = 35
	out, err = run(`{"action":"groupby","path":"s.csv","by":"region","column":"units","agg":"sum"}`)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "40") || !strings.Contains(out, "35") {
		return fmt.Errorf("groupby sums wrong: %s", clipStress(out, 300))
	}
	// filter numeric: units > 10 → 20,30,15
	out, err = run(`{"action":"filter","path":"s.csv","column":"units","op":">","value":"10"}`)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "3 of 6 rows") {
		return fmt.Errorf("filter count wrong: %s", clipStress(out, 200))
	}
	// correlation with missing values must not panic (the v0.9 bug)
	out, err = run(`{"action":"correlation","path":"s.csv"}`)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "units") || !strings.Contains(out, "price") {
		return fmt.Errorf("correlation matrix incomplete: %s", clipStress(out, 200))
	}
	// sort desc
	out, err = run(`{"action":"sort","path":"s.csv","column":"units","desc":true,"limit":1}`)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "30") {
		return fmt.Errorf("top unit should be 30: %s", clipStress(out, 200))
	}
	// histogram
	_, err = run(`{"action":"histogram","path":"s.csv","column":"units","bins":3}`)
	if err != nil {
		return err
	}
	// convert csv→json
	out, err = run(`{"action":"convert","path":"s.csv","format":"json"}`)
	if err != nil {
		return err
	}
	jdata, err := os.ReadFile(filepath.Join(dir, "s.json"))
	if err != nil {
		return fmt.Errorf("converted json missing: %v (out=%s)", err, clipStress(out, 200))
	}
	var objs []map[string]any
	if err := json.Unmarshal(jdata, &objs); err != nil {
		return fmt.Errorf("converted json invalid: %v", err)
	}
	if len(objs) != 6 {
		return fmt.Errorf("converted json should have 6 records, has %d", len(objs))
	}
	// dataset cache invalidation: rewrite file with fewer rows → profile must see the change
	if err := os.WriteFile(filepath.Join(dir, "s.csv"), []byte("region,units\nA,1\n"), 0o644); err != nil {
		return err
	}
	out, err = run(`{"action":"profile","path":"s.csv"}`)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "1 rows") {
		return fmt.Errorf("cache did not invalidate after rewrite: %s", clipStress(out, 200))
	}
	return nil
}

func stressDataChartRendering() error {
	dir, err := os.MkdirTemp("", "sheytan-chart-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.DataDir = dir
	_ = cfg.EnsureDirs()
	tools.SetBaseDir(dir)

	csv := "cat,val\nx,10\ny,25\nz,15\nw,40\n"
	if err := os.WriteFile(filepath.Join(dir, "c.csv"), []byte(csv), 0o644); err != nil {
		return err
	}
	dt := tools.NewDataTool(cfg)
	for _, kind := range []string{"bar", "line", "pie"} {
		out, err := dt.Run(context.Background(), json.RawMessage(fmt.Sprintf(
			`{"action":"chart","path":"c.csv","chart":%q,"labelCol":"cat","valueCol":"val","name":"test-%s"}`, kind, kind)))
		if err != nil {
			return fmt.Errorf("chart %s: %w", kind, err)
		}
		path := filepath.Join(cfg.ChartsDir(), "test-"+kind+".svg")
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("chart file missing (%s): %v (out=%s)", kind, err, clipStress(out, 200))
		}
		s := string(data)
		if !strings.HasPrefix(s, "<svg") || !strings.Contains(s, "</svg>") || len(data) < 500 {
			return fmt.Errorf("chart %s not valid SVG (%d bytes)", kind, len(data))
		}
	}
	// scatter uses two numeric columns
	csv2 := "a,b\n1,2\n2,4\n3,5\n4,9\n"
	if err := os.WriteFile(filepath.Join(dir, "sc.csv"), []byte(csv2), 0o644); err != nil {
		return err
	}
	out, err := dt.Run(context.Background(), json.RawMessage(
		`{"action":"chart","path":"sc.csv","chart":"scatter","column":"a","column2":"b","name":"test-scatter"}`))
	if err != nil {
		return fmt.Errorf("scatter: %w", err)
	}
	if !strings.Contains(out, "r =") {
		return fmt.Errorf("scatter should report Pearson r: %s", clipStress(out, 200))
	}
	// error cases must be friendly, not panics
	if _, err := dt.Run(context.Background(), json.RawMessage(`{"action":"chart","path":"c.csv","chart":"pie","labelCol":"cat"}`)); err == nil {
		return fmt.Errorf("pie without valueCol should error")
	}
	if _, err := dt.Run(context.Background(), json.RawMessage(`{"action":"stats","path":"no-such-file.csv"}`)); err == nil {
		return fmt.Errorf("stats on missing file should error")
	}
	if _, err := dt.Run(context.Background(), json.RawMessage(`{"action":"bogus","path":"c.csv"}`)); err == nil {
		return fmt.Errorf("bogus action should error")
	}
	return nil
}

func stressDataToolRegistered() error {
	// The runtime stack must register dataAnalysis with all core tools.
	dir, err := os.MkdirTemp("", "sheytan-stack-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	oldDir := os.Getenv("SHEYTAN_DATA_DIR")
	os.Setenv("SHEYTAN_DATA_DIR", dir)
	defer os.Setenv("SHEYTAN_DATA_DIR", oldDir)

	cfg := config.Default()
	_ = cfg.EnsureDirs()
	st := agentrt.NewStack(cfg)
	defer st.Close()
	names := map[string]bool{}
	for _, t := range st.Orch.Tools() {
		names[t.Name()] = true
	}
	for _, want := range []string{"shell", "files", "webSearch", "git", "browser", "dataAnalysis", "memory"} {
		if !names[want] {
			return fmt.Errorf("tool %q not registered (have %v)", want, names)
		}
	}
	// Base dir must be pinned to the app root.
	if tools.BaseDir() != cfg.DataDir {
		return fmt.Errorf("tools.BaseDir=%q want %q", tools.BaseDir(), cfg.DataDir)
	}
	return nil
}

// --- brand ---

func stressBrandParsaetak() error {
	if !strings.Contains(brand.Copyright(), "Parsaetak") {
		return fmt.Errorf("copyright must name Parsaetak: %q", brand.Copyright())
	}
	if !strings.Contains(brand.TrademarkNotice, "Parsaetak") {
		return fmt.Errorf("trademark notice must name Parsaetak: %q", brand.TrademarkNotice)
	}
	if !strings.Contains(brand.LicenseText, "https://github.com/Parsaetak") {
		return fmt.Errorf("license must carry the Parsaetak GitHub URL")
	}
	if !strings.Contains(brand.LicenseText, "SHEYTAN") {
		return fmt.Errorf("license must mention the SHEYTAN trademark")
	}
	if brand.Licensor != "Parsaetak" {
		return fmt.Errorf("licensor = %q", brand.Licensor)
	}
	if !strings.Contains(brand.LicenseName, "Parsaetak") {
		return fmt.Errorf("license name = %q", brand.LicenseName)
	}
	// The shipped LICENSE file must match the in-app license text byte-for-byte.
	data, err := os.ReadFile("LICENSE")
	if err != nil {
		return fmt.Errorf("LICENSE file unreadable: %v", err)
	}
	if strings.TrimSpace(string(data)) != strings.TrimSpace(brand.LicenseText) {
		return fmt.Errorf("LICENSE file drifted from brand.LicenseText — regenerate with scripts/gen-license.go")
	}
	return nil
}
