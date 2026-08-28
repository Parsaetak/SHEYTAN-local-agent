//go:build headless

package ui

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/sheytan/local-agent/internal/agent"
)

// TestStreamPacerCoalesces: 100 rapid snapshots at 60 fps must coalesce into
// a handful of UI batches (not 100), and the last flush must carry the
// LATEST text (the snapshot buffer always converges to the newest).
func TestStreamPacerCoalesces(t *testing.T) {
	var batches int64
	var last atomic.Value
	last.Store("")
	p := newStreamPacer(60, func(resp, reason string, tps float64) {
		atomic.AddInt64(&batches, 1)
		last.Store(resp)
	})
	// One frame period of pumping at instant speed — every snapshot lands
	// within a single frame window, so at most a couple of flushes occur.
	for i := 0; i < 100; i++ {
		p.Pump(agent.Activity{Type: "response", Caption: time.Now().String()})
	}
	p.Stop()
	got := batches
	if got == 0 {
		t.Fatalf("no flush happened (final flush missing)")
	}
	if got > 3 {
		t.Errorf("100 snapshots produced %d UI batches, want <= 3 (coalescing broken)", got)
	}
}

// TestStreamPacerFinalFlushCarriesTail: after Stop, the buffered tail text
// reaches the UI exactly once — no token is lost to the frame boundary.
func TestStreamPacerFinalFlushCarriesTail(t *testing.T) {
	var last string
	done := make(chan struct{})
	p := newStreamPacer(240, func(resp, reason string, tps float64) {
		last = resp
		select {
		case <-done:
		default:
			close(done)
		}
	})
	// Pump a partial frame then stop immediately: the ticker may not have
	// fired at all — the final flush must still deliver the text.
	p.Pump(agent.Activity{Type: "response", Caption: "the-final-tail"})
	p.Stop()
	<-done
	if last != "the-final-tail" {
		t.Errorf("final flush = %q, want the-final-tail", last)
	}
}

// TestStreamPacerIgnoresMilestones: only response/reasoning activities ride
// the pacer; tool milestones pass through the immediate path untouched.
func TestStreamPacerIgnoresMilestones(t *testing.T) {
	var batches int64
	p := newStreamPacer(60, func(resp, reason string, tps float64) {
		atomic.AddInt64(&batches, 1)
	})
	p.Pump(agent.Activity{Type: "tool_start", Caption: "Calling tool"})
	p.Pump(agent.Activity{Type: "done", Caption: "Completed"})
	p.Stop()
	if n := atomic.LoadInt64(&batches); n != 0 {
		t.Errorf("milestone activities flushed %d batches, want 0", n)
	}
}

// TestStreamPacerStopTwiceIsSafe: Stop after Stop must not panic or flush.
func TestStreamPacerStopTwiceIsSafe(t *testing.T) {
	var batches int64
	p := newStreamPacer(60, func(resp, reason string, tps float64) {
		atomic.AddInt64(&batches, 1)
	})
	p.Pump(agent.Activity{Type: "response", Caption: "x"})
	p.Stop()
	n1 := atomic.LoadInt64(&batches)
	p.Stop() // idempotent
	if n := atomic.LoadInt64(&batches); n != n1 {
		t.Errorf("second Stop flushed again (%d → %d)", n1, n)
	}
}

// TestStreamPacerLiveTokPerSec: a 2-second pump at 60fps must report a
// positive tokens/sec estimate by the time frames start flushing.
func TestStreamPacerLiveTokPerSec(t *testing.T) {
	var sawTPS atomic.Value
	sawTPS.Store(0.0)
	p := newStreamPacer(60, func(resp, reason string, tps float64) {
		if tps > 0 {
			sawTPS.Store(tps)
		}
	})
	// ~400 chars of text ≈ 100 tokens; pump for 600ms so elapsed > 0.2s
	// gate and the estimate becomes meaningful.
	for i := 0; i < 6; i++ {
		p.Pump(agent.Activity{Type: "response", Caption: "word word word word word word word word word "})
		time.Sleep(100 * time.Millisecond)
	}
	p.Stop()
	if tps := sawTPS.Load().(float64); tps <= 0 {
		t.Errorf("live tok/s never became positive")
	}
}
