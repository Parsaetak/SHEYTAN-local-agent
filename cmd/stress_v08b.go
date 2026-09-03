package cmd

// stress_v08b.go — retry-path chaos test: a 429 followed by success must be
// transparently retried, not surfaced as a fatal error.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
)

func stressLLMRetry429() error {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n <= 2 {
			// first two attempts: rate limited
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":{"message":"Too many requests, please try again later"}}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"finally ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "fake"
	client := llm.NewClient(cfg)

	resp, err := client.Chat(context.Background(), client.BuildChatRequest("fake", nil, nil))
	if err != nil {
		return fmt.Errorf("429 should have been retried, got: %v", err)
	}
	if got := resp.Choices[0].Message.Content; got != "finally ok" {
		return fmt.Errorf("unexpected content after retry: %q", got)
	}
	if got := hits.Load(); got != 3 {
		return fmt.Errorf("expected 3 attempts, got %d", got)
	}
	return nil
}

func stressLLMRetryStreaming429() error {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"error":{"message":"model overloaded"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"stream ok\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "fake"
	client := llm.NewClient(cfg)

	var content string
	err := client.StreamChat(context.Background(), client.BuildChatRequest("fake", nil, nil), func(ev llm.StreamEvent) error {
		if ev.Content != "" {
			content += ev.Content
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("stream 500 should have been retried, got: %v", err)
	}
	if content != "stream ok" {
		return fmt.Errorf("unexpected streamed content: %q", content)
	}
	return nil
}

// stressToolErrorKeepsOutput verifies the orchestrator surfaces tool output
// alongside errors (git-style diagnostics must not be swallowed).
func stressToolErrorKeepsOutput() error {
	// The git tool with a repo that has no commits yet: `git log` errors
	// but prints useful stderr. Run through a manual tool call the same
	// way the orchestrator does.
	dir, _ := osMkdirTemp("sheytan-git-err-*")
	defer osRemoveAll(dir)
	// init a repo without identity so commit fails
	g := runGitInit(dir)
	_ = g
	out, err := runGit(dir, "log")
	if err == nil {
		return fmt.Errorf("git log should fail on empty repo")
	}
	if out == "" {
		return fmt.Errorf("git tool returned no output on error — orchestrator would hide diagnostics")
	}
	return nil
}
