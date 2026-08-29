package cmd

import (
        "archive/zip"
        "context"
        "encoding/json"
        "fmt"
        "net/http"
        "net/http/httptest"
        "os"
        "path/filepath"
        "strings"
        "time"

        "github.com/sheytan/local-agent/internal/aicontext"
        "github.com/sheytan/local-agent/internal/brand"
        "github.com/sheytan/local-agent/internal/config"
        "github.com/sheytan/local-agent/internal/llm"
        "github.com/sheytan/local-agent/internal/recall"
        "github.com/sheytan/local-agent/internal/sessions"
        "github.com/sheytan/local-agent/internal/tools"
)

// --- v1.0.10 (PRISM) stress scenarios ---

// stressV110Surface: version + signature + context version + the tool
// registry carrying the four new tools.
func stressV110Surface() error {
        // v1.0.11: forward-compatible pin (the v1.0.11 surface test owns
        // the exact-version assertion now).
        if !versionAtLeast(config.AppVersion, "1.0.10") {
                return fmt.Errorf("AppVersion = %q, want >= 1.0.10", config.AppVersion)
        }
        if brand.SignedBy != "Parsa Tak" {
                return fmt.Errorf("SignedBy = %q, want \"Parsa Tak\"", brand.SignedBy)
        }
        if aicontext.ContextVersion != 10 {
                return fmt.Errorf("ContextVersion = %d, want 10", aicontext.ContextVersion)
        }
        // The tool surface: the four v1.0.10 tools construct and answer with
        // the right names/parameters.
        names := []string{}
        for _, t := range []interface{ Name() string }{
                tools.JSONTool{}, tools.ArchiveTool{}, tools.NewFetchTool(), tools.DiffTool{},
        } {
                names = append(names, t.Name())
        }
        if strings.Join(names, ",") != "json,archive,fetch,diff" {
                return fmt.Errorf("new tool names = %v", names)
        }
        for _, t := range []interface{ Parameters() any }{
                tools.JSONTool{}, tools.ArchiveTool{}, tools.NewFetchTool(), tools.DiffTool{},
        } {
                if t.Parameters() == nil {
                        return fmt.Errorf("a v1.0.10 tool has nil parameters")
                }
        }
        return nil
}

// stressV110JSONQuery: dot paths, array indices and [*] wildcards extract
// the right values; missing paths report no match instead of erroring.
func stressV110JSONQuery() error {
        dir := tTempDir("json")
        defer os.RemoveAll(dir)
        tools.SetBaseDir(dir)
        doc := `{
                "store": {"name": "Ember", "open": true},
                "items": [
                        {"id": 1, "name": "ember", "price": 9.5},
                        {"id": 2, "name": "gold", "price": 19.0}
                ],
                "meta": {"depth": {"nested": {"leaf": 42}}}
        }`
        if err := os.WriteFile(filepath.Join(dir, "shop.json"), []byte(doc), 0o644); err != nil {
                return err
        }
        t := tools.JSONTool{}
        run := func(args string) (string, error) {
                return t.Run(context.Background(), json.RawMessage(args))
        }
        // Scalar path.
        out, err := run(`{"action":"query","path":"shop.json","query":"store.name"}`)
        if err != nil {
                return fmt.Errorf("query store.name: %v", err)
        }
        if !strings.Contains(out, `"Ember"`) {
                return fmt.Errorf("store.name wrong: %q", out)
        }
        // Deep nesting.
        out, err = run(`{"action":"query","path":"shop.json","query":"meta.depth.nested.leaf"}`)
        if err != nil || !strings.Contains(out, "42") {
                return fmt.Errorf("deep path wrong: %q (%v)", out, err)
        }
        // Array index.
        out, err = run(`{"action":"query","path":"shop.json","query":"items[1].name"}`)
        if err != nil || !strings.Contains(out, `"gold"`) {
                return fmt.Errorf("items[1].name wrong: %q (%v)", out, err)
        }
        // Wildcard.
        out, err = run(`{"action":"query","path":"shop.json","query":"items[*].price"}`)
        if err != nil || !strings.Contains(out, "9.5") || !strings.Contains(out, "19") {
                return fmt.Errorf("items[*].price wrong: %q (%v)", out, err)
        }
        if !strings.Contains(out, "2 match") {
                return fmt.Errorf("wildcard match count wrong: %q", out)
        }
        // Missing path → graceful no-match.
        out, err = run(`{"action":"query","path":"shop.json","query":"store.nothing"}`)
        if err != nil || !strings.Contains(out, "no match") {
                return fmt.Errorf("missing path must no-match: %q (%v)", out, err)
        }
        return nil
}

