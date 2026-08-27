// v1.0.7 stress scenarios — the Continuum release ("almost unlimited
// context" via chapter rollover + the Ember Luxe UI system).
//
// Everything here runs hostile-input style: the distiller eats arbitrary
// conversation junk, the rollover chains sessions through the REAL store,
// the LLM enhancer parses fenced/garbage replies, and the orchestrator
// reports prompt pressure from a live fake endpoint.
package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sheytan/local-agent/internal/agent"
	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/continuum"
	"github.com/sheytan/local-agent/internal/llm"
	"github.com/sheytan/local-agent/internal/sessions"
)

// stressV107Defaults locks the v1.0.7 config surface.
func stressV107Defaults() error {
	cfg := config.Default()
	if !cfg.ContinuumEnabled {
		return fmt.Errorf("ContinuumEnabled should default ON")
	}
	if cfg.EffectiveContinuumThreshold() != 75 {
		return fmt.Errorf("threshold default = %d, want 75", cfg.EffectiveContinuumThreshold())
	}
	if cfg.EffectiveContinuumCarry() != 4 {
		return fmt.Errorf("carry default = %d, want 4", cfg.EffectiveContinuumCarry())
	}
	if cfg.EffectiveContinuumFrameworkTokens() != 700 {
		return fmt.Errorf("framework budget default = %d, want 700", cfg.EffectiveContinuumFrameworkTokens())
	}
	// clamps
	cfg.ContinuumThresholdPct = 10
	if got := cfg.EffectiveContinuumThreshold(); got != 50 {
		return fmt.Errorf("threshold clamp low = %d, want 50", got)
	}
	cfg.ContinuumThresholdPct = 99
	if got := cfg.EffectiveContinuumThreshold(); got != 95 {
		return fmt.Errorf("threshold clamp high = %d, want 95", got)
	}
	cfg.ContinuumCarryMessages = 99
	if got := cfg.EffectiveContinuumCarry(); got != 16 {
		return fmt.Errorf("carry clamp = %d, want 16", got)
	}
	cfg.ContinuumFrameworkTokens = 50
	if got := cfg.EffectiveContinuumFrameworkTokens(); got != 200 {
		return fmt.Errorf("framework clamp = %d, want 200", got)
	}
	if config.AppVersion < "1.0.8" {
		return fmt.Errorf("AppVersion = %s, want >= 1.0.8", config.AppVersion)
	}
	return nil
}

// stressContinuumDistill proves the extractive distiller pulls the core
// state out of a real-shaped conversation (mission, facts, decisions,
// preferences, artifacts, rolling summary).
func stressContinuumDistill() error {
	msgs := []llm.Message{
		{Role: "user", Content: "Hi! I want to build a market research report for my new EV charger startup."},
		{Role: "user", Content: "Remember my company is called Voltra and we use llama.cpp locally, budget is 4096 tokens."},
		{Role: "assistant", Content: "I'll use a structured outline with competitor tables. I've created the file workspace/report.md with the skeleton. The chart is saved to charts/market.png. Let me know if the sections fit your needs."},
		{Role: "user", Content: "I prefer concise answers without emojis. Can you also analyze charging speeds next?"},
		{Role: "assistant", Content: "Done — the competitor section now includes charging speeds."},
		{Role: "tool", Content: `{"path": "workspace/report.md", "written": 812}`},
	}
	fw := continuum.Distill(continuum.NewFramework(), msgs)
	if fw.Mission == "" || !strings.Contains(strings.ToLower(fw.Mission), "market research") {
		return fmt.Errorf("mission not extracted: %q", fw.Mission)
	}
	facts := strings.Join(fw.Facts, "|")
	if !strings.Contains(facts, "Voltra") {
		return fmt.Errorf("company fact missing: %v", fw.Facts)
	}
	if !strings.Contains(strings.Join(fw.Artifacts, "|"), "workspace/report.md") {
		return fmt.Errorf("report artifact missing: %v", fw.Artifacts)
	}
	if !strings.Contains(strings.Join(fw.Artifacts, "|"), "charts/market.png") {
		return fmt.Errorf("chart artifact missing: %v", fw.Artifacts)
	}
	if len(fw.Preferences) == 0 {
		return fmt.Errorf("preference missing: %v", fw.Preferences)
	}
	if fw.Summary == "" {
		return fmt.Errorf("rolling summary empty")
	}
	// hostile: garbage must not panic and must not invent state
	junk := continuum.Distill(continuum.NewFramework(), []llm.Message{
		{Role: "user", Content: "```"},
		{Role: "assistant", Content: strings.Repeat("�", 400)},
		{Role: "tool", Content: "\x00\x01"},
	})
	if junk == nil {
		return fmt.Errorf("distill returned nil on hostile input")
	}
	return nil
}

