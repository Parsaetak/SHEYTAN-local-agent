// Package runtime wires the full SHEYTAN agent stack: LLM client,
// orchestrator with every built-in tool, memory, multi-agent layer,
// research, and the llama.cpp subprocess manager. Both the desktop GUI
// and the headless `ask` CLI build on this so they stay feature-identical.
package runtime

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/agent"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/aicontext"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
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
			githubProvider := research.NewGitHubProvider(
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
			redditProvider := research.NewRedditProvider(
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
			duckDuckGoProvider :=
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
			searxngProvider :=
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

// EnsureLLM makes sure an LLM backend is reachable:
//
//   - provider "local": boots the bundled llama.cpp server unless one is
//     already running
//   - provider "remote": nothing to boot — the endpoint is used as-is
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
