// Package runtime wires the full SHEYTAN agent stack: LLM client,
// orchestrator with every built-in tool, memory, multi-agent layer, and the
// llama.cpp subprocess manager. Both the desktop GUI and the headless `ask`
// CLI build on this so they stay feature-identical.
package runtime

import (
        "fmt"

        "github.com/sheytan/local-agent/internal/agent"
        "github.com/sheytan/local-agent/internal/aicontext"
        "github.com/sheytan/local-agent/internal/config"
        "github.com/sheytan/local-agent/internal/llm"
        "github.com/sheytan/local-agent/internal/logging"
        "github.com/sheytan/local-agent/internal/memory"
        "github.com/sheytan/local-agent/internal/multiagent"
        "github.com/sheytan/local-agent/internal/recall"
        "github.com/sheytan/local-agent/internal/sandbox"
        "github.com/sheytan/local-agent/internal/sessions"
        "github.com/sheytan/local-agent/internal/tools"
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

        // v1.0.1: materialize AI-CONTEXT.md in the app folder (user-readable,
        // user-editable, version-managed) and remember its path for the UI/CLI.
        if path, err := aicontext.EnsureFile(cfg.DataDir); err != nil {
                logging.Default().Warn("runtime", "AI context file: %v", err)
        } else {
                logging.Default().Info("runtime", "AI context file: %s", path)
        }

        // Canonical base dir for every tool: the portable app folder.
        // Relative paths in files/shell/git/dataAnalysis all resolve here,
        // so chained workflows (files→dataAnalysis→git) never break.
        tools.SetBaseDir(cfg.DataDir)

        // Core tools
        orch.Register(tools.Shell{})
        orch.Register(tools.Files{})
        orch.Register(tools.CodeExec{})
        orch.Register(tools.WebSearch{})
        orch.Register(tools.Git{})
        orch.Register(tools.NewBrowserTool(cfg)) // browser automation
        orch.Register(tools.NewDataTool(cfg))    // data analysis + charts

        // v1.0.6: vision + terminal
        llamaSrv := llm.NewLlamaServer(cfg)
        orch.Register(tools.Screenshot{})
        linuxSim := tools.NewLinuxSim(cfg.DataDir)
        orch.Register(linuxSim)
        // Vision gate: the screenshot tool refuses politely when the engine
        // cannot see images (no projector paired / remote provider).
        tools.VisionCheck = func() error {
                if cfg.IsRemote() {
                        return fmt.Errorf("the remote provider does not accept tool-result images — switch to the local engine with an mmproj projector, or attach the image to your message instead")
                }
                if !llamaSrv.VisionActive() {
                        if !cfg.VisionEnabled {
                                return fmt.Errorf("vision is disabled in Settings — enable it and add an mmproj-*.gguf projector to the models folder")
                        }
                        return fmt.Errorf("no multimodal projector paired with the current model — drop a matching mmproj-*.gguf (e.g. mmproj-gemma-4-E2B-it-BF16.gguf) into the models folder and restart the engine")
                }
                return nil
        }

        // Memory + persistent recall (v1.0.2): the recall engine indexes a tiny
        // digest of every completed exchange and re-injects the most relevant
        // ones into each new turn — past chats stay usable without re-feeding
        // them into the context window.
        mem := memory.New(cfg.DataDir + "/memory.jsonl")
        engine := recall.New(cfg.DataDir)
        orch.Register(memory.Tool{
                Store: mem,
                RecallSearch: func(query string, k int) []string {
                        var lines []string
                        for _, c := range engine.Search(query, k) {
                                lines = append(lines, formatCapsuleLine(c))
                        }
                        return lines
                },
        })
        if cfg.RecallEnabled {
                orch.SetRecaller(engine)
                go func() {
                        store := sessions.New(cfg.SessionsDir)
                        if err := engine.Backfill(store); err != nil {
                                logging.Default().Warn("recall", "backfill: %v", err)
                        } else if n := engine.Count(); n > 0 {
                                logging.Default().Info("recall", "index ready: %d past exchanges", n)
                        }
                }()
        }

        // Job-Object sandbox (overrides plain codeExec when available);
        // workdirs live under <app folder>/sandbox/
        sb, sbErr := sandbox.NewCodeExecSandbox(512, 25, cfg.SandboxDir())
        if sbErr == nil {
                orch.Register(sb)
        } else {
                logging.Default().Warn("runtime", "Job-Object sandbox unavailable, using plain codeExec: %v", sbErr)
        }

        multi := multiagent.NewMultiAgent(client, orch, mem, cfg.EffectiveModel, cfg.EffectiveMultiAgentDepth())

        return &Stack{
                Cfg:     cfg,
                Client:  client,
                Orch:    orch,
                Multi:   multi,
                Mem:     mem,
                Llama:   llamaSrv,
                Sandbox: sb,
                Recall:  engine,
                Linux:   linuxSim,
        }
}

// formatCapsuleLine renders one recall capsule for the memory tool's
// history action.
func formatCapsuleLine(c recall.Capsule) string {
        line := c.TS.Format("2006-01-02") + " [" + c.SessionID + "]"
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
//   - provider "local": boots the bundled llama.cpp server (downloads the
//     binary on first run) unless one is already running
//   - provider "remote": nothing to boot — the endpoint is used as-is
func (s *Stack) EnsureLLM() error {
        if s.Cfg.IsRemote() {
                logging.Default().Info("runtime", "remote provider active: %s (model %s)", s.Cfg.RemoteBaseURL, s.Cfg.EffectiveModel())
                return nil
        }
        if err := s.Llama.Start(); err != nil {
                return err
        }
        return nil
}

// Close tears down every owned subprocess/handle.
func (s *Stack) Close() {
        if bt := s.BrowserTool(); bt != nil {
                bt.Close()
        }
        if s.Sandbox != nil {
                _ = s.Sandbox.Close()
        }
        _ = s.Llama.Stop()
}
