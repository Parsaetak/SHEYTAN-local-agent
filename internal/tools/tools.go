// Package tools contains the built-in agent tools: shell, files, code-exec,
// web-search, git, browser. All tools implement the agent.Tool interface.
package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/sheytan/local-agent/internal/logging"
	"github.com/sheytan/local-agent/internal/netcheck"
	"github.com/sheytan/local-agent/internal/proc"
)

// OnFileCreated is the optional v1.0.4 artifact hook: the GUI installs a
// callback at boot so every file a tool writes can be reported to the
// artifacts tracker and surfaced in the UI ("created files" chips + Files
// view). Always nil in CLI mode — callers must nil-check.
var OnFileCreated func(path string)

// --- Shell ---

type Shell struct{}

func (Shell) Name() string { return "shell" }
func (Shell) Description() string {
	return "Run a shell command and return stdout+stderr. On Windows commands run via cmd.exe; on macOS/Linux via bash. Default working directory is the app folder (use cwd to change). Relative paths match the files/dataAnalysis tools."
}
func (Shell) Parameters() any {
	return struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd,omitempty"`
		Timeout int    `json:"timeout,omitempty"`
	}{}
}
func (Shell) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("bad args: %w", err)
	}
	if p.Command == "" {
		return "", fmt.Errorf("command is required")
	}
	timeout := 60
	if p.Timeout > 0 {
		timeout = p.Timeout
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// v1.0.4: hidden console — cmd.exe would otherwise flash a
		// terminal window for every shell tool call.
		cmd = proc.CommandContext(cctx, "cmd", "/c", p.Command)
	} else {
		cmd = proc.CommandContext(cctx, "bash", "-c", p.Command)
	}
	if p.Cwd != "" {
		cmd.Dir = ResolvePath(p.Cwd)
	} else {
		// Canonical base = the app folder, so relative paths mean the
		// same thing here as in the files/dataAnalysis tools.
		if base := BaseDir(); base != "" {
			cmd.Dir = base
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil && cctx.Err() == context.DeadlineExceeded {
		return string(out) + "\n[timeout]", nil
	}
	return string(out), err
}

// --- Files ---

type Files struct{}

func (Files) Name() string { return "files" }
func (Files) Description() string {
	return "Read, write, list, or delete files. Actions: read|write|list|delete. Relative paths resolve against the app folder, so a file written here can be read by shell/git/dataAnalysis with the same relative path. Chain: write CSV → dataAnalysis to profile it."
}
func (Files) Parameters() any {
	return struct {
		Action  string `json:"action"`
		Path    string `json:"path"`
		Content string `json:"content,omitempty"`
	}{}
}
func (Files) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Action  string `json:"action"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("bad args: %w", err)
	}
	switch strings.ToLower(p.Action) {
	case "read":
		data, err := os.ReadFile(ResolvePath(p.Path))
		if err != nil {
			return "", err
		}
		return string(data), nil
	case "write":
		dst := ResolvePath(p.Path)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(dst, []byte(p.Content), 0o644); err != nil {
			return "", err
		}
		// v1.0.4: surface the created file in the UI artifacts feed.
		if OnFileCreated != nil {
			OnFileCreated(dst)
		}
		return fmt.Sprintf("wrote %d bytes to %s", len(p.Content), dst), nil
	case "list":
		entries, err := os.ReadDir(ResolvePath(p.Path))
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for _, e := range entries {
			dir := ""
			if e.IsDir() {
				dir = "/"
			}
			fmt.Fprintf(&b, "%s%s\n", e.Name(), dir)
		}
		return b.String(), nil
	case "delete":
		dst := ResolvePath(p.Path)
		return fmt.Sprintf("deleted %s", dst), os.Remove(dst)
	default:
		return "", fmt.Errorf("unknown action %q", p.Action)
	}
}

// --- Code Exec ---

type CodeExec struct{}

func (CodeExec) Name() string { return "codeExec" }
func (CodeExec) Description() string {
	return "Run Python code and return stdout/stderr. Code is written to a temp file and executed with python3."
}
func (CodeExec) Parameters() any {
	return struct {
		Lang    string `json:"lang"`
		Code    string `json:"code"`
		Timeout int    `json:"timeout,omitempty"`
	}{}
}
func (CodeExec) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Lang    string `json:"lang"`
		Code    string `json:"code"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("bad args: %w", err)
	}
	if p.Code == "" {
		return "", fmt.Errorf("code is required")
	}
	timeout := 60
	if p.Timeout > 0 {
		timeout = p.Timeout
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	lang := strings.ToLower(p.Lang)
	if lang == "" {
		lang = "python"
	}
	switch lang {
	case "python", "py":
		py, err := pythonBin()
		if err != nil {
			return "", err
		}
		tmp, err := os.CreateTemp("", "sheytan-*.py")
		if err != nil {
			return "", err
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.WriteString(p.Code); err != nil {
			return "", err
		}
		_ = tmp.Close()
		cmd = proc.CommandContext(cctx, py, tmp.Name())
	case "node", "javascript", "js":
		tmp, err := os.CreateTemp("", "sheytan-*.js")
		if err != nil {
			return "", err
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.WriteString(p.Code); err != nil {
			return "", err
		}
		_ = tmp.Close()
		cmd = proc.CommandContext(cctx, "node", tmp.Name())
	default:
		return "", fmt.Errorf("unsupported language: %s", lang)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// --- Web Search (multi-engine, pure Go — no bash/curl needed) ---
//
// Engine chain: DuckDuckGo html → DuckDuckGo lite → Bing. Each engine is
// tried in order; bot-block pages (DDG "anomaly" challenges, HTTP 202) are
// detected and skipped so the agent always gets real results or a clear
// error. Bing redirect URLs (bing.com/ck/a?...&u=a1<base64>) are decoded
// to the real destination.

type WebSearch struct{}

func (WebSearch) Name() string { return "webSearch" }
func (WebSearch) Description() string {
	return "Search the web (DuckDuckGo with Bing fallback, no API key required) and return the top 5 results with title, snippet, and real URL. Pair with the browser tool to open and read a promising result, and with files/dataAnalysis to save & analyze what you find."
}
func (WebSearch) Parameters() any {
	return struct {
		Query string `json:"query"`
	}{}
}

var (
	reResultA = regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	reSnippet = regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
	reTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reEntity  = regexp.MustCompile(`&[a-z#0-9]+;`)
	// v1.0.1 perf: hoisted from parseDDG — it used to be recompiled on
	// every lite-page parse (regexp.MustCompile scans + builds a machine).
	reLite = regexp.MustCompile(`(?s)<a[^>]*class="result-link"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
)

type searchResult struct {
	title, href, snippet string
}

// ddgRealURL unwraps DuckDuckGo redirect links:
// //duckduckgo.com/l/?uddg=<url-encoded-real-url>&rut=... → the real URL.
func ddgRealURL(href string) string {
	h := strings.TrimSpace(href)
	if strings.HasPrefix(h, "//") {
		h = "https:" + h
	}
	if u, err := url.Parse(h); err == nil {
		if u.Host == "duckduckgo.com" && u.Path == "/l/" {
			if raw := u.Query().Get("uddg"); raw != "" {
				if dec, err := url.QueryUnescape(raw); err == nil {
					return dec
				}
			}
		}
	}
	return h
}

// bingRealURL decodes Bing redirect links:
// https://www.bing.com/ck/a?!&&p=...&u=a1aHR0cHM6... → the real URL.
func bingRealURL(href string) string {
	if !strings.Contains(href, "bing.com/ck/") {
		return strings.ReplaceAll(strings.TrimSpace(href), "&amp;", "&")
	}
	h := strings.ReplaceAll(href, "&amp;", "&")
	u, err := url.Parse(h)
	if err != nil {
		return h
	}
	enc := u.Query().Get("u")
	if enc == "" {
		return h
	}
	enc = strings.TrimPrefix(enc, "a1")
	pad := strings.Repeat("=", (4-len(enc)%4)%4)
	if dec, err := base64.RawURLEncoding.DecodeString(enc + pad); err == nil {
		if s := string(dec); strings.HasPrefix(s, "http") {
			return s
		}
	}
	return h
}

func htmlToText(s string) string {
	s = reTag.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#x27;", "'")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = reEntity.ReplaceAllString(s, "")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

var searchUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36"

func fetchSearchPage(ctx context.Context, endpoint string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", searchUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(data), resp.StatusCode, nil
}

// looksBlocked reports whether a response body is a bot-challenge page.
func looksBlocked(status int, body string) bool {
	if status == 202 || status == 403 || status == 429 {
		return true
	}
	if strings.Contains(body, "anomaly") && strings.Contains(body, "challenge") {
		return true
	}
	if strings.Contains(body, "If this persists, please") && strings.Contains(body, "duckduckgo") {
		return true
	}
	return false
}

// parseDDG extracts results from a DuckDuckGo html/lite results page.
func parseDDG(body string) []searchResult {
	var results []searchResult
	for _, m := range reResultA.FindAllStringSubmatch(body, 8) {
		results = append(results, searchResult{href: ddgRealURL(m[1]), title: htmlToText(m[2])})
	}
	if len(results) == 0 {
		for _, m := range reLite.FindAllStringSubmatch(body, 8) {
			results = append(results, searchResult{href: ddgRealURL(m[1]), title: htmlToText(m[2])})
		}
	}
	snips := reSnippet.FindAllStringSubmatch(body, 8)
	for i := range results {
		if i < len(snips) {
			results[i].snippet = htmlToText(snips[i][1])
		}
	}
	return results
}

var (
	reBingBlock  = regexp.MustCompile(`(?s)<li class="b_algo"(.*)`)
	reBingAnchor = regexp.MustCompile(`(?s)<h2[^>]*>\s*<a[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	reBingSnip   = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
)

// parseBing extracts results from a Bing results page.
func parseBing(body string) []searchResult {
	var results []searchResult
	for _, block := range reBingBlock.FindAllStringSubmatch(body, 8) {
		a := reBingAnchor.FindStringSubmatch(block[1])
		if a == nil {
			continue
		}
		r := searchResult{href: bingRealURL(a[1]), title: htmlToText(a[2])}
		if sn := reBingSnip.FindStringSubmatch(block[1]); sn != nil {
			r.snippet = htmlToText(sn[1])
		}
		if r.title != "" && strings.HasPrefix(r.href, "http") {
			results = append(results, r)
		}
	}
	return results
}

func (WebSearch) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("bad args: %w", err)
	}
	if p.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	// Offline fast-fail: never burn a 25s timeout chain when there is
	// no connectivity — tell the LLM immediately and point it at the
	// tools that still work.
	if netcheck.IsOffline() {
		logging.Default().Warn("webSearch", "offline — skipping search for %q", p.Query)
		return "", fmt.Errorf("webSearch unavailable: no internet connection detected. " +
			"Do not retry web tools this session unless the user says they are back online. " +
			"All local tools still work: files, shell, codeExec, git, dataAnalysis, memory")
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	q := url.QueryEscape(p.Query)
	engines := []struct {
		name  string
		url   string
		parse func(string) []searchResult
	}{
		{"duckduckgo", "https://html.duckduckgo.com/html/?q=" + q, parseDDG},
		{"duckduckgo-lite", "https://lite.duckduckgo.com/lite/?q=" + q, parseDDG},
		{"bing", "https://www.bing.com/search?q=" + q + "&count=10", parseBing},
	}

	var results []searchResult
	var tried []string
	for _, eng := range engines {
		body, status, err := fetchSearchPage(cctx, eng.url)
		if err != nil {
			tried = append(tried, fmt.Sprintf("%s: network error", eng.name))
			logging.Default().Warn("webSearch", "engine %s failed: %v", eng.name, err)
			continue
		}
		if looksBlocked(status, body) {
			tried = append(tried, fmt.Sprintf("%s: blocked (HTTP %d)", eng.name, status))
			logging.Default().Warn("webSearch", "engine %s blocked (HTTP %d)", eng.name, status)
			continue
		}
		results = eng.parse(body)
		if len(results) > 0 {
			logging.Default().Info("webSearch", "query %q via %s → %d results", p.Query, eng.name, len(results))
			break
		}
		tried = append(tried, fmt.Sprintf("%s: no results parsed (HTTP %d)", eng.name, status))
	}

	if len(results) == 0 {
		return "", fmt.Errorf("web search failed for %q (tried: %s) — the network may be blocking search engines; try the browser tool to navigate to a search page directly",
			p.Query, strings.Join(tried, "; "))
	}
	if len(results) > 5 {
		results = results[:5]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Search results for %q:\n\n", p.Query)
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n   URL: %s\n", i+1, r.title, r.href)
		if r.snippet != "" {
			fmt.Fprintf(&b, "   %s\n", clipStr(r.snippet, 200))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func clipStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// --- Git ---

type Git struct{}

func (Git) Name() string { return "git" }
func (Git) Description() string {
	return "Run a git subcommand in a repository. Pass 'repo' (relative paths resolve against the app folder) and 'args' (space-separated)."
}
func (Git) Parameters() any {
	return struct {
		Repo string `json:"repo"`
		Args string `json:"args"`
	}{}
}
func (Git) Run(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Repo string `json:"repo"`
		Args string `json:"args"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("bad args: %w", err)
	}
	if p.Args == "" {
		return "", fmt.Errorf("args is required")
	}
	argList := strings.Fields(p.Args)
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := proc.CommandContext(cctx, "git", argList...)
	if p.Repo != "" {
		cmd.Dir = ResolvePath(p.Repo)
	} else if base := BaseDir(); base != "" {
		cmd.Dir = base
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// pythonBin resolves the Python interpreter across platforms
// (python3 on Unix, python/py on Windows).
func pythonBin() (string, error) {
	if runtime.GOOS == "windows" {
		for _, cand := range []string{"python", "py"} {
			if _, err := exec.LookPath(cand); err == nil {
				return cand, nil
			}
		}
		return "", fmt.Errorf("python not found (install from python.org or the Microsoft Store)")
	}
	if _, err := exec.LookPath("python3"); err == nil {
		return "python3", nil
	}
	if _, err := exec.LookPath("python"); err == nil {
		return "python", nil
	}
	return "", fmt.Errorf("python3 not found")
}
