// Package api serves the embedded web UI and exposes the JSON REST+WS API
// that the UI talks to.
package api

import (
        "context"
        "encoding/json"
        "fmt"
        "io/fs"
        "net/http"
        "strconv"
        "strings"
        "sync"
        "time"

        "github.com/gorilla/websocket"
        "github.com/sheytan/local-agent/internal/agent"
        "github.com/sheytan/local-agent/internal/chunking"
        "github.com/sheytan/local-agent/internal/config"
        "github.com/sheytan/local-agent/internal/installer"
        "github.com/sheytan/local-agent/internal/llm"
        "github.com/sheytan/local-agent/internal/recall"
        "github.com/sheytan/local-agent/internal/sessions"
        "github.com/sheytan/local-agent/internal/sysinfo"
        "github.com/sheytan/local-agent/internal/tools"
        "github.com/sheytan/local-agent/web"
)

// Server bundles everything the HTTP handlers need.
type Server struct {
        cfg       *config.Config
        store     *sessions.Store
        orch      *agent.Orchestrator
        llama     *llm.LlamaServer
        installer *installer.Manager
        sys       *sysinfo.SysInfo
        recall    *recall.Engine // v1.0.2 persistent memory over past chats

        // active runs: sessionID → runState
        runsMu sync.Mutex
        runs   map[string]*runState
}

type runState struct {
        cancel  context.CancelFunc
        updates chan agent.Activity
}

// New constructs a fully-wired server.
func New(cfg *config.Config) (*Server, error) {
        store := sessions.New(cfg.SessionsDir)
        client := llm.NewClient(cfg)
        orch := agent.New(cfg, client)
        orch.Register(tools.Shell{})
        orch.Register(tools.Files{})
        orch.Register(tools.CodeExec{})
        orch.Register(tools.WebSearch{})
        orch.Register(tools.Git{})
        // v1.0.9: headless serve previously shipped WITHOUT the data tool —
        // the GUI and the serve mode must expose the same agent surface.
        orch.Register(tools.NewDataTool(cfg))
        // v1.0.10 (PRISM): the new tools register in serve mode too — GUI and
        // web stay feature-identical.
        orch.Register(tools.JSONTool{})
        orch.Register(tools.ArchiveTool{})
        orch.Register(tools.NewFetchTool())
        orch.Register(tools.DiffTool{})
        // v1.0.2: persistent recall over past conversations.
        engine := recall.New(cfg.DataDir)
        if cfg.RecallEnabled {
                orch.SetRecaller(engine)
        }
        return &Server{
                cfg:       cfg,
                store:     store,
                orch:      orch,
                llama:     llm.NewLlamaServer(cfg),
                installer: installer.New(cfg),
                runs:      make(map[string]*runState),
                sys:       sysinfo.Probe(),
                recall:    engine,
        }, nil
}

// EnsureSetup runs the installer + creates directories before serving.
func (s *Server) EnsureSetup() error {
        if err := s.cfg.EnsureDirs(); err != nil {
                return err
        }
        if _, _, err := s.installer.EnsureRun(false); err != nil {
                return err
        }
        return nil
}

// Handler returns the http.Handler with all routes mounted.
func (s *Server) Handler() http.Handler {
        mux := http.NewServeMux()

        // Static UI
        staticRoot, _ := fs.Sub(web.StaticFS, "static")
        mux.Handle("/", http.FileServer(http.FS(staticRoot)))

        // REST API
        mux.HandleFunc("/api/state", s.handleState)
        mux.HandleFunc("/api/sysinfo", s.handleSysinfo)
        mux.HandleFunc("/api/presets", s.handlePresets)
        mux.HandleFunc("/api/models", s.handleModels)
        mux.HandleFunc("/api/sessions", s.handleSessions)
        mux.HandleFunc("/api/sessions/", s.handleSession)
        mux.HandleFunc("/api/config", s.handleConfig)
        mux.HandleFunc("/api/llama", s.handleLlama)
        mux.HandleFunc("/api/run", s.handleRun)
        mux.HandleFunc("/api/abort", s.handleAbort)
        mux.HandleFunc("/api/tools", s.handleTools)

        // WebSocket: real-time agent activity for a session
        mux.HandleFunc("/ws/activity", s.handleActivityWS)

        return withCORS(mux)
}

// withCORS allows the dev server (Vite, etc.) to call the API during dev.
func withCORS(h http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Access-Control-Allow-Origin", "*")
                w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
                w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
                if r.Method == "OPTIONS" {
                        w.WriteHeader(204)
                        return
                }
                h.ServeHTTP(w, r)
        })
}

// --- State ---

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
        st, _, _ := s.installer.EnsureRun(false)
        writeJSON(w, map[string]any{
                "appName":    config.AppName,
                "appVersion": config.AppVersion,
                "state":      st,
        })
}

