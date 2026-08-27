// Package ui — v1.0.6 feedback (👍/👎) and image thumbnail widgets. The
// feedback row rides under every assistant bubble; the thumbnail strip shows
// attached images inside bubbles with a tap-to-zoom preview.
package ui

import (
	"image/color"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// feedbackRow renders the like/dislike button pair. State: -1 / 0 / +1.
// Tapping the active button clears the verdict (toggle-off); tapping the
// opposite one switches it.
type feedbackRow struct {
	widget.BaseWidget
	obj    fyne.CanvasObject
	up     *canvas.Image
	down   *canvas.Image
	state  int
	onVote func(fb int)
}

func newFeedbackRow(initial int, onVote func(int)) *feedbackRow {
	f := &feedbackRow{state: initial, onVote: onVote}
	f.ExtendBaseWidget(f)
	f.up = canvas.NewImageFromResource(iconMuted("thumbUp"))
	f.up.SetMinSize(fyne.NewSize(16, 16))
	f.down = canvas.NewImageFromResource(iconMuted("thumbDown"))
	f.down.SetMinSize(fyne.NewSize(16, 16))
	f.obj = container.NewHBox(
		container.NewPadded(f.up),
		container.NewPadded(f.down),
	)
	f.applyState()
	return f
}

func (f *feedbackRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(f.obj)
}

func (f *feedbackRow) applyState() {
	switch f.state {
	case 1:
		f.up.Resource = icon("thumbUp")
		f.down.Resource = iconMuted("thumbDown")
	case -1:
		f.up.Resource = iconMuted("thumbUp")
		f.down.Resource = icon("thumbDown")
	default:
		f.up.Resource = iconMuted("thumbUp")
		f.down.Resource = iconMuted("thumbDown")
	}
	f.up.Refresh()
	f.down.Refresh()
}

// SetState updates the verdict without firing the callback.
func (f *feedbackRow) SetState(fb int) {
	f.state = fb
	f.applyState()
}

func (f *feedbackRow) vote(fb int) {
	if f.state == fb {
		fb = 0 // toggle off
	}
	f.state = fb
	f.applyState()
	if f.onVote != nil {
		f.onVote(fb)
	}
}

func (f *feedbackRow) TappedUp()   { f.vote(1) }
func (f *feedbackRow) TappedDown() { f.vote(-1) }

// Tap targets: the icon canvases live inside padded containers, so the whole
// row widget dispatches by x position (left half = up, right half = down).
func (f *feedbackRow) Tapped(ev *fyne.PointEvent) {
	if ev == nil {
		return
	}
	// up button occupies roughly the left half.
	if ev.Position.X < f.obj.MinSize().Width/2 {
		f.vote(1)
	} else {
		f.vote(-1)
	}
}

func (f *feedbackRow) MouseIn(*desktop.MouseEvent)    {}
func (f *feedbackRow) MouseOut()                      {}
func (f *feedbackRow) MouseMoved(*desktop.MouseEvent) {}

// imageThumb is one attached-image thumbnail: the decoded picture in a
// rounded frame; tapping opens a zoomed preview dialog.
type imageThumb struct {
	widget.BaseWidget
	obj   fyne.CanvasObject
	frame *canvas.Rectangle
	img   *canvas.Image
	name  *canvas.Text
	path  string
	onTap func()
}

func newImageThumb(path string, onTap func()) *imageThumb {
	t := &imageThumb{path: path, onTap: onTap}
	t.ExtendBaseWidget(t)
	t.frame = canvas.NewRectangle(ColBgDeep)
	t.frame.CornerRadius = 8
	t.frame.StrokeColor = ColBorderSoft
	t.frame.StrokeWidth = 1
	t.img = canvas.NewImageFromFile(path)
	t.img.FillMode = canvas.ImageFillCover
	t.img.SetMinSize(fyne.NewSize(92, 64))
	t.name = canvas.NewText(clipStrMemory(filepath.Base(path), 18), ColTextMuted)
	t.name.TextSize = 9
	t.name.Alignment = fyne.TextAlignCenter
	t.obj = container.NewVBox(
		container.NewStack(t.frame, t.img),
		t.name,
	)
	return t
}

func (t *imageThumb) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.obj)
}

func (t *imageThumb) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *imageThumb) MouseIn(*desktop.MouseEvent) {
	animateRectStroke(t.frame, color.NRGBA{R: 255, G: 90, B: 38, A: 180}, 120*time.Millisecond)
}
func (t *imageThumb) MouseOut() {
	animateRectStroke(t.frame, ColBorderSoft, 180*time.Millisecond)
}
func (t *imageThumb) MouseMoved(*desktop.MouseEvent) {}
