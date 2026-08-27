//go:build headless

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func newTestApp() fyne.App {
	a := test.NewApp()
	a.Settings().SetTheme(Theme())
	return a
}

// TestLabelTruncationMinSize checks whether Truncation reduces a Label's
// MinSize (it must, otherwise HBox/Border rows keep their full text width
// and clip at panel edges instead of truncating inside the row).
func TestLabelTruncationMinSize(t *testing.T) {
	a := newTestApp()
	_ = a

	long := "This is a very long caption that goes on and on and should be truncated at the container edge "
	long += long + long

	l1 := widget.NewLabel(long)
	w1 := l1.MinSize().Width

	l2 := widget.NewLabel(long)
	l2.Truncation = fyne.TextTruncateClip
	w2 := l2.MinSize().Width

	t.Logf("plain min width=%.0f  truncated min width=%.0f", w1, w2)
	if w2 >= w1 {
		t.Logf("NOTE: Truncation does NOT reduce MinSize in this Fyne version")
	}
}

// TestOpacityNotCompounding is the v0.9.1 regression test for the animation
// fade bug: setOpacity used to multiply alpha into the CURRENT color on every
// tick, so any pulsing object (typing dots, cross-fade veil) exponentially
// decayed to invisible within seconds of the animation running.
func TestOpacityNotCompounding(t *testing.T) {
	_ = newTestApp()

	c := canvas.NewCircle(color.NRGBA{R: 255, G: 90, B: 38, A: 255})
	for i := 0; i < 200; i++ { // simulate 200 animation ticks
		setOpacity(c, 0.2)
		setOpacity(c, 1.0)
	}
	_, _, _, a := c.FillColor.RGBA()
	if float64(a)/65535 < 0.99 {
		t.Errorf("opacity compounded: after 200 ticks alpha=%.3f, want ~1.0", float64(a)/65535)
	}
	setOpacity(c, 0.2)
	_, _, _, a = c.FillColor.RGBA()
	if got := float64(a) / 65535; got < 0.19 || got > 0.21 {
		t.Errorf("dim step wrong: alpha=%.3f, want 0.20", got)
	}

	txt := canvas.NewText("x", color.NRGBA{R: 255, G: 90, B: 38, A: 255})
	for i := 0; i < 100; i++ {
		setOpacity(txt, 0.5)
	}
	_, _, _, a = txt.Color.RGBA()
	if got := float64(a) / 65535; got < 0.49 || got > 0.51 {
		t.Errorf("text opacity compounded: alpha=%.3f, want 0.50", got)
	}

	rect := canvas.NewRectangle(color.NRGBA{R: 13, G: 7, B: 7, A: 255})
	for i := 0; i < 100; i++ {
		setOpacity(rect, 1.0)
		setOpacity(rect, 0.0)
	}
	setOpacity(rect, 1.0)
	_, _, _, a = rect.FillColor.RGBA()
	if float64(a)/65535 < 0.99 {
		t.Errorf("veil never covers again: alpha=%.3f, want 1.0", float64(a)/65535)
	}
}

// TestStripAbortVisible reproduces the live-agent strip: Border with
// left=icons, right=abort button, center=long caption, squeezed into a
// narrow width — the abort button must remain visible.
func TestStripAbortVisible(t *testing.T) {
	_ = newTestApp()

	caption := widget.NewLabel("Calling tool: dataAnalysis({\"action\":\"stats\",\"path\":\"sales.csv\"}) — a deliberately long caption")
	caption.Truncation = fyne.TextTruncateClip

	flame := widget.NewLabel("🔥")
	abort := widget.NewButtonWithIcon("Abort", nil, func() {})
	abort.Importance = widget.DangerImportance

	strip := container.NewBorder(nil, nil, container.NewHBox(flame), abort, caption)

	c := software.NewCanvas()
	c.SetContent(container.NewPadded(strip))
	c.Resize(fyne.NewSize(480, 200))

	// find the abort button's position
	ab := abort.Position()
	asz := abort.Size()
	cap := caption.Position()
	csz := caption.Size()
	t.Logf("abort at (%.0f,%.0f) size %.0fx%.0f ; caption at (%.0f,%.0f) size %.0fx%.0f",
		ab.X, ab.Y, asz.Width, asz.Height, cap.X, cap.Y, csz.Width, csz.Height)

	if ab.X < cap.X+csz.Width-2 && asz.Width > 0 {
		// overlap check: abort starts before caption ends
		if ab.X < cap.X+csz.Width {
			t.Logf("OVERLAP: caption right edge %.0f > abort left edge %.0f", cap.X+csz.Width, ab.X)
		}
	}
	if asz.Width < 30 {
		t.Errorf("abort button squeezed to %.0fx%.0f — invisible", asz.Width, asz.Height)
	}
	if ab.X+asz.Width > 475 {
		t.Errorf("abort button escapes container: right edge %.0f", ab.X+asz.Width)
	}
}