func (s *Server) handleSysinfo(w http.ResponseWriter, r *http.Request) {
        // Re-probe on demand so the UI can refresh
        info := sysinfo.Probe()
        writeJSON(w, info)
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
        writeJSON(w, llm.Presets())
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
        local := llm.ListLocalModels(s.cfg.ModelsDir)
        loaded, err := s.llama.ListLoadedModels()
        if err != nil {
                loaded = nil
        }
        writeJSON(w, map[string]any{
                "local":        local,
                "loaded":       loaded,
                "llamaRunning": s.llama.IsRunning(),
        })
}

func (s *Server) handleLlama(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodGet:
                writeJSON(w, map[string]any{
                        "state": s.llama.State(),
                        "logs":  s.llama.Logs(),
                })
        case http.MethodPost:
                var body struct {
                        Action string `json:"action"`
                }
                _ = json.NewDecoder(r.Body).Decode(&body)
                switch body.Action {
                case "start":
                        if err := s.llama.Start(); err != nil {
                                writeErr(w, 500, err)
                                return
                        }
                case "stop":
                        _ = s.llama.Stop()
                default:
                        writeErr(w, 400, fmt.Errorf("unknown action %q", body.Action))
                        return
                }
                writeJSON(w, map[string]any{"state": s.llama.State()})
        default:
                writeErr(w, 405, fmt.Errorf("method not allowed"))
        }
}

// --- Sessions ---

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodGet:
                list, err := s.store.List()
                if err != nil {
                        writeErr(w, 500, err)
                        return
                }
                writeJSON(w, list)
        case http.MethodPost:
                sess := s.store.Create()
                writeJSON(w, sess)
        default:
                writeErr(w, 405, fmt.Errorf("method not allowed"))
        }
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
        id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
        if id == "" {
                writeErr(w, 400, fmt.Errorf("missing session id"))
                return
        }
        switch r.Method {
        case http.MethodGet:
                sess, err := s.store.Get(id)
                if err != nil {
                        writeErr(w, 404, err)
                        return
                }
                writeJSON(w, sess)
        case http.MethodDelete:
                if err := s.store.Delete(id); err != nil {
                        writeErr(w, 500, err)
                        return
                }
                writeJSON(w, map[string]any{"ok": true})
        case http.MethodPut:
                // Update title/context/etc.
                var body struct {
                        Title   *string           `json:"title,omitempty"`
                        Context *sessions.Context `json:"context,omitempty"`
                        Model   *string           `json:"model,omitempty"`
                }
                if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
                        writeErr(w, 400, err)
                        return
                }
                if body.Title != nil {
                        _ = s.store.UpdateTitle(id, *body.Title)
                }
                if body.Context != nil {
                        _ = s.store.UpdateContext(id, *body.Context)
                }
                if body.Model != nil {
                        _ = s.store.SetModel(id, *body.Model)
                }
                sess, _ := s.store.Get(id)
                writeJSON(w, sess)
        default:
                writeErr(w, 405, fmt.Errorf("method not allowed"))
        }
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodGet:
                writeJSON(w, s.cfg)
        case http.MethodPut, http.MethodPost:
                var body config.Config
                if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
                        writeErr(w, 400, err)
                        return
                }
                // Persist
                if err := config.Save(s.cfg.ConfigPath(), &body); err != nil {
                        writeErr(w, 500, err)
                        return
                }
                s.cfg = &body
                writeJSON(w, map[string]any{"ok": true})
        default:
                writeErr(w, 405, fmt.Errorf("method not allowed"))
        }
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
        var out []map[string]any
        for _, t := range s.orch.Tools() {
                out = append(out, map[string]any{
                        "name":        t.Name(),
                        "description": t.Description(),
                })
        }
        writeJSON(w, out)
}