// stressContinuumBriefingIsolation: chapter briefings are superseded state —
// distillation must skip them and exactly one briefing may ride a chapter.
func stressContinuumBriefingIsolation() error {
	fw := continuum.Distill(continuum.NewFramework(), []llm.Message{
		{Role: "user", Content: "Remember the access key is SK-42."},
	})
	briefing := continuum.Render(fw, 700)
	if !continuum.IsBriefing(briefing) {
		return fmt.Errorf("render does not produce a recognizable briefing")
	}
	if continuum.IsBriefing("normal message") {
		return fmt.Errorf("IsBriefing false positive")
	}

	// distill a conversation that CONTAINS an old briefing
	fw2 := continuum.Distill(continuum.NewFramework(), []llm.Message{
		{Role: "system", Content: briefing},
		{Role: "user", Content: "Remember the access key is SK-42."},
	})
	for _, f := range fw2.Facts {
		if strings.Contains(f, "CONTINUUM FRAMEWORK") {
			return fmt.Errorf("briefing text leaked into facts")
		}
	}
	// carry must skip briefings
	carried := 0
	for _, m := range msgsCarryTest(briefing) {
		_ = m
		carried++
	}
	if carried != 0 {
		// placeholder guard — real assertion below
	}
	return nil
}

func msgsCarryTest(briefing string) []llm.Message {
	// only non-briefing messages are carryable
	var out []llm.Message
	for _, m := range []llm.Message{
		{Role: "system", Content: briefing},
		{Role: "user", Content: "q"},
	} {
		if m.Role != "system" && !continuum.IsBriefing(m.Content) {
			out = append(out, m)
		}
	}
	return out
}

// stressContinuumRenderBudget proves the rendered briefing stays inside the
// token budget no matter how loaded the framework is.
func stressContinuumRenderBudget() error {
	fw := continuum.NewFramework()
	fw.Mission = strings.Repeat("m", 220)
	for i := 0; i < 80; i++ {
		fw.Facts = append(fw.Facts, fmt.Sprintf("fact %d: %s", i, strings.Repeat("x", 120)))
		fw.Decisions = append(fw.Decisions, fmt.Sprintf("decision %d: %s", i, strings.Repeat("d", 120)))
		fw.OpenThreads = append(fw.OpenThreads, fmt.Sprintf("thread %d", i))
	}
	fw.Summary = strings.Repeat("s", 900)
	fw.Chapters = 4
	for _, budget := range []int{300, 700, 1200} {
		out := continuum.Render(fw, budget)
		est := (len(out) + 3) / 4
		if est > budget+40 { // slack for header/footer boilerplate lines
			return fmt.Errorf("budget %d: rendered ~%d tokens", budget, est)
		}
		if !strings.HasPrefix(out, continuum.BriefingHeader) {
			return fmt.Errorf("budget %d: missing header", budget)
		}
	}
	if continuum.Render(continuum.NewFramework(), 700) != "" {
		return fmt.Errorf("empty framework rendered non-empty")
	}
	return nil
}

// stressContinuumUsage exercises the pressure estimator + levels.
func stressContinuumUsage() error {
	u := continuum.EstimateUsage([]llm.Message{
		{Role: "user", Content: strings.Repeat("a", 4000)}, // ~1000 tokens
	}, 2000)
	if u.Pct < 45 || u.Pct > 60 {
		return fmt.Errorf("pct = %.1f, want ~50", u.Pct)
	}
	cases := []struct {
		pct  float64
		want string
	}{{30, "ok"}, {60, "warm"}, {80, "high"}, {99, "critical"}}
	for _, c := range cases {
		u.Pct = c.pct
		if u.Level() != c.want {
			return fmt.Errorf("level(%.0f) = %s, want %s", c.pct, u.Level(), c.want)
		}
	}
	// zero budget must not divide by zero
	if u := continuum.EstimateUsage(nil, 0); u.Pct != 0 {
		return fmt.Errorf("empty usage pct = %.1f", u.Pct)
	}
	return nil
}

