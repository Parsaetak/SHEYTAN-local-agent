// Package runtime wires the full SHEYTAN agent stack: LLM client,
// orchestrator with every built-in tool, attachments, context cache,
// memory, multi-agent layer, research, and the llama.cpp subprocess
// manager. Both the desktop GUI and the headless `ask` CLI build on this
// so they stay feature-identical.
package runtime

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/agent"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/aicontext"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/attachments"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/contextcache"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/lab"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/logging"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/memory"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/multiagent"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/recall"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/research"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sandbox"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sessions"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/tools"
)

// Stack is the fully-wired agent runtime.
type Stack struct {
	Cfg     *config.Config
	Client  *llm.Client
	Orch    *agent.Orchestrator
	Multi   *multiagent.MultiAgent
	Mem     *memory.Store
	Llama   *llm.LlamaServer
	Browser *tools.BrowserTool
	Sandbox *sandbox.CodeExecSandbox
	Recall  *recall.Engine

	// Lab is the autonomous Coding Lab tool.
	Lab *lab.Tool

	// Research is the unified external research service.
	Research *research.Service

	// ResearchTool is the agent-facing research tool backed by Research.
	ResearchTool *research.Tool

	// Attachments is the staged-file store backing real file uploads.
	Attachments *attachments.Manager

	// Cache is the process-wide content-aware context cache.
	Cache *contextcache.Cache

	// Linux (v1.0.6) is the built-in Linux-like shell used by BOTH the agent
	// (the `linux` tool) and the Terminal view — one shared instance so the
	// user sees (and can replay) exactly what the agent did.
	Linux *tools.LinuxSim
}

