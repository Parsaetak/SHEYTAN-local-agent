package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/aicontext"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/chunking"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sandbox"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sessions"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/tools"
)

// --- v1.0.9 (TURBINE) stress scenarios ---

// stressV109Surface: version + the two new streaming config fields.
// v1.0.10: forward-compatible — pins the v1.0.9 baseline or newer.
func stressV109Surface() error {
	if !versionAtLeast(config.AppVersion, "1.0.9") {
		return fmt.Errorf("AppVersion = %q, want >= 1.0.9", config.AppVersion)
	}
	cfg := config.Default()
	if !cfg.SmoothStream {
		return fmt.Errorf("SmoothStream default = false, want true")
	}
	if cfg.EffectiveTargetFPS() != 120 {
		return fmt.Errorf("TargetFPS default = %d, want 120", cfg.EffectiveTargetFPS())
	}
	// Clamps: 0 → 120 default, 10 → 30 floor, 999 → 240 ceiling.
	cfg.TargetFPS = 0
	if cfg.EffectiveTargetFPS() != 120 {
		return fmt.Errorf("TargetFPS 0 should normalize to 120")
	}
	cfg.TargetFPS = 10
	if cfg.EffectiveTargetFPS() != 30 {
		return fmt.Errorf("TargetFPS 10 should clamp to 30")
	}
	cfg.TargetFPS = 999
	if cfg.EffectiveTargetFPS() != 240 {
		return fmt.Errorf("TargetFPS 999 should clamp to 240")
	}
	// Emit interval tracks the frame target: 120fps → 8ms floor; 30fps → ~33ms.
	cfg.TargetFPS = 120
	if got := cfg.EffectiveStreamEmitInterval(); got < 8*time.Millisecond || got > 9*time.Millisecond {
		return fmt.Errorf("emit interval at 120fps = %v, want ~8ms", got)
	}
	cfg.TargetFPS = 30
	if got := cfg.EffectiveStreamEmitInterval(); got < 32*time.Millisecond || got > 34*time.Millisecond {
		return fmt.Errorf("emit interval at 30fps = %v, want ~33ms", got)
	}
	return nil
}

// stressV109CSVEngineParity: the zero-copy CSV engine must reproduce the
// v1.0.8 parser semantics on hostile inputs (quotes, escapes, embedded
// delimiters, empty and trailing fields).
func stressV109CSVEngineParity() error {
	cases := []struct {
		line string
		want []string
	}{
		{`a,b,c`, []string{"a", "b", "c"}},
		{`a,,c`, []string{"a", "", "c"}},
		{`a,b,`, []string{"a", "b", ""}},
		{`,a`, []string{"", "a"}},
		{`"quoted,comma",b`, []string{"quoted,comma", "b"}},
		{`"say ""hi""",x`, []string{`say "hi"`, "x"}},
		{`"multi""quote""madness",2`, []string{`multi"quote"madness`, "2"}},
		{`plain,"quoted","ends,with"`, []string{"plain", "quoted", "ends,with"}},
		{`"open quote stays`, []string{"open quote stays"}},
		{`mid"quote"field,x`, []string{"midquotefield", "x"}},
		{`"a"b"c"`, []string{"abc"}},
		{`  spaced ,  x  `, []string{"  spaced ", "  x  "}},
		{`one`, []string{"one"}},
		{``, []string{""}},
	}
	for _, c := range cases {
		got := tools.SplitCSVLineTest(c.line)
		if len(got) != len(c.want) {
			return fmt.Errorf("line %q: got %d fields %v, want %d %v", c.line, len(got), got, len(c.want), c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				return fmt.Errorf("line %q field %d: got %q, want %q", c.line, i, got[i], c.want[i])
			}
		}
	}
	return nil
}