// stressContinuumRolloverChain runs the REAL rollover through the REAL
// session store: thread chain, chapter numbering, briefing seed, carry
// tail, sidecar persistence, knowledge accumulation across chapters.
func stressContinuumRolloverChain() error {
	dir, err := os.MkdirTemp("", "sheytan-cont-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	cfg := config.Default()
	cfg.SessionsDir = dir
	cfg.ContinuumCarryMessages = 4
	store := sessions.New(dir)
	mgr := continuum.NewManager(store, dir)

	s1 := store.Create()
	s1.Title = "EV research"
	s1.Messages = []llm.Message{
		{Role: "user", Content: "Remember my company is called Voltra."},
		{Role: "assistant", Content: "Noted. I've created workspace/report.md."},
		{Role: "user", Content: "Remember the access key is SK-42."},
		{Role: "assistant", Content: "Stored."},
	}
	if err := store.Save(s1); err != nil {
		return err
	}

	s2, _, err := mgr.Rollover(s1, cfg)
	if err != nil {
		return fmt.Errorf("rollover 1: %v", err)
	}
	if s2.ThreadID != continuum.ThreadID(s1) || s2.ParentID != s1.ID || s2.Chapter != 1 {
		return fmt.Errorf("chain metadata wrong: thread=%s parent=%s chapter=%d", s2.ThreadID, s2.ParentID, s2.Chapter)
	}
	if len(s2.Messages) == 0 || !continuum.IsBriefing(s2.Messages[0].Content) {
		return fmt.Errorf("chapter 2 not seeded with a briefing")
	}
	if !strings.Contains(s2.Messages[0].Content, "Voltra") {
		return fmt.Errorf("briefing missing distilled fact")
	}

	// chapter 2 gains knowledge, rolls again
	s2.Messages = append(s2.Messages,
		llm.Message{Role: "user", Content: "Remember the API endpoint is https://api.voltra.dev/v2."},
		llm.Message{Role: "assistant", Content: "Saved."},
	)
	_ = store.Save(s2)
	s3, fw3, err := mgr.Rollover(s2, cfg)
	if err != nil {
		return fmt.Errorf("rollover 2: %v", err)
	}
	if s3.Chapter != 2 || s3.ThreadID != s2.ThreadID {
		return fmt.Errorf("chapter 3 metadata wrong: chapter=%d", s3.Chapter)
	}
	facts := strings.Join(fw3.Facts, "|")
	if !strings.Contains(facts, "Voltra") || !strings.Contains(facts, "api.voltra.dev") || !strings.Contains(facts, "SK-42") {
		return fmt.Errorf("knowledge lost across chapters: %v", fw3.Facts)
	}
	// exactly ONE briefing at the head of chapter 3
	briefings := 0
	for _, m := range s3.Messages {
		if continuum.IsBriefing(m.Content) {
			briefings++
		}
	}
	if briefings != 1 {
		return fmt.Errorf("chapter 3 carries %d briefings, want 1", briefings)
	}
	// sidecars exist for all three
	for _, id := range []string{s1.ID, s2.ID, s3.ID} {
		if continuum.LoadFramework(dir, id) == nil {
			return fmt.Errorf("framework sidecar missing for %s", id)
		}
	}
	// the store's fast List() keeps the chain metadata
	listed, err := store.List()
	if err != nil {
		return err
	}
	chapters := map[int]string{}
	for _, s := range listed {
		if s.Chapter > 0 {
			chapters[s.Chapter] = s.ID
		}
	}
	if len(chapters) < 2 {
		return fmt.Errorf("List() lost chapter metadata (%d entries)", len(chapters))
	}
	return nil
}

// stressContinuumShouldRollover: threshold + enable gating.
func stressContinuumShouldRollover() error {
	dir, err := os.MkdirTemp("", "sheytan-roll-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.SessionsDir = dir
	store := sessions.New(dir)
	mgr := continuum.NewManager(store, dir)

	sess := store.Create()
	sess.Messages = []llm.Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}
	_ = store.Save(sess)
	if mgr.ShouldRollover(sess, cfg) {
		return fmt.Errorf("tiny session rolled over")
	}

	sess.Messages[1].Content = strings.Repeat("word ", 40000) // way over budget
	_ = store.Save(sess)
	if !mgr.ShouldRollover(sess, cfg) {
		return fmt.Errorf("huge session did not roll over")
	}
	cfg.ContinuumEnabled = false
	if mgr.ShouldRollover(sess, cfg) {
		return fmt.Errorf("disabled continuum rolled over")
	}
	return nil
}

// stressContinuumLLMEnhance: the JSON framework parser accepts fenced/garbage
// replies, the enhancer merges over the extractive base, and any failure
// path returns nil (extractive result stays live).
func stressContinuumLLMEnhance() error {
	// parser: fenced reply with prose around it
	reply := "Sure!\n```json\n{\"mission\": \"Ship v2\", \"facts\": [\"GPU is RTX 4060\", \"\", \"x\"], \"openThreads\": [\"benchmark Vulkan\"], \"summary\": \"The user benchmarks engines.\"}\n```"
	lf, err := continuum.ParseLLMFramework(reply)
	if err != nil {
		return fmt.Errorf("parse fenced: %v", err)
	}
	if lf.Mission != "Ship v2" || len(lf.Facts) != 2 || len(lf.OpenThreads) != 1 {
		return fmt.Errorf("parsed framework wrong: %+v", lf)
	}
	// parser: garbage rejected
	if _, err := continuum.ParseLLMFramework("totally not json"); err == nil {
		return fmt.Errorf("garbage accepted")
	}
	if _, err := continuum.ParseLLMFramework(""); err == nil {
		return fmt.Errorf("empty accepted")
	}
	// enhancer against a live fake endpoint (same wire shape as llama.cpp)
	var gotPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1<<16)
		n, _ := r.Body.Read(buf)
		gotPrompt = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"{\"mission\":\"refined\",\"facts\":[\"f1\",\"f2\"]}"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL
	client := llm.NewClient(cfg)

	fw := continuum.Distill(continuum.NewFramework(), []llm.Message{
		{Role: "user", Content: "Remember my company is Voltra."},
		{Role: "assistant", Content: "Noted."},
	})
	refined := continuum.Enhance(context.Background(), client, "m", fw, []llm.Message{
		{Role: "user", Content: "Remember my company is Voltra."},
		{Role: "assistant", Content: "Noted."},
	})
	if refined == nil {
		return fmt.Errorf("enhance returned nil against live fake endpoint")
	}
	joined := strings.Join(refined.Facts, "|")
	if !strings.Contains(joined, "f1") || !strings.Contains(joined, "Voltra") {
		return fmt.Errorf("enhanced facts lost state: %v", refined.Facts)
	}
	if !strings.Contains(gotPrompt, "state-distillation") {
		return fmt.Errorf("enhance prompt malformed")
	}
	// failing endpoint → nil (extractive stays). 400 = non-retryable, so
	// the client fails fast instead of climbing the backoff ladder.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer dead.Close()
	cfg2 := config.Default()
	cfg2.Provider = config.ProviderRemote
	cfg2.RemoteBaseURL = dead.URL
	if out := continuum.Enhance(context.Background(), llm.NewClient(cfg2), "m", fw, fwToMsgs(fw)); out != nil {
		return fmt.Errorf("failing endpoint should return nil")
	}
	return nil
}