// NewStack wires every tool into the orchestrator. The sandbox is optional —
// if the Job-Object sandbox can't be created, the plain codeExec tool stays
// registered.
func NewStack(cfg *config.Config) *Stack {
	client := llm.NewClient(cfg)
	orch := agent.New(cfg, client)

	// v1.1.3Z: content-aware context cache shared by attachments, chunking
	// pipelines and retrieval.
	cache := contextcache.New()

	// v1.1.3Z: real attachment staging under the app's private data dir.
	attMgr, attErr := attachments.NewManager(
		filepath.Join(cfg.DataDir, "attachments"),
		attachments.Options{Cache: cache},
	)

	if attErr != nil {
		logging.Default().Warn(
			"runtime",
			"attachment store unavailable: %v",
			attErr,
		)
	}

	// v1.0.1: materialize AI-CONTEXT.md in the app folder.
	if path, err := aicontext.EnsureFile(
		cfg.DataDir,
	); err != nil {
		logging.Default().Warn(
			"runtime",
			"AI context file: %v",
			err,
		)
	} else {
		logging.Default().Info(
			"runtime",
			"AI context file: %s",
			path,
		)
	}

	// Canonical base dir for every tool.
	tools.SetBaseDir(cfg.DataDir)

	// Core tools.
	orch.Register(tools.Shell{})
	orch.Register(tools.Files{})
	orch.Register(tools.CodeExec{})
	orch.Register(tools.WebSearch{})
	orch.Register(tools.Git{})
	orch.Register(tools.NewBrowserTool(cfg))
	orch.Register(tools.NewDataTool(cfg))

	// v1.0.10 (PRISM): structured data, archives, URLs, verification.
	orch.Register(tools.JSONTool{})
	orch.Register(tools.ArchiveTool{})
	orch.Register(tools.NewFetchTool())
	orch.Register(tools.DiffTool{})

	// v1.0.6: vision + terminal.
	llamaSrv := llm.NewLlamaServer(cfg)

	orch.Register(tools.Screenshot{})

	linuxSim := tools.NewLinuxSim(
		cfg.DataDir,
	)

	orch.Register(linuxSim)

	// Version Zeta: autonomous Coding Lab.
	var labTool *lab.Tool

	if cfg.LabEnabled {
		var err error

		labTool, err = lab.NewTool(cfg)

		if err != nil {
			logging.Default().Warn(
				"runtime",
				"Coding Lab unavailable: %v",
				err,
			)
		} else {
			orch.Register(labTool)

			logging.Default().Info(
				"runtime",
				"Coding Lab registered: workspace=%s network=%t",
				cfg.LabWorkspaceRoot,
				cfg.LabAllowNetwork,
			)
		}
	}

	// Version Zeta: unified external research.
	var researchService *research.Service
	var researchTool *research.Tool

	if cfg.ResearchEnabled {
		researchConfig := research.ServiceConfig{
			Backend:    cfg.ResearchBackend,
			MaxResults: cfg.ResearchMaxResults,
			Timeout: researchTimeout(
				cfg.ResearchTimeoutSec,
			),
		}

		researchService = research.NewService(
			researchConfig,
		)

		researchHTTPClient := &http.Client{
			Timeout: researchTimeout(
				cfg.ResearchTimeoutSec,
			),
		}

		researchCacheTTL := researchCacheTTL(
			cfg.ResearchCacheTTLMin,
		)

		if cfg.ResearchGitHub {
			var githubProvider research.Provider

			githubProvider = research.NewGitHubProvider(
				researchHTTPClient,
				"",
				"",
			)

			githubProvider = research.NewCachedProvider(
				githubProvider,
				researchCacheTTL,
			)

			if err := researchService.Register(
				githubProvider,
			); err != nil {
				logging.Default().Warn(
					"research",
					"GitHub provider unavailable: %v",
					err,
				)
			} else {
				logging.Default().Info(
					"research",
					"GitHub provider registered",
				)
			}
		}

		if cfg.ResearchReddit {
			var redditProvider research.Provider

			redditProvider = research.NewRedditProvider(
				researchHTTPClient,
				"",
				"",
				cfg.ResearchUserAgent,
			)

			redditProvider = research.NewCachedProvider(
				redditProvider,
				researchCacheTTL,
			)

			if err := researchService.Register(
				redditProvider,
			); err != nil {
				logging.Default().Warn(
					"research",
					"Reddit provider unavailable: %v",
					err,
				)
			} else {
				logging.Default().Info(
					"research",
					"Reddit provider registered",
				)
			}
		}

		if cfg.ResearchWeb {
			var duckDuckGoProvider research.Provider

			duckDuckGoProvider =
				research.NewDuckDuckGoProvider(
					researchHTTPClient,
					"",
				)

			duckDuckGoProvider = research.NewCachedProvider(
				duckDuckGoProvider,
				researchCacheTTL,
			)

			if err := researchService.Register(
				duckDuckGoProvider,
			); err != nil {
				logging.Default().Warn(
					"research",
					"DuckDuckGo provider unavailable: %v",
					err,
				)
			} else {
				logging.Default().Info(
					"research",
					"DuckDuckGo provider registered",
				)
			}
		}

		if cfg.ResearchSearXNGURL != "" {
			var searxngProvider research.Provider

			searxngProvider =
				research.NewSearXNGProvider(
					researchHTTPClient,
					cfg.ResearchSearXNGURL,
				)

			searxngProvider = research.NewCachedProvider(
				searxngProvider,
				researchCacheTTL,
			)

			if err := researchService.Register(
				searxngProvider,
			); err != nil {
				logging.Default().Warn(
					"research",
					"SearXNG provider unavailable: %v",
					err,
				)
			} else {
				logging.Default().Info(
					"research",
					"SearXNG provider registered: %s",
					cfg.ResearchSearXNGURL,
				)
			}
		}

		tool, err := research.NewTool(
			researchService,
		)

		if err != nil {
			logging.Default().Warn(
				"research",
				"research tool unavailable: %v",
				err,
			)
		} else {
			researchTool = tool

			orch.Register(researchTool)

			logging.Default().Info(
				"research",
				"unified research tool registered: backend=%s results=%d timeout=%s cache=%s providers=%v",
				researchService.Backend(),
				cfg.ResearchMaxResults,
				researchTimeout(
					cfg.ResearchTimeoutSec,
				),
				researchCacheTTL,
				researchService.ProviderNames(),
			)
		}
	}

	// Vision gate: the screenshot tool refuses politely when the engine
	// cannot see images.
	tools.VisionCheck = func() error {
		if cfg.IsRemote() {
			return fmt.Errorf(
				"the remote provider does not accept tool-result images — switch to the local engine with an mmproj projector, or attach the image to your message instead",
			)
		}

		if !llamaSrv.VisionActive() {
			if !cfg.VisionEnabled {
				return fmt.Errorf(
					"vision is disabled in Settings — enable it and add an mmproj-*.gguf projector to the models folder",
				)
			}

			return fmt.Errorf(
				"no multimodal projector paired with the current model — drop a matching mmproj-*.gguf (e.g. mmproj-gemma-4-E2B-it-BF16.gguf) into the models folder and restart the engine",
			)
		}

		return nil
	}

	// Memory + persistent recall.
	mem := memory.New(
		cfg.DataDir + "/memory.jsonl",
	)

	engine := recall.New(
		cfg.DataDir,
	)

	orch.Register(memory.Tool{
		Store: mem,
		RecallSearch: func(
			query string,
			k int,
		) []string {
			var lines []string

			for _, c := range engine.Search(
				query,
				k,
			) {
				lines = append(
					lines,
					formatCapsuleLine(c),
				)
			}

			return lines
		},
	})

	if cfg.RecallEnabled {
		orch.SetRecaller(engine)

		go func() {
			store := sessions.New(
				cfg.SessionsDir,
			)

			if err := engine.Backfill(
				store,
			); err != nil {
				logging.Default().Warn(
					"recall",
					"backfill: %v",
					err,
				)
			} else if n := engine.Count(); n > 0 {
				logging.Default().Info(
					"recall",
					"index ready: %d past exchanges",
					n,
				)
			}
		}()
	}

	// Job-Object sandbox (overrides plain codeExec when available).
	sb, sbErr := sandbox.NewCodeExecSandbox(
		512,
		25,
		cfg.SandboxDir(),
	)

	if sbErr == nil {
		orch.Register(sb)
	} else {
		logging.Default().Warn(
			"runtime",
			"Job-Object sandbox unavailable, using plain codeExec: %v",
			sbErr,
		)
	}

	multi := multiagent.NewMultiAgent(
		client,
		orch,
		mem,
		cfg.EffectiveModel,
		cfg.EffectiveMultiAgentDepth(),
	)

	// v1.1.3Z: inference traffic reports engine busy state to the
	// authoritative state machine (no-op unless the local engine is
	// alive, so remote providers are unaffected).
	client.SetBusyHook(llamaSrv.MarkBusy)

	return &Stack{
		Cfg:          cfg,
		Client:       client,
		Orch:         orch,
		Multi:        multi,
		Mem:          mem,
		Llama:        llamaSrv,
		Browser:      nil,
		Sandbox:      sb,
		Recall:       engine,
		Lab:          labTool,
		Research:     researchService,
		ResearchTool: researchTool,
		Attachments:  attMgr,
		Cache:        cache,
		Linux:        linuxSim,
	}
}

