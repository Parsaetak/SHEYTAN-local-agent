// v1.0.2 stress tests: file attachments + chunking engine, thinking mode
// (reasoning_content + <think> tags), tool selection, persistent recall, and
// the session meta-index.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/sheytan/local-agent/internal/agent"
	"github.com/sheytan/local-agent/internal/chunking"
	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/llm"
	"github.com/sheytan/local-agent/internal/memory"
	"github.com/sheytan/local-agent/internal/recall"
	"github.com/sheytan/local-agent/internal/sessions"
	"github.com/sheytan/local-agent/internal/tools"
)

// --- chunking engine ---

func stressChunkingTokenEstimate() error {
	if got := chunking.EstimateTokens(""); got != 0 {
		return fmt.Errorf("empty string tokens = %d, want 0", got)
	}
	// ~4 chars per token, rounded up.
	if got := chunking.EstimateTokens("hello world, this is a test"); got < 5 || got > 10 {
		return fmt.Errorf("token estimate out of range: %d", got)
	}
	msgs := []llm.Message{{Role: "user", Content: "12345678901234567890123456789012"}}
	if got := chunking.EstimateMessagesTokens(msgs); got != 8 {
		return fmt.Errorf("32 chars = %d tokens, want 8", got)
	}
	return nil
}

func stressChunkingSplitParagraphs() error {
	text := strings.Repeat("para one\n\npara two\n\n", 100)
	chunks := chunking.SplitParagraphs(text, 200)
	if len(chunks) < 10 {
		return fmt.Errorf("expected >= 10 chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if len(c) > 200+len("para two\n\n") { // boundary may exceed max by one paragraph? no — cut is <= max
			return fmt.Errorf("chunk exceeds max: %d bytes", len(c))
		}
	}
	joined := strings.Join(chunks, "")
	if joined != text {
		return fmt.Errorf("re-joining chunks is lossy")
	}
	if chunking.SplitParagraphs("", 100) != nil {
		return fmt.Errorf("empty input should give nil chunks")
	}
	return nil
}

func stressChunkingHeadTailWindow() error {
	small := "line1\nline2\nline3\n"
	if got := chunking.WindowHeadTail(small, 256); got != small {
		return fmt.Errorf("small text must pass through unchanged")
	}
	// Big text: head + marker + tail, total bounded.
	var b strings.Builder
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&b, "line %d — the quick brown fox jumps over the lazy dog\n", i)
	}
	big := b.String()
	out := chunking.WindowHeadTail(big, 4000)
	if len(out) > 6000 {
		return fmt.Errorf("windowed output too large: %d", len(out))
	}
	if !strings.Contains(out, "[elided") {
		return fmt.Errorf("elision marker missing")
	}
	if !strings.HasPrefix(out, "line 0 ") {
		return fmt.Errorf("head must start at the beginning")
	}
	if !strings.Contains(out, "line 4999 ") {
		return fmt.Errorf("tail must contain the end")
	}
	return nil
}

func stressChunkingTextDetection() error {
	dir := tTempDir("chunk-detect")
	defer os.RemoveAll(dir)

	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("hello text"), 0o644); err != nil {
		return err
	}
	if !chunking.IsTextFile(txt) {
		return fmt.Errorf(".txt with text content must be text")
	}
	if !chunking.IsKnownTextExt(txt) {
		return fmt.Errorf(".txt must be a known text extension")
	}
	md := filepath.Join(dir, "README.md")
	if err := os.WriteFile(md, []byte("# hello"), 0o644); err != nil {
		return err
	}
	if !chunking.IsTextFile(md) || !chunking.IsKnownTextExt(md) {
		return fmt.Errorf(".md must be text (100%% supported)")
	}
	// Binary: NUL bytes early.
	bin := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(bin, []byte{0x4D, 0x5A, 0x00, 0x01, 0x02}, 0o644); err != nil {
		return err
	}
	if chunking.IsTextFile(bin) {
		return fmt.Errorf("NUL-containing binary must not be text")
	}
	// Extension-less text (Makefile).
	mk := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(mk, []byte("all:\n\tgo build\n"), 0o644); err != nil {
		return err
	}
	if !chunking.IsTextFile(mk) {
		return fmt.Errorf("Makefile must sniff as text")
	}
	return nil
}

