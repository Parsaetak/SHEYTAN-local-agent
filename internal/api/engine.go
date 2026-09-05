package api

// Engine state surface (v1.1.3Z): the backend engine/process state is the
// single source of truth for the UI. This file exposes it two ways:
//
//   - GET /api/engine — the authoritative snapshot (state, model, detail…)
//   - engine transitions broadcast into every activity WebSocket (both
//     live run hubs and idle standby connections)
//
// The frontend must never invent a state it has not received from here.

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Parsaetak/SHEYTAN-local-agent/internal/agent"
	"github.com/Parsaetak/SHEYTAN-local-agent/internal/llm"
)

// engineSnapshot is the /api/engine payload.
type engineSnapshot struct {
	State      string   `json:"state"`
	Detail     string   `json:"detail,omitempty"`
	Model      string   `json:"model,omitempty"`
	LoadedPath string   `json:"loadedPath,omitempty"`
	Pid        int      `json:"pid,omitempty"`
	Vision     bool     `json:"vision"`
	Provider   string   `json:"provider"`
	Logs       []string `json:"logs,omitempty"`
	CacheStats any      `json:"cacheStats,omitempty"`
	Timestamp  string   `json:"timestamp"`
}

// handleEngine serves the authoritative engine snapshot.
func (s *Server) handleEngine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errMethodNotAllowed())
		return
	}

	writeJSON(w, s.engineSnapshot())
}

func (s *Server) engineSnapshot() engineSnapshot {
	snap := engineSnapshot{
		State:     s.llama.State(),
		Detail:    s.llama.Detail(),
		Pid:       s.llama.Pid(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if s.cfg.IsRemote() {
		snap.Provider = "remote"
		snap.Model = s.cfg.EffectiveModel()

		if snap.State == llm.StateIdle {
			// A remote provider needs no local engine: report a distinct
			// state so the UI does not show a misleading "stopped" badge
			// for remote mode.
			snap.State = "remote"
		}

		return snap
	}

	snap.Provider = "local"
	snap.Model = s.cfg.DisplayModel()
	snap.LoadedPath = s.llama.LoadedModel()
	snap.Vision = s.llama.VisionActive()
	snap.Logs = tailStrings(s.llama.Logs(), 24)

	if s.stack != nil && s.stack.Cache != nil {
		snap.CacheStats = s.stack.Cache.Stats()
	}

	return snap
}

// engineActivity converts one engine transition into an agent.Activity so
// it flows through the normal hub machinery.
func engineActivity(ev llm.EngineEvent) agent.Activity {
	return agent.Activity{
		Type:      "engine",
		Caption:   engineCaption(ev.State),
		Timestamp: ev.Timestamp,
		Detail: map[string]any{
			"state":    ev.State,
			"previous": ev.Previous,
			"model":    ev.Model,
			"detail":   ev.Detail,
		},
	}
}

// watchEngineEvents subscribes to the engine state machine once per
// server and fans transitions out to every live run hub and standby
// connection. It returns when stop closes (server shutdown).
func (s *Server) watchEngineEvents(stop <-chan struct{}) {
	events, unsubscribe := s.llama.SubscribeEvents()
	defer unsubscribe()

	for {
		select {
		case <-stop:
			return

		case ev, ok := <-events:
			if !ok {
				return
			}

			s.broadcastEngineEvent(ev)
		}
	}
}

// broadcastEngineEvent pushes one transition to run hubs and standby
// connections. Standby connections receive the pre-encoded frame on their
// dedicated channel; the standby loop writes it without re-parking.
func (s *Server) broadcastEngineEvent(ev llm.EngineEvent) {
	act := engineActivity(ev)

	s.runsMu.Lock()

	for _, rs := range s.runs {
		if rs != nil && rs.hub != nil {
			rs.hub.publish(act)
		}
	}

	s.runsMu.Unlock()

	frame, err := json.Marshal(map[string]any{
		"type":      act.Type,
		"caption":   act.Caption,
		"state":     ev.State,
		"previous":  ev.Previous,
		"model":     ev.Model,
		"detail":    ev.Detail,
		"timestamp": ev.Timestamp,
	})

	if err != nil {
		return
	}

	s.standbyMu.Lock()

	for _, conns := range s.standby {
		for _, sc := range conns {
			select {
			case sc.engineCh <- frame:
			default:
			}
		}
	}

	s.standbyMu.Unlock()
}

// engineCaption renders the human sentence for one engine state.
func engineCaption(state string) string {
	switch state {
	case llm.StateIdle:
		return "Engine idle"
	case llm.StateDownloading:
		return "Downloading llama.cpp engine…"
	case llm.StateStarting:
		return "Starting llama.cpp and loading the model…"
	case llm.StateReady:
		return "Engine ready — model loaded"
	case llm.StateRunning:
		return "Engine running"
	case llm.StateBusy:
		return "Engine busy — inference in flight"
	case llm.StateStopping:
		return "Engine stopping…"
	case llm.StateStopped:
		return "Engine stopped"
	case llm.StateFailed:
		return "Engine failed — see engine logs"
	default:
		return "Engine: " + state
	}
}

// tailStrings returns the last n strings, preserving order.
func tailStrings(in []string, n int) []string {
	if len(in) <= n {
		return in
	}

	return in[len(in)-n:]
}