// stressV110JSONWhereStats: JSONL row filtering (eq + numeric gt), stats
// key/type profiling, and where-with-dest write-out.
func stressV110JSONWhereStats() error {
        dir := tTempDir("jsonl")
        defer os.RemoveAll(dir)
        tools.SetBaseDir(dir)
        var b strings.Builder
        for i := 0; i < 20; i++ {
                level := "info"
                if i%5 == 0 {
                        level = "error"
                }
                fmt.Fprintf(&b, `{"id":%d,"level":"%s","ms":%d}`+"\n", i, level, i*100)
        }
        if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(b.String()), 0o644); err != nil {
                return err
        }
        t := tools.JSONTool{}
        // where eq: 4 error rows (i=0,5,10,15).
        out, err := t.Run(context.Background(), json.RawMessage(`{"action":"where","path":"events.jsonl","field":"level","op":"eq","value":"error"}`))
        if err != nil {
                return fmt.Errorf("where eq: %v", err)
        }
        if !strings.Contains(out, "4 row") {
                return fmt.Errorf("where eq count wrong: %q", out)
        }
        // where gt (numeric): ms > 1500 → id 16..19 = 4 rows.
        out, err = t.Run(context.Background(), json.RawMessage(`{"action":"where","path":"events.jsonl","field":"ms","op":"gt","value":"1500"}`))
        if err != nil || !strings.Contains(out, "4 row") {
                return fmt.Errorf("where gt wrong: %q (%v)", out, err)
        }
        // where + dest write-out.
        out, err = t.Run(context.Background(), json.RawMessage(`{"action":"where","path":"events.jsonl","field":"level","op":"eq","value":"error","dest":"errors.jsonl"}`))
        if err != nil || !strings.Contains(out, "errors.jsonl") {
                return fmt.Errorf("where dest wrong: %q (%v)", out, err)
        }
        saved, err := os.ReadFile(filepath.Join(dir, "errors.jsonl"))
        if err != nil || strings.Count(string(saved), "\n") != 4 {
                return fmt.Errorf("dest file wrong (%d lines): %v", strings.Count(string(saved), "\n"), err)
        }
        // stats: keys + types.
        out, err = t.Run(context.Background(), json.RawMessage(`{"action":"stats","path":"events.jsonl"}`))
        if err != nil || !strings.Contains(out, "objects: 20") || !strings.Contains(out, "level") || !strings.Contains(out, "number") {
                return fmt.Errorf("stats wrong: %q (%v)", out, err)
        }
        return nil
}

