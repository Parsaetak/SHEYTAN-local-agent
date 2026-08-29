// Package tools — fetch: lightweight HTTP reader for the agent.
//
// The browser tool drives a real Chromium (heavy, slow); webSearch only
// returns snippets. fetch is the middle ground: a single bounded GET that
// returns readable TEXT (script/style stripped, entities decoded) or raw
// bytes (mode=raw), size-capped, streaming, with a sane timeout. The
// classic research chain is webSearch → fetch the promising URL →
// files/dataAnalysis to persist and analyze what it found.
package tools

import (
        "context"
        "encoding/json"
        "fmt"
        "html"
        "io"
        "net/http"
        "strings"
        "time"
)

// FetchTool reads URLs.
type FetchTool struct {
        client *http.Client
}

// NewFetchTool builds a fetch tool with its own bounded client.
func NewFetchTool() *FetchTool {
        return &FetchTool{
                client: &http.Client{
                        Timeout: 30 * time.Second,
                },
        }
}

// Name implements the agent tool interface.
func (t *FetchTool) Name() string { return "fetch" }

// Description implements the agent tool interface.
func (t *FetchTool) Description() string {
        return "Fetch a URL and return readable text (HTML stripped to content) or raw bytes. " +
                `mode:"text" (default) | "raw". maxBytes caps the body (default 512 KB, max 4 MB). ` +
                "Much faster than the browser tool for reading pages; pair with webSearch (find) → fetch (read) → files (save)."
}

// Parameters implements the agent tool interface.
func (t *FetchTool) Parameters() any {
        return struct {
                URL      string `json:"url"`
                Mode     string `json:"mode,omitempty"`
                MaxBytes int    `json:"maxBytes,omitempty"`
        }{}
}

const (
        fetchDefaultCap = 512 << 10 // 512 KB
        fetchMaxCap     = 4 << 20   // 4 MB
        fetchTimeout    = 30 * time.Second
)

