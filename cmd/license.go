package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sheytan/local-agent/internal/brand"
	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/logging"
	"github.com/sheytan/local-agent/internal/sysinfo"
)

// License prints the SHEYTAN™ trademark + full license text.
func License(cfg *config.Config) int {
	fmt.Printf("%s — %s\n", brand.Trademark, brand.FullName)
	fmt.Println(brand.Copyright())
	fmt.Println(brand.TrademarkNotice)
	fmt.Print("\n")
	fmt.Print(brand.LicenseText)
	return 0
}

// Logs prints the tail of the app log and the structured-log stats — the
// "useful data for updates" the log catcher collects.
func Logs(cfg *config.Config, args []string) int {
	n := 50
	if len(args) > 0 {
		fmt.Sscanf(args[0], "%d", &n)
	}
	m := logging.Default()
	if !m.Enabled() {
		// Boot a read-only manager over the log dir.
		rm, err := logging.New(cfg.LogsDir())
		if err != nil {
			fmt.Fprintln(os.Stderr, "logs:", err)
			return 1
		}
		m = rm
	}
	fmt.Printf("SHEYTAN™ log catcher — %s\n\n", cfg.LogsDir())

	fmt.Println("── recent app.log ──")
	lines := readTail(filepath.Join(cfg.LogsDir(), "app.log"), n)
	for _, l := range lines {
		fmt.Println(l)
	}

	fmt.Println("\n── aggregated stats (from tools.jsonl + llm.jsonl) ──")
	stats := m.ComputeStats()
	statsJSON, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Println(string(statsJSON))
	return 0
}

// Diagnostics exports the full diagnostics zip (logs + stats + redacted
// config + sysinfo) for bug reports / update analysis.
func Diagnostics(cfg *config.Config, args []string) int {
	m := logging.Default()
	if !m.Enabled() {
		rm, err := logging.New(cfg.LogsDir())
		if err != nil {
			fmt.Fprintln(os.Stderr, "logs:", err)
			return 1
		}
		m = rm
	}
	out := filepath.Join(cfg.DataDir, "diagnostics", "sheytan-diagnostics.zip")
	if len(args) > 0 && args[0] != "" {
		out = args[0]
	}
	// sysinfo snapshot
	extra := map[string]string{}
	if data, err := json.MarshalIndent(sysinfo.Probe(), "", "  "); err == nil {
		tmp := filepath.Join(os.TempDir(), "sheytan-sysinfo.json")
		if os.WriteFile(tmp, data, 0o644) == nil {
			extra["sysinfo.json"] = tmp
		}
	}
	path, err := m.Diagnostics(out, cfg.ConfigPath(), extra)
	if err != nil {
		fmt.Fprintln(os.Stderr, "diagnostics:", err)
		return 1
	}
	fmt.Println("Diagnostics bundle written to:", path)
	fmt.Println("  (logs, stats, crashes, sysinfo, redacted config)")
	return 0
}

// readTail returns up to n final lines of a file.
func readTail(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{"(no app.log yet)"}
	}
	lines := splitLines(string(data))
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