// stressV110ArchiveRoundtrip: zip → list → unzip restores byte-identical
// content; a crafted zip-slip archive is REJECTED.
func stressV110ArchiveRoundtrip() error {
        dir := tTempDir("arch")
        defer os.RemoveAll(dir)
        tools.SetBaseDir(dir)
        // Source tree.
        if err := os.MkdirAll(filepath.Join(dir, "pack", "sub"), 0o755); err != nil {
                return err
        }
        body := strings.Repeat("sheytan archive payload\n", 50)
        if err := os.WriteFile(filepath.Join(dir, "pack", "a.txt"), []byte(body), 0o644); err != nil {
                return err
        }
        if err := os.WriteFile(filepath.Join(dir, "pack", "sub", "b.bin"), []byte{0, 1, 2, 254, 255}, 0o644); err != nil {
                return err
        }
        t := tools.ArchiveTool{}
        // zip
        out, err := t.Run(context.Background(), json.RawMessage(`{"action":"zip","sources":["pack"],"path":"bundle.zip"}`))
        if err != nil || !strings.Contains(out, "zipped 2 file") {
                return fmt.Errorf("zip wrong: %q (%v)", out, err)
        }
        // list
        out, err = t.Run(context.Background(), json.RawMessage(`{"action":"list","path":"bundle.zip"}`))
        if err != nil || !strings.Contains(out, "a.txt") || !strings.Contains(out, "b.bin") {
                return fmt.Errorf("list wrong: %q (%v)", out, err)
        }
        // unzip
        if _, err := t.Run(context.Background(), json.RawMessage(`{"action":"unzip","path":"bundle.zip","dest":"restored"}`)); err != nil {
                return fmt.Errorf("unzip: %v", err)
        }
        got, err := os.ReadFile(filepath.Join(dir, "restored", "pack", "a.txt"))
        if err != nil || string(got) != body {
                return fmt.Errorf("restored a.txt mismatch (%v)", err)
        }
        gotBin, err := os.ReadFile(filepath.Join(dir, "restored", "pack", "sub", "b.bin"))
        if err != nil || len(gotBin) != 5 {
                return fmt.Errorf("restored b.bin mismatch (%v)", err)
        }
        // tar roundtrip (gzip).
        if _, err := t.Run(context.Background(), json.RawMessage(`{"action":"tar","sources":["pack/a.txt"],"path":"pack.tgz"}`)); err != nil {
                return fmt.Errorf("tar: %v", err)
        }
        if _, err := t.Run(context.Background(), json.RawMessage(`{"action":"untar","path":"pack.tgz","dest":"untarred"}`)); err != nil {
                return fmt.Errorf("untar: %v", err)
        }
        got2, err := os.ReadFile(filepath.Join(dir, "untarred", "pack", "a.txt"))
        if err != nil || string(got2) != body {
                return fmt.Errorf("untar content mismatch (%v)", err)
        }
        // ZIP-SLIP: craft an archive with a traversal entry.
        evil := filepath.Join(dir, "evil.zip")
        zf, err := os.Create(evil)
        if err != nil {
                return err
        }
        zw := zip.NewWriter(zf)
        w, _ := zw.Create("../escaped.txt")
        _, _ = w.Write([]byte("pwn"))
        _ = zw.Close()
        _ = zf.Close()
        if _, err := t.Run(context.Background(), json.RawMessage(`{"action":"unzip","path":"evil.zip","dest":"safe"}`)); err == nil {
                return fmt.Errorf("zip-slip archive must be rejected")
        }
        if _, err := os.Stat(filepath.Join(dir, "escaped.txt")); err == nil {
                return fmt.Errorf("zip-slip wrote OUTSIDE the destination")
        }
        return nil
}

// stressV110FetchText: HTML → readable text (scripts stripped, entities
// decoded, case preserved), size cap enforced, non-http rejected.
func stressV110FetchText() error {
        srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                switch r.URL.Path {
                case "/page":
                        w.Header().Set("Content-Type", "text/html; charset=utf-8")
                        fmt.Fprint(w, `<html><head><title>T</title><style>.x{}</style></head>
<body><script>var x="NOISE";</script><h1>Ember Report</h1><p>Revenue grew &amp; profit &quot;doubled&quot;.</p>
<p>Second paragraph.</p></body></html>`)
                case "/json":
                        w.Header().Set("Content-Type", "application/json")
                        fmt.Fprint(w, `{"ok":true,"items":[1,2,3]}`)
                case "/big":
                        w.Header().Set("Content-Type", "text/plain")
                        _, _ = w.Write([]byte(strings.Repeat("x", 2<<20))) // 2 MB
                default:
                        http.NotFound(w, r)
                }
        }))
        defer srv.Close()

        f := tools.NewFetchTool()
        run := func(args string) (string, error) {
                return f.Run(context.Background(), json.RawMessage(args))
        }
        // HTML page → text.
        out, err := run(fmt.Sprintf(`{"url":%q}`, srv.URL+"/page"))
        if err != nil {
                return fmt.Errorf("fetch page: %v", err)
        }
        if !strings.Contains(out, "Ember Report") || !strings.Contains(out, `profit "doubled"`) {
                return fmt.Errorf("text extraction wrong: %q", out)
        }
        if strings.Contains(out, "NOISE") || strings.Contains(out, "<p>") {
                return fmt.Errorf("script tags leaked: %q", out)
        }
        // JSON passthrough.
        out, err = run(fmt.Sprintf(`{"url":%q}`, srv.URL+"/json"))
        if err != nil || !strings.Contains(out, `"ok":true`) {
                return fmt.Errorf("json passthrough wrong: %q (%v)", out, err)
        }
        // Size cap.
        out, err = run(fmt.Sprintf(`{"url":%q,"maxBytes":4096}`, srv.URL+"/big"))
        if err != nil || !strings.Contains(out, "truncated") {
                return fmt.Errorf("cap must truncate: %q (%v)", firstN(out, 80), err)
        }
        // Non-http rejected.
        if _, err := run(`{"url":"file:///etc/passwd"}`); err == nil {
                return fmt.Errorf("file:// must be rejected")
        }
        return nil
}

