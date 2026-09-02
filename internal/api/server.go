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
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/agent"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/chunking"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/installer"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/recall"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/runtime"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sessions"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sysinfo"
	"github.com/Parsaetak/SHEYTAN-local-agent/web"
)

// Server bundles everything the HTTP handlers need.
//
// The runtime stack is the single source of truth for the agent backend.
// This keeps the HTTP/API surface feature-identical with the CLI and desktop
// runtime instead of constructing a second independent orchestrator.
type Server struct {
	cfg       *config.Config
	store     *sessions.Store
	stack     *runtime.Stack
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
	cancel context.CancelFunc
	hub    *activityHub
}

// activityHub broadcasts activity events to all WebSocket subscribers of a
// single active run. Each client receives its own buffered channel so clients
// do not consume events from one another.
type activityHub struct {
	mu      sync.RWMutex
	clients map[int]chan agent.Activity
	nextID  int
	closed  bool
}

func newActivityHub() *activityHub {
	return &activityHub{
		clients: make(map[int]chan agent.Activity),
	}
}

func (h *activityHub) subscribe() (int, <-chan agent.Activity, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		ch := make(chan agent.Activity)
		close(ch)
		return 0, ch, func() {}
	}

	id := h.nextID
	h.nextID++

	ch := make(chan agent.Activity, 128)
	h.clients[id] = ch

	var once sync.Once

	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()

			if existing, ok := h.clients[id]; ok {
				delete(h.clients, id)
				close(existing)
			}
		})
	}

	return id, ch, unsubscribe
}

func (h *activityHub) publish(ev agent.Activity) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, ch := range h.clients {
		select {
		case ch <- ev:
		default:
			// A slow WebSocket must never block the agent run.
			// Only that slow subscriber may lose an event.
		}
	}
}

func (h *activityHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}

	h.closed = true

	for id, ch := range h.clients {
		delete(h.clients, id)
		close(ch)
	}
}

// New constructs a fully-wired server from the canonical runtime stack.
func New(cfg *config.Config) (*Server, error) {
	store := sessions.New(cfg.SessionsDir)
	stack := runtime.NewStack(cfg)

	return &Server{
		cfg:       cfg,
		store:     store,
		stack:     stack,
		orch:      stack.Orch,
		llama:     stack.Llama,
		installer: installer.New(cfg),
		runs:      make(map[string]*runState),
		sys:       sysinfo.Probe(),
		recall:    stack.Recall,
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

// withCORS allows only approved local origins during development and
// same-origin requests in normal operation.
//
// Wildcard origins are intentionally prohibited because this API can expose
// local runtime state and perform privileged local operations.
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))

		if origin != "" {
			if !allowedOrigin(origin, r.Host) {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
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
	// Re-probe on demand so the UI can refresh.
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

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}

		switch body.Action {
		case "start":
			if err := s.llama.Start(); err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}

		case "stop":
			_ = s.llama.Stop()

		default:
			writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown action %q", body.Action))
			return
		}

		writeJSON(w, map[string]any{
			"state": s.llama.State(),
		})

	default:
		writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