func stressChunkingAttachmentFormat() error {
	dir := tTempDir("chunk-attach")
	defer os.RemoveAll(dir)

	// Small text file → full fenced block.
	small := filepath.Join(dir, "small.md")
	if err := os.WriteFile(small, []byte("# Title\n\nBody text."), 0o644); err != nil {
		return err
	}
	out := chunking.FormatFileAttachment(small, 4096)
	if !strings.Contains(out, "attached file: small.md") || !strings.Contains(out, "```md") || !strings.Contains(out, "Body text.") {
		return fmt.Errorf("small text attachment not fully inlined: %q", out)
	}

	// Huge text file → head + elision marker + tail.
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&b, "record %d alpha beta gamma delta epsilon\n", i)
	}
	big := filepath.Join(dir, "big.log")
	if err := os.WriteFile(big, []byte(b.String()), 0o644); err != nil {
		return err
	}
	out = chunking.FormatFileAttachment(big, 4000)
	if !strings.Contains(out, "[elided") || !strings.Contains(out, "record 0 ") || !strings.Contains(out, "record 19999 ") {
		return fmt.Errorf("huge attachment missing head/tail/elision")
	}

	// Binary file → metadata note with path hint.
	bin := filepath.Join(dir, "image.bin")
	if err := os.WriteFile(bin, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		return err
	}
	out = chunking.FormatFileAttachment(bin, 4096)
	if !strings.Contains(out, "binary") || !strings.Contains(out, "files tool") {
		return fmt.Errorf("binary attachment must become a metadata note: %q", out)
	}

	// Missing file → readable error note.
	out = chunking.FormatFileAttachment(filepath.Join(dir, "nope.txt"), 4096)
	if !strings.Contains(out, "could not be read") && !strings.Contains(out, "read failed") {
		return fmt.Errorf("missing file must produce an error note: %q", out)
	}
	return nil
}