// Run implements the agent tool interface.
func (t *FetchTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
        var p struct {
                URL      string `json:"url"`
                Mode     string `json:"mode"`
                MaxBytes int    `json:"maxBytes"`
        }
        if err := json.Unmarshal(args, &p); err != nil {
                return "", fmt.Errorf("bad args: %w", err)
        }
        if p.URL == "" {
                return "", fmt.Errorf("url is required")
        }
        if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
                return "", fmt.Errorf("only http(s) URLs are supported (got %q)", p.URL)
        }
        bodyCap := fetchDefaultCap
        if p.MaxBytes > 0 {
                bodyCap = p.MaxBytes
                if bodyCap > fetchMaxCap {
                        bodyCap = fetchMaxCap
                }
        }
        cctx, cancel := context.WithTimeout(ctx, fetchTimeout)
        defer cancel()
        req, err := http.NewRequestWithContext(cctx, http.MethodGet, p.URL, nil)
        if err != nil {
                return "", err
        }
        req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SHEYTAN-Local-Agent/1.10; +local)")
        req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain,*/*")
        resp, err := t.client.Do(req)
        if err != nil {
                return "", fmt.Errorf("fetch: %w", err)
        }
        defer resp.Body.Close()

        // Stream with cap: a 1 GB response never enters memory.
        limited := io.LimitReader(resp.Body, int64(bodyCap)+1)
        body, err := io.ReadAll(limited)
        if err != nil {
                return "", fmt.Errorf("read body: %w", err)
        }
        truncated := len(body) > bodyCap
        if truncated {
                body = body[:bodyCap]
        }

        var b strings.Builder
        fmt.Fprintf(&b, "status: %s | content-type: %s | %d bytes%s\n",
                resp.Status, resp.Header.Get("Content-Type"), len(body), truncMark(truncated))
        ctype := strings.ToLower(resp.Header.Get("Content-Type"))
        isHTML := strings.Contains(ctype, "html") ||
                (!strings.Contains(ctype, "json") && !strings.Contains(ctype, "text/plain") && strings.HasPrefix(strings.TrimSpace(string(body)), "<"))
        if strings.EqualFold(p.Mode, "raw") {
                b.Write(body)
                return b.String(), nil
        }
        if isHTML {
                text := htmlPageToText(string(body))
                if len(text) > bodyCap {
                        text = text[:bodyCap]
                }
                b.WriteString(text)
                return b.String(), nil
        }
        b.Write(body)
        return b.String(), nil
}

func truncMark(t bool) string {
        if t {
                return " (truncated at cap)"
        }
        return ""
}

// htmlPageToText strips the chrome from an HTML document and returns
// readable text: script/style/head noise removed, tags dropped, entities
// decoded, whitespace squeezed (but paragraph structure preserved — this
// is the full-page reader, unlike the snippet-oriented htmlToText).
func htmlPageToText(doc string) string {
        s := doc
        // Remove the invisible sections wholesale (case-insensitive).
        for _, section := range []string{"script", "style", "noscript", "svg", "head"} {
                s = removeTagBlock(s, section)
        }
        // Comments.
        s = stripBetween(s, "<!--", "-->")
        // Block boundaries -> newlines (before tag surgery loses them);
        // case-insensitive without lowercasing the content itself.
        for _, tag := range []string{"</p>", "<br>", "<br/>", "<br />", "</div>", "</li>", "</tr>", "</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>", "</pre>", "</table>"} {
                s = replaceIgnoreCase(s, tag, "\n")
        }
        // Strip remaining tags.
        var b strings.Builder
        inTag := false
        for i := 0; i < len(s); i++ {
                switch s[i] {
                case '<':
                        inTag = true
                case '>':
                        if inTag {
                                inTag = false
                                b.WriteByte(' ')
                        }
                default:
                        if !inTag {
                                b.WriteByte(s[i])
                        }
                }
        }
        text := html.UnescapeString(b.String())
        // Squeeze runs of blank lines; trim trailing spaces.
        lines := strings.Split(text, "\n")
        out := make([]string, 0, len(lines))
        blank := 0
        for _, ln := range lines {
                ln = strings.TrimSpace(ln)
                if ln == "" {
                        blank++
                        if blank > 1 {
                                continue
                        }
                } else {
                        blank = 0
                }
                out = append(out, ln)
        }
        return strings.TrimSpace(strings.Join(out, "\n"))
}

// replaceIgnoreCase replaces case-insensitive occurrences of old with
// neu without changing the case of the surrounding text.
func replaceIgnoreCase(s, old, neu string) string {
        lower := strings.ToLower(s)
        oldLower := strings.ToLower(old)
        var b strings.Builder
        i := 0
        for {
                idx := strings.Index(lower[i:], oldLower)
                if idx < 0 {
                        b.WriteString(s[i:])
                        return b.String()
                }
                idx += i
                b.WriteString(s[i:idx])
                b.WriteString(neu)
                i = idx + len(old)
        }
}

// removeTagBlock removes <tag ...>...</tag> (case-insensitive) including
// content.
func removeTagBlock(s, tag string) string {
        lower := strings.ToLower(s)
        var b strings.Builder
        i := 0
        for {
                start := strings.Index(lower[i:], "<"+tag)
                if start < 0 {
                        b.WriteString(s[i:])
                        return b.String()
                }
                start += i
                b.WriteString(s[i:start])
                end := strings.Index(lower[start:], "</"+tag+">")
                if end < 0 {
                        // Unterminated block: drop the rest.
                        return b.String()
                }
                i = start + end + len(tag) + 3
        }
}

func stripBetween(s, open, close string) string {
        for {
                i := strings.Index(s, open)
                if i < 0 {
                        return s
                }
                j := strings.Index(s[i:], close)
                if j < 0 {
                        return s[:i]
                }
                s = s[:i] + s[i+j+len(close):]
        }
}