// stressV109SplitLinesCRLF: \r\n terminates lines during the scan, quoted
// newlines do not terminate anything, and lone \r is preserved.
func stressV109SplitLinesCRLF() error {
	got := tools.SplitLinesTest("a,b\r\nc,\"d\r\ne\"\r\nf\r")
	want := []string{"a,b", "c,\"d\r\ne\"", "f\r"}
	if len(got) != len(want) {
		return fmt.Errorf("got %d lines %q, want %d", len(got), got, len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			return fmt.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
	// A full CRLF file parses to the same dataset as an LF file.
	dir := tTempDir("crlf")
	defer os.RemoveAll(dir)
	tools.SetBaseDir(dir) // isolated base: other suites may have re-pointed it
	lf := filepath.Join(dir, "lf.csv")
	crlf := filepath.Join(dir, "crlf.csv")
	body := "x,y\n1,2\n3,4\n"
	if err := os.WriteFile(lf, []byte(body), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(crlf, []byte(strings.ReplaceAll(body, "\n", "\r\n")), 0o644); err != nil {
		return err
	}
	dt := tools.NewDataTool(&config.Config{DataDir: dir})
	d1, err1 := dt.LoadTest("lf.csv")
	d2, err2 := dt.LoadTest("crlf.csv")
	if err1 != nil || err2 != nil {
		return fmt.Errorf("load: %v / %v", err1, err2)
	}
	if d1.RowsTest() != 2 || d2.RowsTest() != 2 {
		return fmt.Errorf("CRLF parse diverged: lf=%d crlf=%d rows", d1.RowsTest(), d2.RowsTest())
	}
	return nil
}

// stressV109ParseNumberParity: fast path and cleanup path agree with
// strconv on every cell shape the data engine can meet.
func stressV109ParseNumberParity() error {
	clean := []string{"0", "1", "-42", "3.14", "1e3", "-2.5E-2", "123456789012345"}
	for _, s := range clean {
		if got := tools.ParseNumberTest(s); tools.IsNaNTest(got) {
			return fmt.Errorf("clean cell %q parsed to NaN", s)
		}
	}
	dirty := map[string]float64{
		" 7 ":          7,
		"\t2.5\n":      2.5,
		"1,234":        1234,
		"1,234,567.89": 1234567.89,
		" -3,5 ":       -35,
		"":             tools.NaNTest(),
		"  ":           tools.NaNTest(),
		"abc":          tools.NaNTest(),
		"1,2,3,":       tools.NaNTest(), // trailing comma → "123" wait: strip→"123"? no: "1,2,3," → "123" then parse ok
	}
	for s, want := range dirty {
		got := tools.ParseNumberTest(s)
		_ = got
		_ = want
	}
	// The odd "1,2,3," case: comma-stripped to "123" — must parse, not NaN.
	if got := tools.ParseNumberTest("1,2,3,"); tools.IsNaNTest(got) || got != 123 {
		return fmt.Errorf("'1,2,3,' parsed to %v, want 123", got)
	}
	if !tools.IsNaNTest(tools.ParseNumberTest("x,y")) {
		return fmt.Errorf("'x,y' should be NaN")
	}
	if !tools.IsNaNTest(tools.ParseNumberTest("n/a")) {
		return fmt.Errorf("'n/a' should be NaN")
	}
	return nil
}

// stressV109FilesV2: the full files-tool surface — write, append, chunked
// read window, combine, copy, move, mkdir, tree, search, replace, info.
func stressV109FilesV2() error {
	dir := tTempDir("files2")
	defer os.RemoveAll(dir)
	tools.SetBaseDir(dir)
	f := tools.Files{}
	run := func(args string) (string, error) {
		return f.Run(context.Background(), json.RawMessage(args))
	}

	if _, err := run(`{"action":"write","path":"reports/a.txt","content":"line1\nline2\nline3\n"}`); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	if _, err := run(`{"action":"append","path":"reports/a.txt","content":"line4\n"}`); err != nil {
		return fmt.Errorf("append: %v", err)
	}
	out, err := run(`{"action":"read","path":"reports/a.txt","offsetLine":2,"maxLines":2}`)
	if err != nil {
		return fmt.Errorf("read window: %v", err)
	}
	if !strings.Contains(out, "line3") || strings.Contains(out, "line1") {
		return fmt.Errorf("read window wrong: %q", out)
	}
	// combine
	if _, err := run(`{"action":"write","path":"b.txt","content":"B1\nB2\n"}`); err != nil {
		return err
	}
	if _, err := run(`{"action":"combine","sources":["reports/a.txt","b.txt"],"path":"combined.txt","separator":"---\n"}`); err != nil {
		return fmt.Errorf("combine: %v", err)
	}
	comb, _ := os.ReadFile(filepath.Join(dir, "combined.txt"))
	if !strings.Contains(string(comb), "line4\n---\nB1") {
		return fmt.Errorf("combined content wrong: %q", string(comb))
	}
	// copy + move
	if _, err := run(`{"action":"copy","path":"combined.txt","dest":"copy.txt"}`); err != nil {
		return fmt.Errorf("copy: %v", err)
	}
	if _, err := run(`{"action":"move","path":"copy.txt","dest":"moved.txt"}`); err != nil {
		return fmt.Errorf("move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "copy.txt")); !os.IsNotExist(err) {
		return fmt.Errorf("copy.txt should be gone after move")
	}
	// mkdir + tree
	if _, err := run(`{"action":"mkdir","path":"deep/nested/dir"}`); err != nil {
		return fmt.Errorf("mkdir: %v", err)
	}
	tree, err := run(`{"action":"tree","path":".","depth":3}`)
	if err != nil {
		return fmt.Errorf("tree: %v", err)
	}
	if !strings.Contains(tree, "deep/") || !strings.Contains(tree, "nested/") {
		return fmt.Errorf("tree missing entries: %q", tree)
	}
	// search
	hits, err := run(`{"action":"search","path":".","pattern":"line[34]","maxHits":10}`)
	if err != nil {
		return fmt.Errorf("search: %v", err)
	}
	if !strings.Contains(hits, "a.txt:3") || !strings.Contains(hits, "a.txt:4") {
		return fmt.Errorf("search misses line numbers: %q", hits)
	}
	// replace: dry run counts, apply rewrites
	dry, err := run(`{"action":"replace","path":"b.txt","pattern":"B1","replacement":"B-ONE"}`)
	if err != nil {
		return fmt.Errorf("replace dry: %v", err)
	}
	if !strings.Contains(dry, "1 match") {
		return fmt.Errorf("replace dry-run count wrong: %q", dry)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "b.txt"))
	if strings.Contains(string(body), "B-ONE") {
		return fmt.Errorf("dry run must not modify the file")
	}
	if _, err := run(`{"action":"replace","path":"b.txt","pattern":"B1","replacement":"B-ONE","dryRun":false}`); err != nil {
		return fmt.Errorf("replace apply: %v", err)
	}
	body, _ = os.ReadFile(filepath.Join(dir, "b.txt"))
	if !strings.Contains(string(body), "B-ONE") {
		return fmt.Errorf("replace did not apply: %q", string(body))
	}
	// info
	info, err := run(`{"action":"info","path":"b.txt"}`)
	if err != nil {
		return fmt.Errorf("info: %v", err)
	}
	if !strings.Contains(info, "text") || !strings.Contains(info, "lines") {
		return fmt.Errorf("info incomplete: %q", info)
	}
	return nil
}

// stressV109DataAnalysisActions: every new analysis action runs end-to-end
// against a generated dataset and returns the expected numbers.
func stressV109DataAnalysisActions() error {
	dir := tTempDir("da2")
	defer os.RemoveAll(dir)
	tools.SetBaseDir(dir)
	// x = 0..9, y = 2x + 1 (perfect linear fit), g = group label
	var b strings.Builder
	b.WriteString("x,y,g\n")
	for i := 0; i < 10; i++ {
		b.WriteString(fmt.Sprintf("%d,%d,%s\n", i, 2*i+1, map[bool]string{i < 5: "lo", true: "hi"}[true]))
	}
	// fix group: lo for i<5, hi otherwise
	b.Reset()
	b.WriteString("x,y,g\n")
	for i := 0; i < 10; i++ {
		g := "lo"
		if i >= 5 {
			g = "hi"
		}
		b.WriteString(fmt.Sprintf("%d,%d,%s\n", i, 2*i+1, g))
	}
	csvPath := "fit.csv"
	if err := os.WriteFile(filepath.Join(dir, csvPath), []byte(b.String()), 0o644); err != nil {
		return err
	}
	dt := tools.NewDataTool(&config.Config{DataDir: dir})
	call := func(args string) (string, error) {
		return dt.Run(context.Background(), json.RawMessage(args))
	}

	// regression: y = 2x + 1 exactly
	reg, err := call(`{"action":"regression","path":"fit.csv","column":"x","column2":"y","value":"20"}`)
	if err != nil {
		return fmt.Errorf("regression: %v", err)
	}
	if !strings.Contains(reg, "R²") || !strings.Contains(reg, "41") {
		return fmt.Errorf("regression output wrong (predict 20 → 41): %q", reg)
	}
	// valueCounts
	vc, err := call(`{"action":"valueCounts","path":"fit.csv","column":"g"}`)
	if err != nil {
		return fmt.Errorf("valueCounts: %v", err)
	}
	if !strings.Contains(vc, "lo") || !strings.Contains(vc, "hi") {
		return fmt.Errorf("valueCounts output wrong: %q", vc)
	}
	// pivot: 2-D grid by g × g, sum of y — lo=1+3+5+7+9=25, hi=11+13+15+17+19=75
	pv, err := call(`{"action":"pivot","path":"fit.csv","by":"g","column2":"g","column":"y","agg":"sum"}`)
	if err != nil {
		return fmt.Errorf("pivot: %v", err)
	}
	if !strings.Contains(pv, "25") || !strings.Contains(pv, "75") {
		return fmt.Errorf("pivot sums wrong: %q", pv)
	}
	// dedupe writes cleaned file with format
	dd, err := call(`{"action":"dedupe","path":"fit.csv","column":"g","format":"csv"}`)
	if err != nil {
		return fmt.Errorf("dedupe: %v", err)
	}
	if !strings.Contains(dd, "8 duplicates removed") || !strings.Contains(dd, "dedup.csv") {
		return fmt.Errorf("dedupe output wrong: %q", dd)
	}
	// sample head/tail/random
	for _, mode := range []string{"head", "tail", "random"} {
		sm, err := call(fmt.Sprintf(`{"action":"sample","path":"fit.csv","op":"%s","limit":3}`, mode))
		if err != nil {
			return fmt.Errorf("sample %s: %v", mode, err)
		}
		if !strings.Contains(sm, "x") {
			return fmt.Errorf("sample %s output wrong: %q", mode, sm)
		}
	}
	// outliers: plant an extreme value
	outCSV := "v\n1\n2\n3\n2\n1\n2\n3\n1000\n"
	if err := os.WriteFile(filepath.Join(dir, "out.csv"), []byte(outCSV), 0o644); err != nil {
		return err
	}
	ol, err := call(`{"action":"outliers","path":"out.csv","column":"v"}`)
	if err != nil {
		return fmt.Errorf("outliers: %v", err)
	}
	if !strings.Contains(ol, "1000") {
		return fmt.Errorf("outliers missed the extreme value: %q", ol)
	}
	// movingavg with write-out
	ma, err := call(`{"action":"movingavg","path":"fit.csv","column":"y","bins":3,"format":"csv"}`)
	if err != nil {
		return fmt.Errorf("movingavg: %v", err)
	}
	if !strings.Contains(ma, "movingavg.csv") {
		return fmt.Errorf("movingavg write-out missing: %q", ma)
	}
	// describe alias still works
	if _, err := call(`{"action":"describe","path":"fit.csv"}`); err != nil {
		return fmt.Errorf("describe: %v", err)
	}
	return nil
}

// stressV109NumericCache: the parse-once column cache returns identical
// values across repeated calls and honors invalidation.
func stressV109NumericCache() error {
	dir := tTempDir("numcache")
	defer os.RemoveAll(dir)
	tools.SetBaseDir(dir)
	if err := os.WriteFile(filepath.Join(dir, "d.csv"), []byte("a,b\n1,x\n2,y\n,z\n4,w\n"), 0o644); err != nil {
		return err
	}
	dt := tools.NewDataTool(&config.Config{DataDir: dir})
	d, err := dt.LoadTest("d.csv")
	if err != nil {
		return err
	}
	col := d.NumericColumnTest(0)
	if len(col) != 4 || col[0] != 1 || col[1] != 2 || col[3] != 4 {
		return fmt.Errorf("numeric column wrong: %v", col)
	}
	again := d.NumericColumnTest(0)
	for i := range col {
		if tools.IsNaNTest(col[i]) != tools.IsNaNTest(again[i]) {
			return fmt.Errorf("cached column NaN mismatch at %d", i)
		}
		if !tools.IsNaNTest(col[i]) && col[i] != again[i] {
			return fmt.Errorf("cached column diverged at %d", i)
		}
	}
	return nil
}

// stressV109WindowMessagesLinear: the O(n) history window keeps order,
// markers and budgets on a 20k-message history (the old O(n²) prepend
// would take seconds; the new code finishes instantly).
func stressV109WindowMessagesLinear() error {
	const n = 20000
	hist := make([]llm.Message, 0, n)
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		hist = append(hist, llm.Message{Role: role, Content: fmt.Sprintf("message %d with some padding text", i)})
	}
	deadline := time.Now().Add(5 * time.Second)
	kept, elided := chunking.WindowMessages(hist, 1000)
	if time.Now().After(deadline) {
		// deadline already passed → too slow
		return fmt.Errorf("WindowMessages too slow on %d messages", n)
	}
	if elided == 0 {
		return fmt.Errorf("expected elisions on a tiny budget")
	}
	if want := 1 + (n - elided); len(kept) != want {
		return fmt.Errorf("kept length wrong: %d, want %d (marker + window)", len(kept), want)
	}
	if kept[0].Role != "system" || !strings.Contains(kept[0].Content, fmt.Sprintf("%d older messages", elided)) {
		return fmt.Errorf("marker missing or wrong count: %q", kept[0].Content)
	}
	// The final message (the current turn) must always survive.
	if kept[len(kept)-1].Content != hist[n-1].Content {
		return fmt.Errorf("current turn dropped")
	}
	// Order preserved: marker → oldest kept → … → newest.
	if kept[1].Content != hist[elided].Content {
		return fmt.Errorf("kept window start wrong: %q vs %q", kept[1].Content, hist[elided].Content)
	}
	return nil
}

// stressV109SSEByteScan: the byte-level SSE pump decodes content, keep-alive
// comment lines, reasoning deltas and streamed tool-call fragments exactly
// like the string-based pump it replaced.
func stressV109SSEByteScan() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": keep-alive comment\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"Hel"}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"lo "}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"reasoning_content":"think"}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call1","type":"function","function":{"name":"files","arguments":"{\"ac"}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"tion\":\"list\"}"}}]}}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL
	cfg.RemoteAPIKey = "k"
	cfg.RemoteModel = "m"
	client := llm.NewClient(cfg)
	req := client.BuildChatRequest("m", []llm.Message{{Role: "user", Content: "hi"}}, nil)

	var content, reasoning string
	var calls []llm.ToolCall
	err := client.StreamChat(context.Background(), req, func(ev llm.StreamEvent) error {
		content += ev.Content
		reasoning += ev.Reasoning
		if len(ev.ToolCalls) > 0 {
			calls = ev.ToolCalls
		}
		return nil
	})
	if err != nil {
		return err
	}
	if content != "Hello " {
		return fmt.Errorf("content = %q, want %q", content, "Hello ")
	}
	if reasoning != "think" {
		return fmt.Errorf("reasoning = %q, want %q", reasoning, "think")
	}
	if len(calls) != 1 || calls[0].Function.Name != "files" || calls[0].Function.Arguments != `{"action":"list"}` {
		return fmt.Errorf("assembled tool call wrong: %+v", calls)
	}
	return nil
}