func firstN(s string, n int) string {
        if len(s) <= n {
                return s
        }
        return s[:n]
}

// stressV110DiffLines: Myers diff catches modify/insert/delete, identical
// files report 100% similarity, and unrelated files fall back cleanly.
func stressV110DiffLines() error {
        dir := tTempDir("diff")
        defer os.RemoveAll(dir)
        tools.SetBaseDir(dir)
        base := "alpha\nbravo\ncharlie\ndelta\necho\nfoxtrot\ngolf\nhotel\n"
        mod := "alpha\nbravo\nCHARLIE\ndelta\necho\nfoxtrot\ngolf\nhotel\nindia\n"

        if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(base), 0o644); err != nil {
                return err
        }
        if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte(mod), 0o644); err != nil {
                return err
        }
        d := tools.DiffTool{}
        out, err := d.Run(context.Background(), json.RawMessage(`{"path":"a.txt","path2":"b.txt"}`))
        if err != nil {
                return fmt.Errorf("diff: %v", err)
        }
        if !strings.Contains(out, "- charlie") || !strings.Contains(out, "+ CHARLIE") || !strings.Contains(out, "+ india") {
                return fmt.Errorf("diff regions wrong: %q", out)
        }
        if !strings.Contains(out, "1 removed") || !strings.Contains(out, "2 added") {
                return fmt.Errorf("diff counts wrong: %q", out)
        }
        // Identical files.
        if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte(base), 0o644); err != nil {
                return err
        }
        out, err = d.Run(context.Background(), json.RawMessage(`{"path":"a.txt","path2":"c.txt"}`))
        if err != nil || !strings.Contains(out, "100.0% similar") {
                return fmt.Errorf("identical files wrong: %q (%v)", out, err)
        }
        // Wildly different files → bounded fallback (no hang, no giant output).
        var x, y strings.Builder
        for i := 0; i < 300; i++ {
                fmt.Fprintf(&x, "line-a-%d\n", i)
                fmt.Fprintf(&y, "line-b-%d\n", i)
        }
        if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte(x.String()), 0o644); err != nil {
                return err
        }
        if err := os.WriteFile(filepath.Join(dir, "y.txt"), []byte(y.String()), 0o644); err != nil {
                return err
        }
        deadline := time.Now().Add(5 * time.Second)
        out, err = d.Run(context.Background(), json.RawMessage(`{"path":"x.txt","path2":"y.txt"}`))
        if time.Now().After(deadline) {
                return fmt.Errorf("diff on different files too slow")
        }
        if err != nil {
                return fmt.Errorf("fallback diff errored: %v", err)
        }
        if !strings.Contains(out, "similar") && !strings.Contains(out, "overlap") && !strings.Contains(out, "unchanged") {
                return fmt.Errorf("fallback summary missing: %q", firstN(out, 120))
        }
        return nil
}

