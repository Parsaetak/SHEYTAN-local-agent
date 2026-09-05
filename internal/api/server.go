// Package api serves the embedded web UI and exposes the JSON REST+WS API
// that the UI talks to.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/agent"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/attachments"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/chunking"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/installer"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/recall"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/runtime"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sessions"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/sysinfo"
	"github.com/Parsaetak/SHEYTAN-local-agent/web"

	"github.com/gorilla/websocket"
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

	// v1.1.2Z: idle WebSocket standby registry — sessionID → connections.
	// Before this, an activity WebSocket with no active run was closed
	// immediately after one "idle" sentinel, so the UI permanently showed
	// "Offline" between runs. Idle connections now stay open, and the
	// instant a run starts they are woken and attached to the run hub.
	// v1.1.3Z: each standby connection also owns an engineCh fed by the
	// engine event bus so idle UIs still observe engine transitions.
	standbyMu sync.Mutex
	standby   map[string][]*standbyConn

	// engineStop terminates the engine event fan-out on Close.
	engineStop chan struct{}
	engineDone chan struct{}
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

// standbyConn carries the two channels one idle activity connection waits
// on: the run-start wake and pre-encoded engine event frames.
type standbyConn struct {
	wake     chan struct{}
	engineCh chan []byte
}

// enterStandby registers a standby connection for an idle activity
// connection. The wake channel is closed by wakeStandby the moment a run
// starts for the session; engineCh receives pre-encoded engine frames so
// idle UIs still observe engine transitions.
func (s *Server) enterStandby(sessionID string) *standbyConn {
	sc := &standbyConn{
		wake:     make(chan struct{}),
		engineCh: make(chan []byte, 8),
	}

	s.standbyMu.Lock()
	s.standby[sessionID] = append(s.standby[sessionID], sc)
	s.standbyMu.Unlock()

	return sc
}

// leaveStandby removes a standby connection that is no longer waiting (the
// client disconnected, or the connection was already woken).
func (s *Server) leaveStandby(sessionID string, sc *standbyConn) {
	s.standbyMu.Lock()
	defer s.standbyMu.Unlock()

	waiting := s.standby[sessionID]

	kept := waiting[:0]

	for _, w := range waiting {
		if w != sc {
			kept = append(kept, w)
		}
	}

	if len(kept) == 0 {
		delete(s.standby, sessionID)
	} else {
		s.standby[sessionID] = kept
	}
}

// wakeStandby releases every idle connection waiting on a session so they
// can attach to the freshly registered run hub.
func (s *Server) wakeStandby(sessionID string) {
	s.standbyMu.Lock()
	waiting := s.standby[sessionID]
	delete(s.standby, sessionID)
	s.standbyMu.Unlock()

	for _, sc := range waiting {
		close(sc.wake)
	}
}

// New constructs a fully-wired server from the canonical runtime stack.
func New(cfg *config.Config) (*Server, error) {
	store := sessions.New(cfg.SessionsDir)
	stack := runtime.NewStack(cfg)

	s := &Server{
		cfg:        cfg,
		store:      store,
		stack:      stack,
		orch:       stack.Orch,
		llama:      stack.Llama,
		installer:  installer.New(cfg),
		runs:       make(map[string]*runState),
		standby:    make(map[string][]*standbyConn),
		sys:        sysinfo.Probe(),
		recall:     stack.Recall,
		engineStop: make(chan struct{}),
		engineDone: make(chan struct{}),
	}

	// v1.1.3Z: the engine event bus fans authoritative state transitions
	// to every WebSocket as they happen.
	go func() {
		defer close(s.engineDone)
		s.watchEngineEvents(s.engineStop)
	}()

	return s, nil
}