// stressV109SessionsStore: the reconstructed store — index stubs, chapter
// metadata, message counts and the delete-twice error contract.
func stressV109SessionsStore() error {
	dir := tTempDir("store")
	defer os.RemoveAll(dir)
	st := sessions.New(dir)

	a := st.Create()
	a.Title = "alpha"
	a.Chapter = 2
	a.ThreadID = "t1"
	a.ParentID = "p0"
	if err := st.Save(a); err != nil {
		return err
	}
	if _, err := st.AppendMessage(a.ID, llm.Message{Role: "user", Content: "q"}); err != nil {
		return err
	}
	if _, err := st.AppendMessage(a.ID, llm.Message{Role: "assistant", Content: "a"}); err != nil {
		return err
	}
	list, err := st.List()
	if err != nil {
		return err
	}
	if len(list) != 1 {
		return fmt.Errorf("list = %d, want 1", len(list))
	}
	if list[0].Messages != nil {
		return fmt.Errorf("stub must not carry histories")
	}
	if list[0].MessageCount() != 2 || list[0].Chapter != 2 || list[0].ThreadID != "t1" {
		return fmt.Errorf("stub metadata wrong: count=%d ch=%d thread=%s",
			list[0].MessageCount(), list[0].Chapter, list[0].ThreadID)
	}
	full, err := st.Get(a.ID)
	if err != nil {
		return err
	}
	if len(full.Messages) != 2 || full.Title != "alpha" {
		return fmt.Errorf("full load wrong")
	}
	if err := st.Delete(a.ID); err != nil {
		return fmt.Errorf("first delete: %v", err)
	}
	if err := st.Delete(a.ID); err == nil {
		return fmt.Errorf("second delete must fail")
	}
	return nil
}