// stressV110SessionsSidecar: activity entries live in the append-only
// sidecar (NOT in the session JSON), Get() merges them back, Delete
// removes them, and the legacy inline format migrates on read.
func stressV110SessionsSidecar() error {
        dir := tTempDir("sidecar")
        defer os.RemoveAll(dir)
        st := sessions.New(dir)
        s := st.Create()
        if _, err := st.AppendMessage(s.ID, jsonMsg("user", "hi")); err != nil {
                return err
        }
        for i := 0; i < 5; i++ {
                if err := st.AppendActivity(s.ID, sessions.ActivityEntry{Type: "tool_start", Caption: fmt.Sprintf("t%d", i)}); err != nil {
                        return err
                }
        }
        // The session JSON must NOT contain the activity entries anymore.
        data, err := os.ReadFile(filepath.Join(dir, s.ID+".json"))
        if err != nil {
                return err
        }
        if strings.Contains(string(data), "tool_start") {
                return fmt.Errorf("activities leaked into the session JSON")
        }
        // The sidecar exists and carries 5 lines.
        side, err := os.ReadFile(filepath.Join(dir, s.ID+".activities.jsonl"))
        if err != nil || strings.Count(string(side), "\n") != 5 {
                return fmt.Errorf("sidecar wrong (%d lines): %v", strings.Count(string(side), "\n"), err)
        }
        // Get merges them back.
        full, err := st.Get(s.ID)
        if err != nil {
                return err
        }
        if len(full.Activities) != 5 || full.Activities[0].Caption != "t0" {
                return fmt.Errorf("Get merge wrong: %d entries", len(full.Activities))
        }
        // Legacy migration: a v1.0.9-style file with inline activities moves
        // them to the sidecar on read.
        legacy := `{"id":"legacy1","title":"old","messages":[{"role":"user","content":"q"}],"activities":[{"type":"plan","caption":"p1","ts":"2026-01-01T00:00:00Z"}]}`
        if err := os.WriteFile(filepath.Join(dir, "legacy1.json"), []byte(legacy), 0o644); err != nil {
                return err
        }
        if _, err := st.Get("legacy1"); err != nil {
                return fmt.Errorf("legacy read: %v", err)
        }
        migrated, err := os.ReadFile(filepath.Join(dir, "legacy1.activities.jsonl"))
        if err != nil || !strings.Contains(string(migrated), "p1") {
                return fmt.Errorf("legacy activities did not migrate: %v", err)
        }
        // Delete cleans the sidecar too.
        if err := st.Delete(s.ID); err != nil {
                return err
        }
        if _, err := os.Stat(filepath.Join(dir, s.ID+".activities.jsonl")); !os.IsNotExist(err) {
                return fmt.Errorf("sidecar survived delete")
        }
        return nil
}

func jsonMsg(role, content string) llm.Message {
        return llm.Message{Role: role, Content: content}
}

// stressV110RecallCache: the BM25 corpus cache returns IDENTICAL results
// across repeated searches and invalidates when a new capsule lands.
func stressV110RecallCache() error {
        dir := tTempDir("recall")
        defer os.RemoveAll(dir)
        e := recall.New(dir)
        topics := []string{
                "golang goroutine leak debugging buffered channel",
                "python pandas dataframe merge on key column",
                "windows job object memory limit assign process",
                "csv parsing quoted fields rfc 4180 escape",
                "react virtual list rendering performance",
                "bm25 scoring idf term frequency ranking",
        }
        for i, topic := range topics {
                if err := e.IndexTurn(fmt.Sprintf("s%d", i), "t", "how do I handle "+topic, "use the documented approach for "+topic, nil); err != nil {
                        return err
                }
        }
        q := "job object memory limit"
        first := e.Search(q, 3)
        if len(first) == 0 {
                return fmt.Errorf("search returned nothing")
        }
        for i := 0; i < 5; i++ {
                again := e.Search(q, 3)
                if len(again) != len(first) {
                        return fmt.Errorf("cached search length diverged (%d vs %d)", len(again), len(first))
                }
                for j := range first {
                        if again[j].ID != first[j].ID {
                                return fmt.Errorf("cached search order diverged at %d", j)
                        }
                }
        }
        // Invalidation: a new capsule on the SAME topic must be able to win.
        if err := e.IndexTurn("s9", "t", "windows job object assign by pid", "openprocess then assign", nil); err != nil {
                return err
        }
        after := e.Search(q, 3)
        if len(after) == 0 || after[0].ID == first[0].ID && after[0].SessionID == first[0].SessionID && len(after) == len(first) {
                // Same top hit is fine — but the corpus stats MUST have been
                // recomputed (id count changed). Verify by searching the new
                // capsule's unique terms.
                fresh := e.Search("openprocess assign pid", 2)
                if len(fresh) == 0 || fresh[0].SessionID != "s9" {
                        return fmt.Errorf("new capsule not searchable after invalidation")
                }
        }
        return nil
}

// stressV110AicontextV10: the AI briefing teaches the four new tools and
// the live tool list carries them.
func stressV110AicontextV10() error {
        if aicontext.ContextVersion != 10 {
                return fmt.Errorf("ContextVersion = %d, want 10", aicontext.ContextVersion)
        }
        body := aicontext.SystemMessage(config.Default())
        for _, want := range []string{"json", "archive", "fetch", "diff", "items[*].name", "bundle.zip"} {
                if !strings.Contains(body, want) {
                        return fmt.Errorf("AI context missing %q", want)
                }
        }
        return nil
}