// EnsureSetup runs the installer, creates directories, and prewarms the
// local engine (launch → llama.cpp starts automatically → healthy model).
func (s *Server) EnsureSetup() error {
	if err := s.cfg.EnsureDirs(); err != nil {
		return err
	}

	if _, _, err := s.installer.EnsureRun(false); err != nil {
		return err
	}

	// v1.1.3Z — THE acceptance requirement: the application owns the engine
	// lifecycle. A clean launch must reach a healthy model without any
	// manual llama.cpp intervention.
	if s.cfg.LlamaAutoStart && s.stack != nil {
		s.stack.PrewarmLLM()
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
	mux.HandleFunc("/api/engine", s.handleEngine)
	mux.HandleFunc("/api/attachments", s.handleAttachments)
	mux.HandleFunc("/api/attachments/", s.handleAttachments)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/abort", s.handleAbort)
	mux.HandleFunc("/api/tools", s.handleTools)
	mux.HandleFunc("/api/lab", s.handleLab)
	mux.HandleFunc("/api/lab/", s.handleLabTask)
	mux.HandleFunc("/api/research", s.handleResearch)

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

// modelInfo is the rich local-model descriptor consumed by the UI's model
// pickers. Before v1.1.2Z the API shipped bare filename strings while the
// frontend expected {id, name, path, sizeBytes} objects — every <option>
// rendered with an undefined value, so selecting a model silently wrote
// "undefined" into the config. The wire format now matches the contract.
type modelInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider,omitempty"`
	Path      string `json:"path,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	local := llm.ListLocalModels(s.cfg.ModelsDir)

	loaded, err := s.llama.ListLoadedModels()
	if err != nil {
		loaded = nil
	}

	localInfos := make([]modelInfo, 0, len(local))

	for _, name := range local {
		info := modelInfo{
			ID:       name,
			Name:     strings.TrimSuffix(name, ".gguf"),
			Provider: "local",
			Path:     filepath.Join(s.cfg.ModelsDir, name),
		}

		if fi, statErr := os.Stat(info.Path); statErr == nil {
			info.SizeBytes = fi.Size()
		}

		localInfos = append(localInfos, info)
	}

	loadedInfos := make([]modelInfo, 0, len(loaded))

	for _, id := range loaded {
		loadedInfos = append(loadedInfos, modelInfo{
			ID:       id,
			Name:     id,
			Provider: "local",
		})
	}

	writeJSON(w, map[string]any{
		"local":        localInfos,
		"loaded":       loadedInfos,
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
	if patch == nil {
		return fmt.Errorf("configuration patch must be a JSON object")
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
		writeErr(w, http.StatusMethodNotAllowed, errMethodNotAllowed())
		return
	}

	// Bound request parsing; the run itself is async and bounded elsewhere.
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)

	var body struct {
		SessionID     string   `json:"sessionId"`
		Message       string   `json:"message"`
		AttachmentIDs []string `json:"attachmentIds,omitempty"`
		Regenerate    bool     `json:"regenerate,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	if body.SessionID == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("sessionId required"))
		return
	}

	if body.Message == "" && !body.Regenerate {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("message required (or regenerate=true)"))
		return
	}

	sess, err := s.store.Get(body.SessionID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}

	// Regenerate: drop trailing assistant output so the previous user
	// turn re-runs; the message body is optional in that case.
	if body.Regenerate {
		for len(sess.Messages) > 0 {
			last := sess.Messages[len(sess.Messages)-1]

			if last.Role == "assistant" {
				sess.Messages = sess.Messages[:len(sess.Messages)-1]
				continue
			}

			if last.Role == "tool" {
				sess.Messages = sess.Messages[:len(sess.Messages)-1]
				continue
			}

			break
		}
	}

	// Resolve staged attachments for this turn: explicit ids from the
	// request plus the session's persisted association.
	attIDs := s.attachmentIDsForSession(body.AttachmentIDs)

	for _, id := range s.attachmentIDsForSession(sess.Context.AttachmentIDs) {
		known := false

		for _, knownID := range attIDs {
			if knownID == id {
				known = true
				break
			}
		}

		if !known {
			attIDs = append(attIDs, id)
		}
	}

	// Append the user message (display carries attachment names).
	if body.Message != "" {
		userMsg := llm.Message{
			Role:    "user",
			Content: body.Message,
		}

		if len(attIDs) > 0 && s.stack != nil && s.stack.Attachments != nil {
			for _, id := range attIDs {
				if att, ok := s.stack.Attachments.Get(id); ok {
					userMsg.Attachments = append(
						userMsg.Attachments,
						att.Name,
					)
				}
			}
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
	}

	// Persist attachment association on the session context so later turns
	// keep the files reusable.
	if len(attIDs) > 0 {
		merged := sess.Context.AttachmentIDs

		for _, id := range attIDs {
			known := false

			for _, knownID := range merged {
				if knownID == id {
					known = true
					break
				}
			}

			if !known {
				merged = append(merged, id)
			}
		}

		sess.Context.AttachmentIDs = merged

		for _, id := range attIDs {
			if s.stack != nil && s.stack.Attachments != nil {
				s.stack.Attachments.Associate(id, sess.ID)
			}
		}
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

	// Legacy AttachedFiles (server-side paths) still flow through the
	// chunking engine; the staged attachments pipeline is layered on top.
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

	// The freshest user message carries the current turn's images —
	// both legacy paths and newly staged image attachments.
	stagedImagePaths := make([]string, 0, 4)

	if len(attIDs) > 0 && s.stack != nil && s.stack.Attachments != nil {
		for _, id := range attIDs {
			if att, ok := s.stack.Attachments.Get(id); ok &&
				att.Kind == attachments.KindImage {
				stagedImagePaths = append(
					stagedImagePaths,
					s.stack.Attachments.StagePath(id),
				)
			}
		}
	}

	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			messages[i].Images = append(messages[i].Images, attachedImages...)
			messages[i].Images = append(messages[i].Images, stagedImagePaths...)
			break
		}
	}

	// v1.1.3Z: staged attachments enter the real context pipeline here —
	// relevant chunks are retrieved for the current query and injected
	// as a bounded, provenance-tagged block BEFORE the fresh user turn
	// (stable prompt prefix preserves the engine KV cache). Raw binaries
	// never reach the prompt. The block is content-cached: identical
	// attachments + query skip re-chunking entirely.
	if len(attIDs) > 0 && s.stack != nil && s.stack.Attachments != nil {
		query := body.Message

		if query == "" {
			// Regenerate: retrieve against the last user turn.
			for i := len(sess.Messages) - 1; i >= 0; i-- {
				if sess.Messages[i].Role == "user" {
					query = sess.Messages[i].Content
					break
				}
			}
		}

		budget := s.cfg.AttachmentsBudgetBytes()

		if block := s.stack.Attachments.Retrieve(
			r.Context(),
			query,
			attIDs,
			budget,
		); block != "" {
			blockMsg := llm.Message{
				Role:    "system",
				Content: "[staged attachments relevant to this turn]\n\n" + block,
			}

			// Insert before the last user message.
			insertAt := len(messages)

			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == "user" {
					insertAt = i
					break
				}
			}

			messages = append(
				messages[:insertAt],
				append(
					[]llm.Message{blockMsg},
					messages[insertAt:]...,
				)...,
			)
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

	// v1.1.2Z: release every activity connection parked in standby for
	// this session so they attach to the new run hub immediately.
	s.wakeStandby(sess.ID)

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

		// v1.1.3Z: gate every run on a real, healthy engine. A cold start
		// here streams the authoritative engine transitions to the UI
		// (starting → ready) while the request waits, bounded by the
		// engine gate timeout.
		gateCtx, gateCancel := context.WithTimeout(ctx, 3*time.Minute)

		if err := s.stack.EnsureLLMContext(gateCtx); err != nil {
			gateCancel()

			hub.publish(agent.Activity{
				Type: "error",
				Caption: fmt.Sprintf(
					"Engine unavailable: %v",
					err,
				),
				Timestamp: time.Now(),
			})

			return
		}

		gateCancel()

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

// wsPingInterval is how often an idle standby connection is pinged at the
// protocol level so half-open TCP sockets are detected and released.
const wsPingInterval = 25 * time.Second

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

	// v1.1.2Z: the read pump owns every incoming frame. It detects client
	// disconnects (read error closes clientGone) and routes abort actions
	// to whatever run is currently active for the session.
	clientGone := make(chan struct{})

	go func() {
		defer close(clientGone)

		conn.SetReadLimit(1024)

		for {
			var msg map[string]any

			if err := conn.ReadJSON(&msg); err != nil {
				return
			}

			if action, _ := msg["action"].(string); action == "abort" {
				s.runsMu.Lock()
				rs, ok := s.runs[sessionID]
				s.runsMu.Unlock()

				if ok {
					rs.cancel()
				}
			}
		}
	}()

	idleSentinel := map[string]any{
		"type":    "idle",
		"caption": "No active run",
	}

	for {
		// Fast path: a run is already active — attach to its hub.
		s.runsMu.Lock()
		rs, ok := s.runs[sessionID]
		s.runsMu.Unlock()

		if ok {
			_, updates, unsubscribe := rs.hub.subscribe()

			served := false

			for ev := range updates {
				if err := conn.WriteJSON(ev); err != nil {
					served = true
					break
				}

				served = true
			}

			unsubscribe()

			// Hub closed (run finished) or write failed. On write
			// failure the client is gone; wait for clientGone so
			// the read pump teardown wins the race deterministically.
			if served {
				select {
				case <-clientGone:
					return
				default:
				}
			} else {
				<-clientGone
				return
			}

			// Run finished: fall through to standby.
			_ = conn.WriteJSON(idleSentinel)
		}

		// Standby: park the connection until a run starts, an engine
		// transition arrives, the client leaves, or a keepalive ping
		// is due.
		sc := s.enterStandby(sessionID)

		_ = conn.WriteJSON(idleSentinel)

		disconnected := false

		for !disconnected {
			select {
			case <-sc.wake:
				disconnected = false
				goto attached

			case frame := <-sc.engineCh:
				// Idle connections still observe engine state
				// transitions (authoritative backend state).
				if err := conn.WriteMessage(
					websocket.TextMessage,
					frame,
				); err != nil {
					disconnected = true
				}

			case <-clientGone:
				disconnected = true

			case <-time.After(wsPingInterval):
				_ = conn.WriteControl(
					websocket.PingMessage,
					nil,
					time.Now().Add(5*time.Second),
				)
			}
		}

		s.leaveStandby(sessionID, sc)
		return

	attached:
		s.leaveStandby(sessionID, sc)
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

func errMethodNotAllowed() error {
	return fmt.Errorf("method not allowed")
}

func parseIntDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}

	return n
}