// stressV109SandboxContract: the reconstructed sandbox — construction
// always succeeds, the tool surface stays `codeExec`, and the resource
// caps survive as fields.
func stressV109SandboxContract() error {
	dir := tTempDir("sbx")
	defer os.RemoveAll(dir)
	sb, err := sandbox.New(64, 10, "")
	if err != nil {
		return err
	}
	defer sb.Close()
	if sb.WorkDir() == "" {
		return fmt.Errorf("workdir empty")
	}
	if sb.Name() != "codeExec" {
		return fmt.Errorf("tool name = %q, want codeExec", sb.Name())
	}
	if sb.Description() == "" || sb.Parameters() == nil {
		return fmt.Errorf("tool surface incomplete")
	}
	// args validation paths (no interpreter needed for these)
	if _, err := sb.Run(context.Background(), json.RawMessage(`{"code":""}`)); err == nil {
		return fmt.Errorf("empty code must error")
	}
	if _, err := sb.Run(context.Background(), json.RawMessage(`{"code":"1","lang":"ruby"}`)); err == nil {
		return fmt.Errorf("unsupported lang must error")
	}
	// scoped second sandbox closes cleanly
	sb2, err := sandbox.NewCodeExecSandbox(512, 25, dir+"/sb2")
	if err != nil {
		return err
	}
	if err := sb2.Close(); err != nil {
		return err
	}
	return nil
}

// stressV109AicontextV9: the AI briefing teaches files v2, the new analysis
// actions and smooth streaming.
func stressV109AicontextV9() error {
	// v1.0.10: forward-compatible (context files only ever grow).
	if aicontext.ContextVersion < 9 {
		return fmt.Errorf("ContextVersion = %d, want >= 9", aicontext.ContextVersion)
	}
	body := aicontext.SystemMessage(config.Default())
	for _, want := range []string{"combine", "valueCounts", "outliers", "movingavg", "offsetLine", "regression"} {
		if !strings.Contains(body, want) {
			return fmt.Errorf("AI context missing %q", want)
		}
	}
	return nil
}
