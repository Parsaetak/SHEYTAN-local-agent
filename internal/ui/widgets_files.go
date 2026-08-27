package ui

// widgets_files.go — v1.0.4 artifact widgets: the Files-view list row and
// the in-chat "created file" chip. Both lead with a BIG type icon (the
// user's explicit ask) and expose the four file actions: preview, open
// with the default app, reveal in Explorer, copy path.

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/sheytan/local-agent/internal/artifacts"
	"github.com/sheytan/local-agent/internal/llm"
)

// artifactActions carries the four deeds a row can perform (wired by the
// desktop app so widgets stay decoupled from state).
type artifactActions struct {
	onPreview func()
	onOpen    func()
	onReveal  func()
	onCopy    func()
}

// artifactKindIcon maps an artifact kind to its icon name.
func artifactKindIcon(k artifacts.Kind) string {
	switch k {
	case artifacts.KindChart:
		return "data"
	case artifacts.KindData:
		return "data"
	case artifacts.KindDoc:
		return "doc"
	case artifacts.KindCode:
		return "code"
	case artifacts.KindImage:
		return "image"
	case artifacts.KindArchive:
		return "archive"
	case artifacts.KindDiagnostics:
		return "logs"
	default:
		return "files"
	}
}

// --- tappable icon button ---

// iconTapArea is a borderless, backgroundless icon button: an image that
// responds to taps with a subtle hover glow. Used for the artifact row
// action cluster (eye / open / folder / copy).
type iconTapArea struct {
	widget.BaseWidget
	img   *canvas.Image
	onTap func()
}

func newIconTap(iconName string, onTap func()) *iconTapArea {
	t := &iconTapArea{onTap: onTap}
	t.ExtendBaseWidget(t)
	t.img = canvas.NewImageFromResource(iconMuted(iconName))
	t.img.SetMinSize(fyne.NewSize(18, 18))
	return t
}

func (t *iconTapArea) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewPadded(t.img))
}

func (t *iconTapArea) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

// --- Files-view list row ---

// artifactRow is one file row: big type icon, name, meta line, and the
// action cluster (preview / open / reveal / copy).
type artifactRow struct {
	widget.BaseWidget
	bg      *canvas.Rectangle
	iconRes *canvas.Image
	name    *canvas.Text
	meta    *canvas.Text
	timeTxt *canvas.Text

	previewBtnArea *iconTapArea
	openBtnArea    *iconTapArea
	revealBtnArea  *iconTapArea
	copyBtnArea    *iconTapArea

	actions artifactActions
	hover   *hoverFx
	obj     fyne.CanvasObject
}

func newArtifactRow() *artifactRow {
	r := &artifactRow{}
	r.ExtendBaseWidget(r)
	r.bg = canvas.NewRectangle(ColBgRaised)
	r.bg.CornerRadius = 10
	r.bg.StrokeColor = ColBorderSoft
	r.bg.StrokeWidth = 1

	r.iconRes = canvas.NewImageFromResource(icon("files"))
	r.iconRes.SetMinSize(fyne.NewSize(30, 30))

	r.name = canvas.NewText("", ColText)
	r.name.TextSize = 13
	r.name.TextStyle.Bold = true

	r.meta = canvas.NewText("", ColTextMuted)
	r.meta.TextSize = 11

	r.timeTxt = canvas.NewText("", ColTextMuted)
	r.timeTxt.TextSize = 10
	r.timeTxt.Alignment = fyne.TextAlignTrailing

	// icon-only action buttons (v1.0.4: big-icon-only cluster)
	r.previewBtnArea = newIconTap("eye", func() {
		if r.actions.onPreview != nil {
			r.actions.onPreview()
		}
	})
	r.openBtnArea = newIconTap("open", func() {
		if r.actions.onOpen != nil {
			r.actions.onOpen()
		}
	})
	r.revealBtnArea = newIconTap("folder", func() {
		if r.actions.onReveal != nil {
			r.actions.onReveal()
		}
	})
	r.copyBtnArea = newIconTap("copy", func() {
		if r.actions.onCopy != nil {
			r.actions.onCopy()
		}
	})

	head := container.NewVBox(r.name, r.meta)
	cluster := container.NewHBox(
		container.NewPadded(r.previewBtnArea),
		container.NewPadded(r.openBtnArea),
		container.NewPadded(r.revealBtnArea),
		container.NewPadded(r.copyBtnArea),
	)
	body := container.NewBorder(nil, nil,
		container.NewPadded(container.NewPadded(r.iconRes)),
		container.NewPadded(cluster),
		container.NewPadded(head),
	)
	r.obj = container.NewStack(r.bg, body)
	r.hover = newHoverFx(r.bg, ColBgRaised, color.NRGBA{R: 40, G: 18, B: 13, A: 255},
		ColBorderSoft, color.NRGBA{R: 255, G: 90, B: 38, A: 140}, true)
	return r
}