// --- Sessions ---

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.List()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, list)

	case http.MethodPost:
		sess := s.store.Create()
		writeJSON(w, sess)

	default:
		writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")

	if id == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("missing session id"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		sess, err := s.store.Get(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}

		writeJSON(w, sess)

	case http.MethodDelete:
		if err := s.store.Delete(id); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, map[string]any{
			"ok": true,
		})

	case http.MethodPut:
		var body struct {
			Title   *string           `json:"title,omitempty"`
			Context *sessions.Context `json:"context,omitempty"`
			Model   *string           `json:"model,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}

		if body.Title != nil {
			if err := s.store.UpdateTitle(id, *body.Title); err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
		}

		if body.Context != nil {
			if err := s.store.UpdateContext(id, *body.Context); err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
		}

		if body.Model != nil {
			if err := s.store.SetModel(id, *body.Model); err != nil {
				writeErr(w, http.StatusInternalServerError, err)
				return
			}
		}

		sess, err := s.store.Get(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}

		writeJSON(w, sess)

	default:
		writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

// redactedConfig returns a copy safe for API responses.
// Secrets are never returned to the browser.
func (s *Server) redactedConfig() config.Config {
	cfg := *s.cfg
	cfg.RemoteAPIKey = ""
	return cfg
}

// mergeConfigPatch applies a JSON object as a partial configuration update.
//
// Fields omitted from the request retain their current values. The existing
// Config pointer is preserved so all runtime components continue sharing the
// same configuration object.
func (s *Server) mergeConfigPatch(data []byte) error {
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(data, &patch); err != nil {
		return err
	}

	currentData, err := json.Marshal(s.cfg)
	if err != nil {
		return err
	}

	var current map[string]json.RawMessage
	if err := json.Unmarshal(currentData, &current); err != nil {
		return err
	}

	for key, value := range patch {
		// A blank remoteApiKey coming from a redacted GET response must never
		// erase the stored secret. A non-empty value can intentionally replace
		// the configured key.
		if key == "remoteApiKey" {
			var candidate string
			if err := json.Unmarshal(value, &candidate); err == nil && candidate == "" {
				continue
			}
		}

		current[key] = value
	}

	merged, err := json.Marshal(current)
	if err != nil {
		return err
	}

	var updated config.Config
	if err := json.Unmarshal(merged, &updated); err != nil {
		return err
	}

	// Preserve the original shared pointer.
	*s.cfg = updated

	return nil
}

// handleConfig deliberately updates the existing Config object in place.
//
// GET never exposes RemoteAPIKey.
//
// PUT/POST behave as patch operations: only fields supplied by the caller are
// changed; unspecified settings remain untouched.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.redactedConfig())

	case http.MethodPut, http.MethodPost:
	var raw json.RawMessage

	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	if err := decoder.Decode(&raw); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	if len(raw) == 0 || string(raw) == "null" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("configuration patch must be a JSON object"))
		return
	}

	if err := s.mergeConfigPatch(raw); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	if err := config.Save(s.cfg.ConfigPath(), s.cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	if err := s.cfg.EnsureDirs(); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, s.redactedConfig())
	default:
		writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]any, 0)

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
		writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}

	var body struct {
		SessionID string `json:"sessionId"`
		Message   string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	if body.SessionID == "" || body.Message == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("sessionId and message required"))
		return
	}

	sess, err := s.store.Get(body.SessionID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}

	// Append user message.
	userMsg := llm.Message{
		Role:    "user",
		Content: body.Message,
	}

	sess.Messages = append(sess.Messages, userMsg)

	// If first user message, derive a title.
	if len(sess.Messages) == 1 {
		title := body.Message
		if len(title) > 60 {
			title = title[:60] + "..."
		}

		_ = s.store.UpdateTitle(sess.ID, title)
	}

	_ = s.store.Save(sess)

	// Build the LLM message list (with optional system prompt).
	var messages []llm.Message

	if sess.Context.SystemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: sess.Context.SystemPrompt,
		})
	}

	// Attach file contents through the chunking engine.
	attachedImages, _ := chunking.SplitAttachments(sess.Context.AttachedFiles)

	if len(sess.Context.AttachedFiles) > 0 {
		note := chunking.ComposeUserMessage(
			"",
			sess.Context.AttachedFiles,
			s.cfg.AttachmentsBudgetBytes(),
		)

		messages = append(messages, llm.Message{
			Role:    "system",
			Content: note,
		})
	}

	messages = append(messages, sess.Messages...)

	// The freshest user message carries the current turn's images.
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			messages[i].Images = append(messages[i].Images, attachedImages...)
			break
		}
	}

	// Spawn the run.
	ctx, cancel := context.WithCancel(context.Background())
	hub := newActivityHub()

	s.runsMu.Lock()

	// There can be only one active run per session. Cancel the previous run
	// before replacing its state.
	if old, ok := s.runs[sess.ID]; ok {
		old.cancel()
		old.hub.close()
	}

	s.runs[sess.ID] = &runState{
		cancel: cancel,
		hub:    hub,
	}

	s.runsMu.Unlock()

	go func() {
		defer func() {
			s.runsMu.Lock()

			// Only remove the run if this goroutine still owns the current
			// session entry. A newer run may already have replaced it.
			if current, ok := s.runs[sess.ID]; ok && current.hub == hub {
				delete(s.runs, sess.ID)
			}

			s.runsMu.Unlock()

			hub.close()
			cancel()
		}()

		res, err := s.orch.RunDetailed(
			ctx,
			messages,
			func(a agent.Activity) {
				hub.publish(a)

				// Persist milestone events only. Streaming response/reasoning
				// deltas are intentionally not persisted individually because
				// each persistence operation rewrites the session JSON.
				switch a.Type {
				case "response", "thinking", "reasoning":
					return
				}

				_ = s.store.AppendActivity(
					sess.ID,
					sessions.ActivityEntry{
						Type:      a.Type,
						Caption:   a.Caption,
						Timestamp: a.Timestamp,
					},
				)
			},
		)

		if err != nil {
			hub.publish(agent.Activity{
				Type:      "error",
				Caption:   err.Error(),
				Timestamp: time.Now(),
			})
			return
		}

		// Append assistant reply.
		if res.Text != "" {
			_, _ = s.store.AppendMessage(
				sess.ID,
				llm.Message{
					Role:      "assistant",
					Content:   res.Text,
					Reasoning: res.Reasoning,
				},
			)
		}

		// Index completed exchange into persistent recall.
		if s.recall != nil && res.Text != "" {
			_ = s.recall.IndexTurn(
				sess.ID,
				sess.Title,
				body.Message,
				res.Text,
				res.ToolsUsed,
			)
		}
	}()

	writeJSON(w, map[string]any{
		"ok":        true,
		"sessionId": sess.ID,
	})
}

func (s *Server) handleAbort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}

	var body struct {
		SessionID string `json:"sessionId"`
	}

	_ = json.NewDecoder(r.Body).Decode(&body)

	s.runsMu.Lock()
	rs, ok := s.runs[body.SessionID]
	s.runsMu.Unlock()

	if ok {
		rs.cancel()
	}

	writeJSON(w, map[string]any{
		"ok": true,
	})
}

// --- WebSocket: live activity ---

func (s *Server) handleActivityWS(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")

	if sessionID == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("sessionId required"))
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
		// No active run — close after sending a sentinel.
		_ = conn.WriteJSON(map[string]any{
			"type":    "idle",
			"caption": "No active run",
		})
		return
	}

	_, updates, unsubscribe := rs.hub.subscribe()
	defer unsubscribe()

	// Read loop: client may send {"action":"abort"} to cancel.
	go func() {
		for {
			var msg map[string]any

			if err := conn.ReadJSON(&msg); err != nil {
				return
			}

			if action, _ := msg["action"].(string); action == "abort" {
				rs.cancel()
			}
		}
	}()

	// Forward every activity published to this specific subscriber.
	for ev := range updates {
		if err := conn.WriteJSON(ev); err != nil {
			return
		}
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	})
}

func parseIntDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}

	return n
}
