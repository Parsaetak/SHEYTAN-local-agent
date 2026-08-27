//go:build headless

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/widget"
)

// TestFooterVersionLabelVisible locks in the v1.0.0 audit fix: the footer
// version label must claim its full natural width. Root cause: a Label with
// Truncation=TextTruncateClip reports a collapsed ~one-glyph MinSize, so
// Border layouts stop reserving room and only "SHE" of "SHEYTAN™ v1.0.0"
// rendered (found by pixel audit). Labels that must claim width must NOT set
// Truncation.
func TestFooterVersionLabelVisible(t *testing.T) {
	left := widget.NewLabel("engine: off")
	left.Importance = widget.LowImportance
	update := widget.NewLabel("")
	ver := widget.NewLabel("SHEYTAN v1.0.0")
	ver.TextStyle = fyne.TextStyle{Bold: true}
	// NOTE: deliberately NO Truncation — see the test comment above.

	foot := container.NewBorder(nil, nil,
		container.NewHBox(container.NewPadded(left), container.NewPadded(update)),
		container.NewPadded(ver),
		nil,
	)
	wrap := container.NewVBox(foot)

	c := software.NewCanvas()
	c.SetContent(wrap)
	c.Resize(fyne.NewSize(1340, 60))
	_ = c.Capture()

	vp := ver.Position()
	vs := ver.Size()
	t.Logf("label pos=(%.0f,%.0f) size=%.0fx%.0f", vp.X, vp.Y, vs.Width, vs.Height)
	t.Logf("label min=%v", ver.MinSize())
	if vs.Width < 100 {
		t.Fatalf("version label squeezed to %.0fpx — text would clip", vs.Width)
	}
	if vp.X+vs.Width > 1340 {
		t.Fatalf("version label escapes window: right edge %.0f", vp.X+vs.Width)
	}

	// And the regression proof: the clipped-truncation variant collapses.
	bad := widget.NewLabel("SHEYTAN v1.0.0")
	bad.Truncation = fyne.TextTruncateClip
	if mw := bad.MinSize().Width; mw > 40 {
		t.Logf("note: truncating label min width now %.0f (collapse fixed upstream?)", mw)
	}
}
