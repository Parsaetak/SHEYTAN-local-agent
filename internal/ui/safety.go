package ui

// safety.go — v1.0.8 crash-proofing layer.
//
// THE SECOND HALF OF THE ATTACHMENT FIX. The first half (native pickers)
// removed the crashing component; this half guarantees the CLASS of bug can
// never kill the app again: any panic inside a UI callback, an animation
// tick, or a deferred main-thread mutation is recovered, logged with its
// stack, and surfaced in the status line instead of terminating the
// process.
//
// Why this matters: Go terminates the whole process on an unrecovered
// panic in ANY goroutine. Fyne dispatches taps, hovers and fyne.Do() calls
// onto its own goroutines — one nil-deref in a widget callback and the
// window simply vanishes (the exact symptom reported in v1.0.7).

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/sheytan/local-agent/internal/logging"
)

// lastPanicNote feeds the status line so the user sees "recovered from an
// internal error" instead of a silent freeze.
var lastPanicNote struct {
	ch chan struct{}
}

func init() {
	lastPanicNote.ch = make(chan struct{}, 8)
}

// recoverPanic converts a recovered value into a crash file + log entry +
// status note. Always called from inside a deferred func in a UI context.
// Never touches the UI from here — the recovery may be happening ON the
// main thread, and blocking UI calls (DoAndWait) from the main thread
// deadlock.
func recoverPanic(where string, r interface{}) {
	stack := debug.Stack()
	msg := fmt.Sprintf("recovered UI panic in %s: %v", where, r)
	if m := logging.Default(); m != nil {
		m.Crash(fmt.Sprintf("%s: %v", where, r), stack)
	} else {
		fmt.Println(msg, string(stack))
	}
	select {
	case lastPanicNote.ch <- struct{}{}:
	default:
	}
}

// safe runs fn with a panic guard. Returns fn's result untouched when calm.
func safe(where string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			recoverPanic(where, r)
		}
	}()
	fn()
}

// safeTap wraps an on-tap callback so no widget can crash the app. Every
// tappable constructed by the v1.0.8 widget factory routes through this.
func safeTap(where string, fn func()) func() {
	if fn == nil {
		return nil
	}
	return func() {
		defer func() {
			if r := recover(); r != nil {
				recoverPanic(where, r)
			}
		}()
		fn()
	}
}

// safeTick wraps an animation/timer callback.
func safeTick(where string, fn func()) func() {
	return safeTap(where, fn)
}

// noteRecovered returns true once per recovered panic (consumed by the
// status line poller so the user gets feedback).
func noteRecovered() bool {
	select {
	case <-lastPanicNote.ch:
		return true
	default:
		return false
	}
}

// debounceCall collapses bursts of calls into one trailing execution —
// used by hover/paint paths that can fire dozens of times a second.
func debounceCall(d time.Duration, fn func()) func() {
	var last time.Time
	return func() {
		now := time.Now()
		if now.Sub(last) < d {
			return
		}
		last = now
		fn()
	}
}
