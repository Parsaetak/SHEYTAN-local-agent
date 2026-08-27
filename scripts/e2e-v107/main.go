// e2e-v107 — Continuum chapter-rollover E2E against a REAL LLM (GLM via the
// z-ai proxy at 127.0.0.1:8177).
//
// Proves the whole "almost unlimited context" chain end-to-end:
//
//  1. A REAL exchange (chapter 1) happens through the orchestrator.
//  2. Under context pressure the manager rolls the session into chapter 2,
//     seeded with the distilled Framework briefing.
//  3. A REAL model answers a question in chapter 2 whose answer exists ONLY
//     in the chapter-1 exchange — carried by the briefing (the carried tail
//     is deliberately drowned; the framework is the clean path to the fact).
//  4. Enhance() against the live endpoint is best-effort: refined framework
//     or nil, never a panic.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sheytan/local-agent/internal/agent"
	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/continuum"
	"github.com/sheytan/local-agent/internal/llm"
	"github.com/sheytan/local-agent/internal/sessions"
)

var pass, fail int

func check(name, needle string, haystack string) {
	if strings.Contains(strings.ToLower(haystack), strings.ToLower(needle)) {
		fmt.Printf("PASS: %s\n", name)
		pass++
	} else {
		fmt.Printf("FAIL: %s (missing %q)\n", name, needle)
		if len(haystack) > 400 {
			haystack = haystack[:400] + "…"
		}
		fmt.Printf("--- got: %s\n---\n", haystack)
		fail++
	}
}

func main() {
	dataDir := os.Getenv("SHEYTAN_DATA_DIR")
	if dataDir == "" {
		dataDir = "/tmp/e2e-v107-data"
		_ = os.Setenv("SHEYTAN_DATA_DIR", dataDir)
	}
	cfg := config.Default()
	cfg.DataDir = dataDir
	cfg.SessionsDir = dataDir + "/sessions"
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = "http://127.0.0.1:8177/v1"
	cfg.RemoteModel = "glm-proxy"
	// Small context so a modest chapter crosses the rollover threshold for
	// real (budget = 60% of 1024 ≈ 614 tokens).
	cfg.LLM.NumCtx = 1024
	cfg.MaxIterations = 6
	_ = os.RemoveAll(dataDir)
	_ = os.MkdirAll(cfg.SessionsDir, 0o755)

	client := llm.NewClient(cfg)
	orch := agent.New(cfg, client)
	store := sessions.New(cfg.SessionsDir)
	nop := func(agent.Activity) {}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	fmt.Println("===== E2E v1.0.7: Continuum chapter rollover (real LLM) =====")

	// --- chapter 1: a real exchange with a durable fact -------------------
	s1 := store.Create()
	s1.Title = "Voltra launch"
	ask1 := "IMPORTANT — remember this for the rest of the conversation: my company is called Voltra Industries and our launch code is 7-7-7-EMBER. Acknowledge briefly, no tools."
	res1, err := orch.Run(ctx, []llm.Message{{Role: "user", Content: ask1}}, nop)
	if err != nil {
		fmt.Printf("FAIL: chapter1_real_exchange (%v)\n", err)
		os.Exit(1)
	}
	s1.Messages = []llm.Message{
		{Role: "user", Content: ask1},
		{Role: "assistant", Content: res1},
	}
	fmt.Printf("chapter 1 reply: %.160s\n", res1)
	check("chapter1_acknowledges", "voltra", res1)

	// --- pressure + rollover -----------------------------------------------
	// Simulate a long chapter: append a large (non-LLM) work log so the
	// session crosses the threshold, then roll over.
	s1.Messages = append(s1.Messages, llm.Message{
		Role:    "assistant",
		Content: "Work log: " + strings.Repeat("benchmarked the token pipeline; latency stable; cache warm; ", 220),
	})
	_ = store.Save(s1)

	mgr := continuum.NewManager(store, cfg.SessionsDir)
	if !mgr.ShouldRollover(s1, cfg) {
		fmt.Printf("FAIL: should_rollover (usage %.0f%%)\n", continuum.SessionUsage(s1, cfg).Pct)
		os.Exit(1)
	}
	fmt.Println("PASS: should_rollover")
	pass++

	s2, fw, err := mgr.Rollover(s1, cfg)
	if err != nil {
		fmt.Printf("FAIL: rollover (%v)\n", err)
		os.Exit(1)
	}
	fmt.Println("PASS: rollover_creates_chapter2")
	pass++
	check("briefing_carries_fact", "Voltra", s2.Messages[0].Content)
	check("briefing_carries_code", "7-7-7-EMBER", s2.Messages[0].Content)
	briefings := 0
	for _, m := range s2.Messages {
		if continuum.IsBriefing(m.Content) {
			briefings++
		}
	}
	if briefings == 1 {
		fmt.Println("PASS: exactly_one_briefing")
		pass++
	} else {
		fmt.Printf("FAIL: exactly_one_briefing (%d)\n", briefings)
		fail++
	}

	// --- chapter 2: the REAL model must know chapter 1 through the briefing
	q2 := "Using ONLY your working memory of this conversation thread (the CONTINUUM FRAMEWORK block): what is the name of my company, and what is our launch code? Answer in ONE short line, no tools."
	msgs := append(append([]llm.Message{}, s2.Messages...), llm.Message{Role: "user", Content: q2})
	res2, err := orch.Run(ctx, msgs, nop)
	if err != nil {
		fmt.Printf("FAIL: chapter2_real_answer (%v)\n", err)
		os.Exit(1)
	}
	fmt.Printf("chapter 2 reply: %.200s\n", res2)
	check("chapter2_knows_company", "Voltra", res2)
	check("chapter2_knows_code", "7-7-7-EMBER", res2)

	// --- Enhance against the live endpoint (best-effort) -------------------
	refined := continuum.Enhance(ctx, client, cfg.EffectiveModel(), fw, s1.Messages)
	if refined == nil {
		fmt.Println("PASS: enhance_best_effort_nil (endpoint declined; extractive stays live)")
		pass++
	} else if refined.FactCount() > 0 {
		fmt.Printf("PASS: enhance_refined (%d items, mission: %.60s)\n", refined.FactCount(), refined.Mission)
		pass++
	} else {
		fmt.Println("FAIL: enhance_refined (empty framework)")
		fail++
	}

	fmt.Println("==============================================")
	fmt.Printf("E2E v1.0.7 RESULT: %d pass / %d fail\n", pass, fail)
	if fail > 0 {
		os.Exit(1)
	}
}