// --- Run / Abort ---

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
                writeErr(w, 405, fmt.Errorf("method not allowed"))
                return
        }
        var body struct {
                SessionID string `json:"sessionId"`
                Message   string `json:"message"`
        }
        if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
                writeErr(w, 400, err)
                return
        }
        if body.SessionID == "" || body.Message == "" {
                writeErr(w, 400, fmt.Errorf("sessionId and message required"))
                return
        }

        sess, err := s.store.Get(body.SessionID)
        if err != nil {
                writeErr(w, 404, err)
                return
        }

        // Append user message
        userMsg := llm.Message{Role: "user", Content: body.Message}
        sess.Messages = append(sess.Messages, userMsg)
        // If first user message, derive a title
        if len(sess.Messages) == 1 {
                title := body.Message
                if len(title) > 60 {
                        title = title[:60] + "..."
                }
                _ = s.store.UpdateTitle(sess.ID, title)
        }
        _ = s.store.Save(sess)

        // Build the LLM message list (with optional system prompt)
        var messages []llm.Message
        if sess.Context.SystemPrompt != "" {
                messages = append(messages, llm.Message{Role: "system", Content: sess.Context.SystemPrompt})
        }
        // v1.0.2: attach file CONTENTS (not just paths) through the chunking
        // engine — text files inlined within the byte budget, binaries noted
        // as metadata with their path for the files tool.
        // v1.0.6: IMAGE attachments split out and ride the LAST user message
        // as real image parts (the multimodal wire format).
        attachedImages, _ := chunking.SplitAttachments(sess.Context.AttachedFiles)
        if len(sess.Context.AttachedFiles) > 0 {
                note := chunking.ComposeUserMessage("", sess.Context.AttachedFiles, s.cfg.AttachmentsBudgetBytes())
                messages = append(messages, llm.Message{
                        Role:    "system",
                        Content: note,
                })
        }
        messages = append(messages, sess.Messages...)
        // The freshest user message carries the images (the client keeps only
        // the current turn's images live on the wire — same rule everywhere).
        for i := len(messages) - 1; i >= 0; i-- {
                if messages[i].Role == "user" {
                        messages[i].Images = append(messages[i].Images, attachedImages...)
                        break
                }
        }

        // Spawn the run
        ctx, cancel := context.WithCancel(context.Background())
        updates := make(chan agent.Activity, 64)

        s.runsMu.Lock()
        if old, ok := s.runs[sess.ID]; ok {
                old.cancel()
        }
        s.runs[sess.ID] = &runState{cancel: cancel, updates: updates}
        s.runsMu.Unlock()

        go func() {
                defer close(updates)
                defer func() {
                        s.runsMu.Lock()
                        delete(s.runs, sess.ID)
                        s.runsMu.Unlock()
                        cancel()
                }()
                res, err := s.orch.RunDetailed(ctx, messages, func(a agent.Activity) {
                        select {
                        case updates <- a:
                        default:
                        }
                        // v1.0.1 perf: persist MILESTONE events only. Streaming
                        // "response"/"reasoning" deltas arrive ~12/s (already
                        // coalesced in the orchestrator) and each AppendActivity
                        // rewrites the whole session JSON — for a long reply that
                        // meant hundreds of full-file rewrites. The final text is
                        // persisted as the assistant message anyway.
                        switch a.Type {
                        case "response", "thinking", "reasoning":
                                return
                        }
                        _ = s.store.AppendActivity(sess.ID, sessions.ActivityEntry{
                                Type:      a.Type,
                                Caption:   a.Caption,
                                Timestamp: a.Timestamp,
                        })
                })
                if err != nil {
                        updates <- agent.Activity{Type: "error", Caption: err.Error(), Timestamp: time.Now()}
                        return
                }
                // Append assistant reply (content + reasoning trace)
                if res.Text != "" {
                        _, _ = s.store.AppendMessage(sess.ID, llm.Message{
                                Role:      "assistant",
                                Content:   res.Text,
                                Reasoning: res.Reasoning,
                        })
                }
                // v1.0.2: index the completed exchange into persistent recall
                // so later turns (and later sessions) can retrieve it cheaply.
                if s.recall != nil && res.Text != "" {
                        _ = s.recall.IndexTurn(sess.ID, sess.Title, body.Message, res.Text, res.ToolsUsed)
                }
        }()

        writeJSON(w, map[string]any{
                "ok":        true,
                "sessionId": sess.ID,
        })
}

func (s *Server) handleAbort(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
                writeErr(w, 405, fmt.Errorf("method not allowed"))
                return
        }
        var body struct {
                SessionID string `json:"sessionId"`
        }
        _ = json.NewDecoder(r.Body).Decode(&body)
        s.runsMu.Lock()
        defer s.runsMu.Unlock()
        if rs, ok := s.runs[body.SessionID]; ok {
                rs.cancel()
                s.orch.Abort()
        }
        writeJSON(w, map[string]any{"ok": true})
}

// --- WebSocket: live activity ---

func (s *Server) handleActivityWS(w http.ResponseWriter, r *http.Request) {
        sessionID := r.URL.Query().Get("sessionId")
        if sessionID == "" {
                writeErr(w, 400, fmt.Errorf("sessionId required"))
                return
        }
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
                return
        }
        defer conn.Close()

        s.runsMu.Lock()
        rs, ok := s.runs[sessionID]
        s.runsMu.Unlock()
        if !ok {
                // No active run — close after sending a sentinel
                _ = conn.WriteJSON(map[string]any{"type": "idle", "caption": "No active run"})
                return
        }

        // Read loop: client may send {"action":"abort"} to cancel
        go func() {
                for {
                        var msg map[string]any
                        if err := conn.ReadJSON(&msg); err != nil {
                                return
                        }
                        if action, _ := msg["action"].(string); action == "abort" {
                                rs.cancel()
                                s.orch.Abort()
                        }
                }
        }()

        // Forward every activity to the WS client
        for ev := range rs.updates {
                if err := conn.WriteJSON(ev); err != nil {
                        return
                }
        }
        _ = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, v any) {
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
        w.WriteHeader(code)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func parseIntDefault(s string, def int) int {
        n, err := strconv.Atoi(s)
        if err != nil {
                return def
        }
        return n
}