// researchTimeout converts the configuration's seconds value
// into a safe service/client timeout.
func researchTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = 20
	}

	return time.Duration(seconds) *
		time.Second
}

// researchCacheTTL converts the configured cache lifetime in minutes.
// Zero or negative values disable caching.
func researchCacheTTL(minutes int) time.Duration {
	if minutes <= 0 {
		return 0
	}

	return time.Duration(minutes) *
		time.Minute
}

// formatCapsuleLine renders one recall capsule for the memory
// tool's history action.
func formatCapsuleLine(
	c recall.Capsule,
) string {
	line := c.TS.Format(
		"2006-01-02",
	) +
		" [" +
		c.SessionID +
		"]"

	if c.Title != "" {
		line += " " + c.Title
	}

	if c.Query != "" {
		line += "\n  asked: " + c.Query
	}

	if c.Answer != "" {
		line += "\n  outcome: " + c.Answer
	}

	return line
}

// BrowserTool returns the shared browser tool registered in the stack.
func (s *Stack) BrowserTool() *tools.BrowserTool {
	if s.Browser != nil {
		return s.Browser
	}

	for _, t := range s.Orch.Tools() {
		if bt, ok := t.(*tools.BrowserTool); ok {
			s.Browser = bt
			return bt
		}
	}

	return nil
}

// EnsureLLM makes sure an LLM backend is reachable and ready:
//
//   - provider "local": boots the bundled llama.cpp server unless one is
//     already alive, and blocks until the model is actually serving
//   - provider "remote": nothing to boot — the endpoint is used as-is
//
// This is the ONE canonical engine gate: every inference path (desktop,
// serve, ask) funnels through it.
func (s *Stack) EnsureLLM() error {
	if s.Cfg.IsRemote() {
		logging.Default().Info(
			"runtime",
			"remote provider active: %s (model %s)",
			s.Cfg.RemoteBaseURL,
			s.Cfg.EffectiveModel(),
		)

		return nil
	}

	if err := s.Llama.Start(); err != nil {
		return err
	}

	return nil
}

// PrewarmLLM boots the local engine in the background so a freshly
// launched application reaches a healthy model WITHOUT any user action
// (v1.1.3Z acceptance: launch → engine starts automatically → ready).
// Failures are logged and reflected in the engine state — never fatal,
// because the user may only be browsing settings; a later explicit start
// or the first message retries through EnsureLLM.
func (s *Stack) PrewarmLLM() {
	if s.Cfg.IsRemote() {
		logging.Default().Info(
			"runtime",
			"remote provider active: %s (model %s) — local engine not started",
			s.Cfg.RemoteBaseURL,
			s.Cfg.EffectiveModel(),
		)

		return
	}

	go func() {
		if err := s.Llama.Start(); err != nil {
			logging.Default().Warn(
				"engine",
				"automatic startup failed (the agent will retry on first use): %v",
				err,
			)

			return
		}

		logging.Default().Info(
			"engine",
			"local engine ready automatically (model %s)",
			s.Cfg.EffectiveModel(),
		)
	}()
}

// EnsureLLMContext is EnsureLLM with a deadline: the run path uses it so a
// cold start can never hang a request forever — the engine either becomes
// ready within the timeout or the request fails with a clear, visible
// error while the startup keeps progressing in the background.
func (s *Stack) EnsureLLMContext(ctx context.Context) error {
	if s.Cfg.IsRemote() {
		return nil
	}

	if s.Llama.IsRunning() {
		return nil
	}

	errCh := make(chan error, 1)

	go func() {
		errCh <- s.Llama.Start()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf(
			"engine startup still in progress: %w",
			ctx.Err(),
		)
	}
}

// Close tears down every owned subprocess/handle.
func (s *Stack) Close() {
	if s.BrowserTool() != nil {
		s.BrowserTool().Close()
	}

	if s.Sandbox != nil {
		_ = s.Sandbox.Close()
	}

	_ = s.Llama.Stop()
}