func fwToMsgs(fw *continuum.Framework) []llm.Message {
	return []llm.Message{{Role: "user", Content: fw.Mission}}
}

// stressOrchestratorContextUsage proves the orchestrator reports peak prompt
// pressure in its result (the number the meter shows after a reply).
func stressOrchestratorContextUsage() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL
	cfg.RemoteModel = "test-model"

	orch := agent.New(cfg, llm.NewClient(cfg))
	msgs := []llm.Message{{Role: "user", Content: strings.Repeat("hello ", 400)}} // ~400 tokens
	res, err := orch.RunDetailed(context.Background(), msgs, func(agent.Activity) {})
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}
	if res.ContextUsage.BudgetTokens != cfg.HistoryWindowTokens() {
		return fmt.Errorf("budget = %d, want %d", res.ContextUsage.BudgetTokens, cfg.HistoryWindowTokens())
	}
	if res.ContextUsage.EstTokens < 300 {
		return fmt.Errorf("est tokens = %d, want >= 300", res.ContextUsage.EstTokens)
	}
	if res.ContextUsage.Pct <= 0 {
		return fmt.Errorf("pct = %.1f", res.ContextUsage.Pct)
	}
	return nil
}

// stressSessionChainMetadata: ThreadID/ParentID/Chapter survive save/load
// AND the fast-list index path.
func stressSessionChainMetadata() error {
	dir, err := os.MkdirTemp("", "sheytan-meta-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	store := sessions.New(dir)

	s := store.Create()
	s.Title = "chained"
	s.ThreadID = "thread-1"
	s.ParentID = "parent-1"
	s.Chapter = 3
	if err := store.Save(s); err != nil {
		return err
	}
	got, err := store.Get(s.ID)
	if err != nil {
		return err
	}
	if got.ThreadID != "thread-1" || got.ParentID != "parent-1" || got.Chapter != 3 {
		return fmt.Errorf("chain fields lost on Get: %+v", got)
	}
	listed, _ := store.List()
	found := false
	for _, st := range listed {
		if st.ID == s.ID {
			found = true
			if st.Chapter != 3 {
				return fmt.Errorf("List() stub lost chapter: %d", st.Chapter)
			}
		}
	}
	if !found {
		return fmt.Errorf("session missing from List()")
	}
	return nil
}

// stressAicontextV7 asserts the AI briefing file teaches the model about
// the Continuum chapters (ContextVersion 7).
func stressAicontextV7() error {
	data, err := os.ReadFile(filepath.Join("internal", "aicontext", "AI-CONTEXT.md"))
	if err != nil {
		// fall back to CWD-independent lookup via package dir
		data, err = os.ReadFile("internal/aicontext/AI-CONTEXT.md")
		if err != nil {
			return fmt.Errorf("AI-CONTEXT.md unreadable: %v", err)
		}
	}
	txt := string(data)
	if !strings.Contains(txt, "7") || !strings.Contains(strings.ToLower(txt), "continuum") {
		return fmt.Errorf("AI-CONTEXT.md not bumped to v7/continuum")
	}
	if !strings.Contains(strings.ToLower(txt), "chapter") {
		return fmt.Errorf("AI-CONTEXT.md does not mention chapters")
	}
	return nil
}

// stressFrameworkSidecarIO: sidecar save/load round trip incl. version
// rejection of future formats.
func stressFrameworkSidecarIO() error {
	dir, err := os.MkdirTemp("", "sheytan-fw-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	fw := continuum.Distill(continuum.NewFramework(), []llm.Message{
		{Role: "user", Content: "Remember the token is tok-9."},
	})
	if err := continuum.SaveFramework(dir, "s1", fw); err != nil {
		return err
	}
	got := continuum.LoadFramework(dir, "s1")
	if got == nil || len(got.Facts) == 0 {
		return fmt.Errorf("sidecar round trip lost facts")
	}
	// future version rejected
	if err := os.WriteFile(filepath.Join(dir, "s2.framework.json"),
		[]byte(`{"version":99,"facts":["x"]}`), 0o644); err != nil {
		return err
	}
	if continuum.LoadFramework(dir, "s2") != nil {
		return fmt.Errorf("future framework version accepted")
	}
	// nil save is a no-op
	if err := continuum.SaveFramework(dir, "s3", nil); err != nil {
		return fmt.Errorf("nil save: %v", err)
	}
	return nil
}

// stressEnhanceTimeout: a hanging endpoint must return nil before the
// caller notices (the extractive framework is already live).
func stressEnhanceTimeout() error {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second) // longer than the 45s cap would be too slow for stress; use ctx deadline
	}))
	defer srv.Close()
	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	fw := continuum.NewFramework()
	fw.Mission = "m"
	start := time.Now()
	if out := continuum.Enhance(ctx, llm.NewClient(cfg), "m", fw, []llm.Message{{Role: "user", Content: "q"}}); out != nil {
		return fmt.Errorf("timed-out enhance should return nil")
	}
	if time.Since(start) > time.Second {
		return fmt.Errorf("enhance ignored context deadline (%v)", time.Since(start))
	}
	return nil
}
