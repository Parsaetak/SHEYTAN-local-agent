//go:build headless

package ui

// safety_test.go — v1.0.8 crash-proofing verification: the panic guards
// must swallow deliberate panics, and the Aurora widget factories must
// produce live widgets (no panics at construction, correct API surface).

import (
	"testing"
)

// TestSafeTapRecoversPanic proves a panicking callback wrapped by safeTap
// is recovered (and the clean path still runs).
func TestSafeTapRecoversPanic(t *testing.T) {
	ran := false
	wrapped := safeTap("test-boom", func() {
		ran = true
		panic("deliberate test panic")
	})
	wrapped() // must NOT crash the test process
	if !ran {
		t.Fatal("guarded callback never ran")
	}

	n := 0
	clean := safeTap("test-clean", func() { n++ })
	clean()
	if n != 1 {
		t.Fatalf("clean callback ran %d times, want 1", n)
	}
	if safeTap("test-nil", nil) != nil {
		t.Fatal("nil callback should wrap to nil")
	}
}

// TestSafeRecoversPanic guards the non-wrapped variant.
func TestSafeRecoversPanic(t *testing.T) {
	done := false
	safe("test-safe-boom", func() {
		done = true
		panic("boom")
	})
	if !done {
		t.Fatal("safe() body never ran")
	}
}

// TestNoteRecoveredDrainsOnce verifies the status-line note channel
// signals exactly once per recovered panic.
func TestNoteRecoveredDrainsOnce(t *testing.T) {
	// drain any leftovers from previous tests
	for noteRecovered() {
	}
	safeTap("drain-test", func() { panic("note") })()
	if !noteRecovered() {
		t.Fatal("expected one recovered note after panic")
	}
	if noteRecovered() {
		t.Fatal("second drain should be empty")
	}
}

// TestActionButtonConstruction builds both Aurora variants and exercises
// the API surface used by call sites (Disable/Enable/SetDanger).
func TestActionButtonConstruction(t *testing.T) {
	tapped := 0
	pri := newActionButton("Save", "check", true, func() { tapped++ })
	gho := newActionButton("Cancel", "close", false, func() { tapped++ })

	if pri.primary != true || gho.primary != false {
		t.Fatal("variant flags wrong")
	}
	pri.Tapped(nil) // safeTap-wrapped: must run + not crash
	gho.Tapped(nil)
	if tapped != 2 {
		t.Fatalf("tapped = %d, want 2", tapped)
	}

	pri.Disable()
	if pri.enabled {
		t.Fatal("Disable() left button enabled")
	}
	pri.Tapped(nil) // disabled: no tap
	if tapped != 2 {
		t.Fatalf("disabled button fired tap (%d)", tapped)
	}
	pri.Enable()

	gho.SetDanger() // retint must not panic; primary ignores it
	pri.SetDanger()

	// hover transitions must not panic either
	pri.MouseIn(nil)
	pri.MouseOut()
	gho.MouseIn(nil)
	gho.MouseOut()
}

// TestComposerButtonActiveStates drives the composer tile through its
// active/hover lifecycle.
func TestComposerButtonActiveStates(t *testing.T) {
	taps := 0
	c := newComposerButton("attach", func() { taps++ })
	c.Tapped(nil)
	if taps != 1 {
		t.Fatalf("taps = %d", taps)
	}
	c.MouseIn(nil)
	c.SetActive(true)
	c.SetActive(false)
	c.SetIcon("camera")
	c.MouseOut()
	if c.name != "camera" {
		t.Fatalf("SetIcon failed: %q", c.name)
	}
}

// TestWhiteIconCacheStable proves repeated whiteIcon calls return the
// same cached resource.
func TestWhiteIconCacheStable(t *testing.T) {
	a := whiteIcon("send")
	b := whiteIcon("send")
	if a == nil || a != b {
		t.Fatal("whiteIcon cache not stable")
	}
}