func stressChunkingComposeUserMessage() error {
	dir := tTempDir("chunk-compose")
	defer os.RemoveAll(dir)
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.md")
	if err := os.WriteFile(f1, []byte("alpha content"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(f2, []byte("# beta"), 0o644); err != nil {
		return err
	}
	out := chunking.ComposeUserMessage("please review", []string{f1, f2}, 4096)
	if !strings.HasPrefix(out, "please review") {
		return fmt.Errorf("user text must lead the message")
	}
	if !strings.Contains(out, "### Attached files") {
		return fmt.Errorf("attachment section missing")
	}
	if !strings.Contains(out, "alpha content") || !strings.Contains(out, "# beta") {
		return fmt.Errorf("attachment contents missing")
	}
	// No attachments → text unchanged.
	if got := chunking.ComposeUserMessage("solo", nil, 4096); got != "solo" {
		return fmt.Errorf("no-attachment message must be unchanged: %q", got)
	}
	return nil
}

func stressWindowMessagesBudget() error {
	// Build a long history: 40 alternating messages of ~200 tokens each.
	var hist []llm.Message
	for i := 0; i < 40; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		hist = append(hist, llm.Message{Role: role, Content: strings.Repeat(fmt.Sprintf("msg%02d ", i), 100)})
	}
	// Budget for ~5 messages (each ~25 tokens... scale: each message = 500 tokens here).
	kept, elided := chunking.WindowMessages(hist, 1200)
	if elided == 0 {
		return fmt.Errorf("expected elision with a tight budget")
	}
	if len(kept) > 10 {
		return fmt.Errorf("kept too many messages: %d", len(kept))
	}
	// The last user message must ALWAYS be present verbatim.
	lastUser := ""
	for i := len(hist) - 1; i >= 0; i-- {
		if hist[i].Role == "user" {
			lastUser = hist[i].Content
			break
		}
	}
	found := false
	for _, m := range kept {
		if m.Content == lastUser {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("the latest user message was elided — never allowed")
	}
	// A marker notes the compaction.
	if elided > 0 && !strings.Contains(kept[0].Content, "[context window]") {
		return fmt.Errorf("elision marker missing at position 0")
	}
	// Generous budget keeps everything.
	kept2, elided2 := chunking.WindowMessages(hist, 100000)
	if elided2 != 0 || len(kept2) != len(hist) {
		return fmt.Errorf("generous budget must keep all: %d kept, %d elided", len(kept2), elided2)
	}
	return nil
}

// --- thinking mode ---

func stressReasoningDeltaParse() error {
	var reasoning, content string
	var toolCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"reasoning_content":"let me think about this"}}]}`))
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"reasoning_content":" carefully."}}]}`))
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"The answer is 42."}}]}`))
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "thinky"
	client := llm.NewClient(cfg)
	err := client.StreamChat(context.Background(), &llm.ChatRequest{Model: "thinky"}, func(ev llm.StreamEvent) error {
		reasoning += ev.Reasoning
		content += ev.Content
		if len(ev.ToolCalls) > 0 {
			toolCalls++
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("stream: %v", err)
	}
	if reasoning != "let me think about this carefully." {
		return fmt.Errorf("reasoning not assembled: %q", reasoning)
	}
	if content != "The answer is 42." {
		return fmt.Errorf("content wrong: %q", content)
	}
	if toolCalls != 0 {
		return fmt.Errorf("no tool calls expected")
	}
	return nil
}

func stressThinkTagExtraction() error {
	// Full block.
	r, c := agent.SplitThink("<think>step one, step two</think>The final answer.")
	if r != "step one, step two" || c != "The final answer." {
		return fmt.Errorf("full block: r=%q c=%q", r, c)
	}
	// Unclosed block (stream cut mid-think).
	r, c = agent.SplitThink("prefix <think>still thinking")
	if r != "still thinking" || c != "prefix" {
		return fmt.Errorf("unclosed: r=%q c=%q", r, c)
	}
	// Multiple blocks + surrounding text.
	r, c = agent.SplitThink("a<think>one</think>b<think>two</think>c")
	if !strings.Contains(r, "one") || !strings.Contains(r, "two") || c != "abc" {
		return fmt.Errorf("multi: r=%q c=%q", r, c)
	}
	// No tags → untouched.
	r, c = agent.SplitThink("plain answer")
	if r != "" || c != "plain answer" {
		return fmt.Errorf("plain: r=%q c=%q", r, c)
	}
	// SplitThink must never leak the tags themselves.
	r, c = agent.SplitThink("<think>x</think>y")
	if strings.Contains(r, "think") || strings.Contains(c, "think") {
		return fmt.Errorf("tags leaked into output: r=%q c=%q", r, c)
	}
	return nil
}

func stressOrchestratorThinkingMode() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the thinking nudge is present in the request.
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(string(body), "THINKING MODE (enabled by the user)") {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"<think>user wants 2+2; that is 4.</think>"}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"2 + 2 = 4."}}]}`))
		} else {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"(no nudge) 2+2=4"}}]}`))
		}
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "thinker"
	cfg.ThinkingMode = true
	orch := agent.New(cfg, llm.NewClient(cfg))

	var sawReasoningActivity bool
	res, err := orch.RunDetailed(context.Background(),
		[]llm.Message{{Role: "user", Content: "what is 2+2?"}},
		func(a agent.Activity) {
			if a.Type == "reasoning" {
				sawReasoningActivity = true
			}
		})
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}
	if res.Text != "2 + 2 = 4." {
		return fmt.Errorf("final text wrong: %q", res.Text)
	}
	if !strings.Contains(res.Reasoning, "user wants 2+2") {
		return fmt.Errorf("reasoning not extracted: %q", res.Reasoning)
	}
	if !sawReasoningActivity {
		return fmt.Errorf("no reasoning activity emitted during streaming")
	}
	return nil
}

// --- tool selection ---

func stressOrchestratorToolFiltering() error {
	var firstBody atomic.Value // FIRST request body (tools array check)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if calls.Add(1) == 1 {
			firstBody.Store(string(body))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// 1st call: demand the DISABLED tool. 2nd call: answer.
		if calls.Load() == 1 {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"shell","arguments":"{\"command\":\"dir\"}"}}]}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
		} else {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"done without shell"}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "restricted"
	cfg.EnabledTools = []string{"files"} // shell disabled
	orch := agent.New(cfg, llm.NewClient(cfg))
	orch.Register(tools.Shell{})
	orch.Register(tools.Files{})

	var toolResults []string
	res, err := orch.RunDetailed(context.Background(),
		[]llm.Message{{Role: "user", Content: "run dir"}},
		func(a agent.Activity) {
			if a.Type == "tool_end" {
				toolResults = append(toolResults, fmt.Sprint(a.Detail))
			}
		})
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}
	// The model called the disabled shell tool → must get the disabled error.
	disabledMsg := false
	for _, tr := range toolResults {
		if strings.Contains(tr, "disabled by the user") && strings.Contains(tr, "Enabled tools: files") {
			disabledMsg = true
		}
	}
	if !disabledMsg {
		return fmt.Errorf("disabled tool call did not return the guidance message: %v", toolResults)
	}
	// The tools spec advertised to the LLM must exclude shell — inspect the
	// FIRST request's tools array specifically (the conversation history of
	// later requests legitimately contains the model's own shell tool_call).
	fb, _ := firstBody.Load().(string)
	var probe struct {
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(fb), &probe); err != nil {
		return fmt.Errorf("probe request body: %v", err)
	}
	for _, t := range probe.Tools {
		if t.Function.Name == "shell" {
			return fmt.Errorf("disabled tool must not be advertised in tool specs")
		}
		if t.Function.Name == "files" {
			goto found
		}
	}
	return fmt.Errorf("enabled tool must be advertised")
found:
	if res.Text != "done without shell" {
		return fmt.Errorf("final text wrong: %q", res.Text)
	}

	// Config helpers.
	if !cfg.ToolEnabled("files") || cfg.ToolEnabled("shell") {
		return fmt.Errorf("ToolEnabled filters wrong")
	}
	if got := cfg.EnabledToolList([]string{"shell", "files", "git"}); len(got) != 1 || got[0] != "files" {
		return fmt.Errorf("EnabledToolList wrong: %v", got)
	}
	return nil
}

// --- persistent recall ---

func stressRecallIndexAndSearch() error {
	dir := tTempDir("recall-search")
	defer os.RemoveAll(dir)
	engine := recall.New(dir)

	_ = engine.IndexTurn("s1", "Golang project", "how do I fix the goroutine leak in the worker pool", "found the leak: workers blocked on an unbuffered channel; use buffered chan size 8", []string{"codeExec", "files"})
	_ = engine.IndexTurn("s2", "Trip planning", "best time to visit Kyoto for cherry blossoms", "late March to early April; book hotels 3 months ahead", nil)
	_ = engine.IndexTurn("s3", "Data work", "analyze sales.csv and chart revenue by region", "revenue grew 23% YoY, EMEA leads; chart saved to charts/rev.svg", []string{"dataAnalysis"})

	if got := engine.Count(); got != 3 {
		return fmt.Errorf("count = %d, want 3", got)
	}

	// BM25 must rank the goroutine capsule first for a goroutine question.
	hits := engine.Search("goroutine leak worker pool", 3)
	if len(hits) == 0 {
		return fmt.Errorf("no hits for a clearly relevant query")
	}
	if hits[0].SessionID != "s1" {
		return fmt.Errorf("top hit = %s, want s1", hits[0].SessionID)
	}

	// The data question must find the analysis capsule.
	hits = engine.Search("sales revenue chart", 3)
	if len(hits) == 0 || hits[0].SessionID != "s3" {
		return fmt.Errorf("sales query did not find s3 first: %v", hitIDs(hits))
	}

	// RelevantBlock is bounded and formatted.
	block := engine.RelevantBlock("goroutine leak", 3, 600)
	if !strings.Contains(block, "RELEVANT PAST CONTEXT") || !strings.Contains(block, "goroutine") {
		return fmt.Errorf("block format wrong: %q", block)
	}
	if chunking.EstimateTokens(block) > 700 {
		return fmt.Errorf("recall block exceeded its budget")
	}

	// Irrelevant query → no injection (keeps the prompt clean).
	if block := engine.RelevantBlock("zzz qq xxx meaningless", 1, 100); block != "" {
		return fmt.Errorf("irrelevant query should not inject a block, got %q", block)
	}
	return nil
}

func stressRecallDedupAndClips() error {
	dir := tTempDir("recall-dedup")
	defer os.RemoveAll(dir)
	engine := recall.New(dir)

	long := strings.Repeat("what is the best way to structure a large go codebase ", 20)
	_ = engine.IndexTurn("s1", "t", long, strings.Repeat("answer ", 200), nil)
	_ = engine.IndexTurn("s1", "t", long, strings.Repeat("answer ", 200), nil) // duplicate
	if got := engine.Count(); got != 1 {
		return fmt.Errorf("duplicate index: count = %d, want 1", got)
	}

	// Capsule fields are clipped to keep the index tiny.
	st := engine.Stats()
	if st.Capsules != 1 {
		return fmt.Errorf("stats capsules = %d", st.Capsules)
	}
	hits := engine.Search("structure go codebase", 1)
	if len(hits) != 1 {
		return fmt.Errorf("clipped capsule not searchable")
	}
	if len(hits[0].Query) > 200 {
		return fmt.Errorf("query not clipped: %d bytes", len(hits[0].Query))
	}
	if len(hits[0].Answer) > 400 {
		return fmt.Errorf("answer not clipped: %d bytes", len(hits[0].Answer))
	}
	return nil
}

func stressRecallBackfill() error {
	dir := tTempDir("recall-backfill")
	defer os.RemoveAll(dir)
	store := sessions.New(filepath.Join(dir, "sessions"))
	engine := recall.New(dir)

	// Two sessions with exchanges.
	s1 := store.Create()
	s1.Title = "Go questions"
	s1.Messages = []llm.Message{
		{Role: "user", Content: "how do interfaces work in go"},
		{Role: "assistant", Content: "interfaces are implicit in Go"},
	}
	_ = store.Save(s1)
	s2 := store.Create()
	s2.Title = "Recipes"
	s2.Messages = []llm.Message{
		{Role: "user", Content: "best pasta carbonara recipe"},
		{Role: "assistant", Content: "eggs, pecorino, guanciale, pepper"},
	}
	_ = store.Save(s2)

	if err := engine.Backfill(store); err != nil {
		return fmt.Errorf("backfill: %v", err)
	}
	if got := engine.Count(); got != 2 {
		return fmt.Errorf("backfill indexed %d exchanges, want 2", got)
	}
	hits := engine.Search("carbonara recipe", 2)
	if len(hits) == 0 || hits[0].SessionID != s2.ID {
		return fmt.Errorf("backfilled capsule not searchable")
	}

	// Second backfill is a no-op (marker file).
	if err := engine.Backfill(store); err != nil {
		return fmt.Errorf("second backfill: %v", err)
	}
	if got := engine.Count(); got != 2 {
		return fmt.Errorf("second backfill duplicated capsules: %d", got)
	}

	// Clear wipes everything including the marker.
	if err := engine.Clear(); err != nil {
		return err
	}
	if got := engine.Count(); got != 0 {
		return fmt.Errorf("clear left %d capsules", got)
	}
	return nil
}

// fakeRecaller is a deterministic Recaller for orchestrator tests.
type fakeRecaller struct {
	block string
}

func (f fakeRecaller) RelevantBlock(query string, k, maxTokens int) string {
	return f.block
}

func stressOrchestratorRecallInjection() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(string(body), "RELEVANT PAST CONTEXT") && strings.Contains(string(body), "goroutine leak") {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"I remember: we fixed it with a buffered channel."}}]}`))
		} else {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"no recall context seen"}}]}`))
		}
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "remembery"
	cfg.RecallEnabled = true
	orch := agent.New(cfg, llm.NewClient(cfg))
	orch.SetRecaller(fakeRecaller{block: "## RELEVANT PAST CONTEXT (auto-recalled)\n\n[1] 2026-01-01 · Go debugging\n    user asked: how do I fix the goroutine leak\n    outcome: use a buffered channel\n"})

	res, err := orch.RunDetailed(context.Background(),
		[]llm.Message{
			{Role: "user", Content: "what about my old goroutine problem?"},
		}, func(agent.Activity) {})
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}
	if res.Text != "I remember: we fixed it with a buffered channel." {
		return fmt.Errorf("recall block not injected before the user message: %q", res.Text)
	}
	if res.Recalled != 1 {
		return fmt.Errorf("Recalled = %d, want 1", res.Recalled)
	}

	// RecallEnabled=false → no injection even with a recaller wired.
	cfg2 := config.Default()
	cfg2.Provider = config.ProviderRemote
	cfg2.RemoteBaseURL = srv.URL + "/v1"
	cfg2.RemoteModel = "remembery"
	cfg2.RecallEnabled = false
	orch2 := agent.New(cfg2, llm.NewClient(cfg2))
	orch2.SetRecaller(fakeRecaller{block: "## RELEVANT PAST CONTEXT\nstuff"})
	res2, err := orch2.RunDetailed(context.Background(),
		[]llm.Message{{Role: "user", Content: "anything"}}, func(agent.Activity) {})
	if err != nil {
		return fmt.Errorf("run2: %v", err)
	}
	if res2.Text != "no recall context seen" {
		return fmt.Errorf("recall must be skipped when disabled: %q", res2.Text)
	}
	return nil
}

// --- session meta index ---

func stressSessionsMetaIndex() error {
	dir := tTempDir("sess-meta")
	defer os.RemoveAll(dir)
	store := sessions.New(dir)

	a := store.Create()
	_, _ = store.AppendMessage(a.ID, llm.Message{Role: "user", Content: "hello"})
	_, _ = store.AppendMessage(a.ID, llm.Message{Role: "assistant", Content: "hi there"})
	b := store.Create()
	_ = store.UpdateTitle(b.ID, "second session")

	// List() returns stubs with correct metadata.
	list, err := store.List()
	if err != nil {
		return fmt.Errorf("list: %v", err)
	}
	if len(list) != 2 {
		return fmt.Errorf("list = %d sessions, want 2", len(list))
	}
	if list[0].ID != b.ID {
		return fmt.Errorf("order wrong: %s first", list[0].ID)
	}
	for _, s := range list {
		if s.Messages != nil {
			return fmt.Errorf("List() stubs must not carry full histories")
		}
	}
	// MessageCount works on stubs (b=0, a=2 via index).
	var aCount int
	for _, s := range list {
		if s.ID == a.ID {
			aCount = s.MessageCount()
		}
	}
	if aCount != 2 {
		return fmt.Errorf("stub message count for a = %d, want 2", aCount)
	}

	// A new Store instance reads the persisted index.json.
	store2 := sessions.New(dir)
	list2, _ := store2.List()
	if len(list2) != 2 {
		return fmt.Errorf("reloaded index lost sessions: %d", len(list2))
	}
	if list2[0].Title != "second session" {
		return fmt.Errorf("reloaded index lost the title: %q", list2[0].Title)
	}

	// ListFull returns complete histories.
	full, _ := store2.ListFull()
	if len(full) != 2 || len(full[1].Messages) != 2 {
		return fmt.Errorf("ListFull wrong: %d sessions", len(full))
	}

	// Delete removes from the index too.
	_ = store.Delete(b.ID)
	list3, _ := store.List()
	if len(list3) != 1 || list3[0].ID != a.ID {
		return fmt.Errorf("delete did not update the index: %d left", len(list3))
	}
	return nil
}

// --- config v1.0.2 roundtrip + defaults ---

func stressConfigV102Fields() error {
	cfg := config.Default()
	if !cfg.RecallEnabled || cfg.RecallTopK != 4 {
		return fmt.Errorf("recall defaults wrong: %v %d", cfg.RecallEnabled, cfg.RecallTopK)
	}
	if cfg.ThinkingMode {
		return fmt.Errorf("thinking mode must default off")
	}
	if cfg.AttachmentsBudgetKB != 256 || cfg.AttachmentsBudgetBytes() != 256*1024 {
		return fmt.Errorf("attachment budget default wrong")
	}
	if cfg.HistoryWindowPct != 60 || cfg.HistoryWindowTokens() != cfg.LLM.NumCtx*60/100 {
		return fmt.Errorf("history window default wrong")
	}
	if len(cfg.EnabledTools) != 0 {
		return fmt.Errorf("enabled tools must default empty (= all)")
	}

	// Roundtrip through JSON.
	dir := tTempDir("cfg102")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "config.json")
	cfg.ThinkingMode = true
	cfg.EnabledTools = []string{"files", "memory"}
	cfg.RecallTopK = 7
	cfg.AttachmentsBudgetKB = 512
	cfg.HistoryWindowPct = 75
	if err := config.Save(path, cfg); err != nil {
		return fmt.Errorf("save: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load: %v", err)
	}
	if !loaded.ThinkingMode || loaded.RecallTopK != 7 || loaded.AttachmentsBudgetKB != 512 || loaded.HistoryWindowPct != 75 {
		return fmt.Errorf("v1.0.2 fields did not roundtrip")
	}
	if len(loaded.EnabledTools) != 2 || loaded.EnabledTools[0] != "files" {
		return fmt.Errorf("enabled tools did not roundtrip: %v", loaded.EnabledTools)
	}
	// Clamps.
	loaded.RecallTopK = 99
	if loaded.EffectiveRecallTopK() != 12 {
		return fmt.Errorf("recallTopK clamp wrong: %d", loaded.EffectiveRecallTopK())
	}
	loaded.HistoryWindowPct = 500
	if loaded.HistoryWindowTokens() != loaded.LLM.NumCtx*95/100 {
		return fmt.Errorf("history pct clamp wrong")
	}
	return nil
}

// --- memory tool history action ---

func stressMemoryHistoryAction() error {
	dir := tTempDir("mem-history")
	defer os.RemoveAll(dir)
	engine := recall.New(dir)
	_ = engine.IndexTurn("s1", "Git work", "how to undo the last commit", "use git reset --soft HEAD~1", nil)

	tool := memory.Tool{
		Store: memory.New(filepath.Join(dir, "memory.jsonl")),
		RecallSearch: func(query string, k int) []string {
			var lines []string
			for _, c := range engine.Search(query, k) {
				lines = append(lines, c.TS.Format("2006-01-02")+" ["+c.SessionID+"] "+c.Title+": "+c.Query)
			}
			return lines
		},
	}
	out, err := tool.Run(context.Background(), json.RawMessage(`{"action":"history","query":"undo last commit"}`))
	if err != nil {
		return fmt.Errorf("history action: %v", err)
	}
	if !strings.Contains(out, "git reset --soft HEAD~1") && !strings.Contains(out, "undo the last commit") {
		return fmt.Errorf("history search did not surface the capsule: %q", out)
	}

	// No recall wired → graceful message.
	bare := memory.Tool{Store: memory.New(filepath.Join(dir, "m2.jsonl"))}
	out, err = bare.Run(context.Background(), json.RawMessage(`{"action":"history","query":"x"}`))
	if err != nil || !strings.Contains(out, "not available") {
		return fmt.Errorf("bare tool should degrade gracefully: %q %v", out, err)
	}

	// Missing query → error.
	if _, err := tool.Run(context.Background(), json.RawMessage(`{"action":"history"}`)); err == nil {
		return fmt.Errorf("history without query must error")
	}
	return nil
}

func hitIDs(hits []recall.Capsule) []string {
	var out []string
	for _, h := range hits {
		out = append(out, h.SessionID)
	}
	return out
}