// Set fills the row for one artifact.
func (r *artifactRow) Set(a artifacts.Artifact) {
	r.iconRes.Resource = icon(artifactKindIcon(a.Kind))
	r.iconRes.Refresh()
	r.name.Text = clipStrMemory(a.Name, 42)
	r.meta.Text = fmt.Sprintf("%s · %s", llm.FormatBytes(a.Size), clipStrMemory(a.Dir, 46))
	r.timeTxt.Text = humanize(a.ModTime)
}

// Tapped opens the action sheet; the icon buttons handle their own deeds.
func (r *artifactRow) Tapped(*fyne.PointEvent) {
	if r.actions.onPreview != nil {
		r.actions.onPreview()
	}
}

func (r *artifactRow) TappedSecondary(*fyne.PointEvent) {
	if r.actions.onReveal != nil {
		r.actions.onReveal()
	}
}

func (r *artifactRow) MouseIn(*desktop.MouseEvent)    { r.hover.enter() }
func (r *artifactRow) MouseOut()                      { r.hover.exit() }
func (r *artifactRow) MouseMoved(*desktop.MouseEvent) {}

// CreateRenderer renders the row stack.
func (r *artifactRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.obj)
}

// --- in-chat chip ---

// artifactChip is the compact "file created" chip under a chat turn:
// a BIG type icon (26px) + name, hover glow, tap → action sheet.
type artifactChip struct {
	widget.BaseWidget
	bg      *canvas.Rectangle
	iconRes *canvas.Image
	name    *canvas.Text
	hover   *hoverFx
	obj     fyne.CanvasObject
	onTap   func()
}

func newArtifactChip(a artifacts.Artifact, onTap func()) *artifactChip {
	c := &artifactChip{onTap: onTap}
	c.ExtendBaseWidget(c)
	c.bg = canvas.NewRectangle(ColBgDeep)
	c.bg.CornerRadius = 10
	c.bg.StrokeColor = color.NRGBA{R: 255, G: 90, B: 38, A: 70} // ember-tinted border: "new"
	c.bg.StrokeWidth = 1

	c.iconRes = canvas.NewImageFromResource(icon(artifactKindIcon(a.Kind)))
	c.iconRes.SetMinSize(fyne.NewSize(24, 24)) // BIG icon — reads at a glance

	c.name = canvas.NewText(clipStrMemory(a.Name, 20), ColText)
	c.name.TextSize = 11

	row := container.NewHBox(
		container.NewPadded(c.iconRes),
		container.NewPadded(c.name),
	)
	c.obj = container.NewStack(c.bg, row)
	c.hover = newHoverFx(c.bg, ColBgDeep, color.NRGBA{R: 44, G: 20, B: 13, A: 255},
		color.NRGBA{R: 255, G: 90, B: 38, A: 70}, color.NRGBA{R: 255, G: 90, B: 38, A: 200}, true)
	return c
}

func (c *artifactChip) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.obj)
}

func (c *artifactChip) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *artifactChip) MouseIn(*desktop.MouseEvent)    { c.hover.enter() }
func (c *artifactChip) MouseOut()                      { c.hover.exit() }
func (c *artifactChip) MouseMoved(*desktop.MouseEvent) {}

// artifactKindLabel renders a one-word type label for chips.
func artifactKindLabel(k artifacts.Kind) string {
	return strings.ToUpper(string(k))
}
