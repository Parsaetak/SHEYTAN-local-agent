// Package ui — v1.0.9 (TURBINE) frame-paced streaming pump.
//
// The pacer decouples HOW OFTEN tokens arrive from HOW OFTEN the widget
// tree is touched. The orchestrator now forwards stream snapshots at the
// frame target (up to 120/s); the pacer buffers those snapshots and, on a
// frame ticker, coalesces them into AT MOST ONE UI batch per frame:
//
//	tokens ▸─ Pump() ─▸ latest snapshot ─▸ frame ticker ▸ flush (1 batch)
//
// Every flush updates the live labels, the status line (with live tok/s),
// refreshes the activity box once, and scrolls once — the exact work the
// old code did once per snapshot, now capped at the display cadence with
// zero UI work on frames where nothing changed. The result is text that
// streams at the monitor's refresh rate and a widget tree that never
// queues more than one pending refresh, which is what keeps the whole app
// at a smooth 120 fps while tokens pour in.
//
// The ticker only lives while a stream is active: Pump starts it lazily,
// Stop parks it after the turn. Idle frames cost nothing.
package ui

import (
	"sync"
	"time"

	"github.com/sheytan/local-agent/internal/agent"
)

// streamPacer coalesces streaming activity snapshots into per-frame UI
// flushes.
type streamPacer struct {
	mu        sync.Mutex
	resp      string // latest response snapshot (full text so far)
	reason    string // latest reasoning snapshot
	dirty     bool   // new data since the last flush
	started   bool
	stopped   bool
	firstAt   time.Time // first token of the current turn (tok/s base)
	respChars int       // total response chars seen (tok/s denominator)
	stopCh    chan struct{}

	// frame budget telemetry (stress-tested)
	frames   int // flushes that actually touched the UI
	coalesced int // snapshots absorbed into an already-dirty frame

	hz        int
	onFlush   func(resp, reason string, tps float64)
}

// newStreamPacer builds a pacer targeting hz frames per second. onFlush is
// invoked on the UI thread with the latest snapshots (empty strings = no
// change since the last flush for that channel).
func newStreamPacer(hz int, onFlush func(resp, reason string, tps float64)) *streamPacer {
	if hz < 30 {
		hz = 30
	}
	if hz > 240 {
		hz = 240
	}
	return &streamPacer{
		hz:      hz,
		onFlush: onFlush,
		stopCh:  make(chan struct{}),
	}
}

// Pump absorbs one streaming activity. Safe from any goroutine; starts the
// frame ticker on first use. Response/reasoning snapshots replace the
// buffered ones (they are cumulative), so a slow UI always catches up to
// the LATEST text — no queue backlog, ever.
func (p *streamPacer) Pump(a agent.Activity) {
	if p == nil {
		return
	}
	p.mu.Lock()
	switch a.Type {
	case "response":
		p.resp = a.Caption
		if p.firstAt.IsZero() {
			p.firstAt = time.Now()
		}
		p.respChars = len(a.Caption)
		p.dirty = true
	case "reasoning":
		p.reason = a.Caption
		p.dirty = true
	default:
		p.mu.Unlock()
		return
	}
	if p.stopped {
		p.mu.Unlock()
		return
	}
	if !p.started {
		p.started = true
		stop := p.stopCh
		interval := time.Second / time.Duration(p.hz)
		go p.loop(interval, stop)
	} else {
		p.coalesced++
	}
	p.mu.Unlock()
}

// Stop parks the ticker and performs a final flush so no tail text is lost.
func (p *streamPacer) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.started || p.stopped {
		// One final synchronous flush for anything unflushed (e.g. a pump
		// that started and stopped within the same frame).
		resp, reason, tps := p.snapshotLocked()
		p.stopped = true
		p.mu.Unlock()
		if resp != "" || reason != "" {
			p.onFlush(resp, reason, tps)
		}
		return
	}
	resp, reason, tps := p.snapshotLocked()
	p.stopped = true
	p.mu.Unlock()
	close(p.stopCh)
	if resp != "" || reason != "" {
		p.onFlush(resp, reason, tps)
	}
}

// snapshotLocked drains the buffers under the caller's lock hold.
func (p *streamPacer) snapshotLocked() (resp, reason string, tps float64) {
	resp, reason = p.resp, p.reason
	p.resp, p.reason = "", ""
	dirty := p.dirty
	p.dirty = false
	if dirty && !p.firstAt.IsZero() && p.respChars > 0 {
		elapsed := time.Since(p.firstAt).Seconds()
		if elapsed > 0.2 {
			tps = float64(p.respChars) / 4.0 / elapsed // ~4 bytes/token
		}
	}
	return
}

// loop is the frame ticker goroutine. Every tick it drains the buffers and,
// when something changed, performs exactly ONE UI batch. Frames with no new
// data cost nothing (no runOnMain hop, no widget refresh).
func (p *streamPacer) loop(interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			p.mu.Lock()
			if p.stopped {
				p.mu.Unlock()
				return
			}
			resp, reason, tps := p.snapshotLocked()
			p.frames++
			p.mu.Unlock()
			if resp != "" || reason != "" || tps > 0 {
				p.onFlush(resp, reason, tps)
			}
		}
	}
}

// Stats returns the pacer telemetry (frames flushed, updates coalesced).
func (p *streamPacer) Stats() (frames, coalesced int) {
	if p == nil {
		return 0, 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.frames, p.coalesced
}
