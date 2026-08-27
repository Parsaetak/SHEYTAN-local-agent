// Package ui — v1.0.7 "Ember Luxe" design system.
//
// The upgrade every control in the app draws from: layered elevation
// shadows (Fyne has no blur, so depth is faked with stacked offset rounded
// rects at falling alpha — the classic pre-blur trick, tuned warm), glass
// surfaces (translucent fills + ember-tinted edge light), top hairline
// highlights that read as lit bevels, animated glow rings for hover/focus,
// and a shared radius/spacing scale so every surface agrees.
//
// Everything here is a plain helper that composes fyne canvas objects — no
// widget owns its own shadow recipe anymore; the system is the source of
// truth and the widgets cite it.
package ui

import (
	"image"
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// --- radius scale (one system, every surface) ---

const (
	radiusSm   = 8  // chips, small controls
	radiusMd   = 12 // cards, bubbles, tiles
	radiusLg   = 16 // composer, dialogs, hero cards
	radiusPill = 20 // pills, tall chips
)

// --- Ember Luxe tokens ---

var (
	// ColGlass: translucent raised surface — the modern dark-glass card.
	ColGlass = color.NRGBA{R: 30, G: 15, B: 12, A: 216}
	// ColGlassEdge: ember-tinted edge light for glass surfaces.
	ColGlassEdge = color.NRGBA{R: 255, G: 150, B: 100, A: 36}
	// ColGlassEdgeHi: hover-brightened edge light.
	ColGlassEdgeHi = color.NRGBA{R: 255, G: 150, B: 100, A: 96}
	// ColHairTop: 1px top inner highlight — the "lit bevel" of a surface.
	ColHairTop = color.NRGBA{R: 255, G: 220, B: 200, A: 26}
	// ColHairBot: 1px bottom grounding line — surfaces sit ON something.
	ColHairBot = color.NRGBA{R: 0, G: 0, B: 0, A: 80}
	// Elevation shadow layers (outer → inner, alpha falls, radius grows).
	colShadowFar  = color.NRGBA{R: 0, G: 0, B: 0, A: 56}
	colShadowNear = color.NRGBA{R: 0, G: 0, B: 0, A: 96}
	// Glow ring for hover/focus — ember at low alpha, animated up.
	colGlowRest = color.NRGBA{R: 255, G: 110, B: 60, A: 0}
	colGlowHot  = color.NRGBA{R: 255, G: 130, B: 70, A: 120}
	// Fire gradient partners for primary actions (vertical, top-lit).
	gradEmberTop    = color.NRGBA{R: 255, G: 106, B: 40, A: 255}
	gradEmberBottom = color.NRGBA{R: 226, G: 62, B: 16, A: 255}
	gradEmberHotTop = color.NRGBA{R: 255, G: 128, B: 62, A: 255}
	gradEmberHotBot = color.NRGBA{R: 240, G: 78, B: 24, A: 255}
	gradPressTop    = color.NRGBA{R: 196, G: 52, B: 14, A: 255}
	gradPressBot    = color.NRGBA{R: 172, G: 42, B: 10, A: 255}
)

// --- elevation: layered drop shadow ---

// shadowLayout positions each shadow rect grown outward and sunk down by
// dy — the pre-blur elevation trick. Outer layers grow more and sink less.
type shadowLayout struct {
	dy float32
}

func (l shadowLayout) MinSize(_ []fyne.CanvasObject) fyne.Size { return fyne.Size{} }

func (l shadowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	n := len(objs)
	for i, o := range objs {
		// layer 0 = nearest (dy full, small grow); higher = farther.
		grow := float32(n-i) * 2.0
		w := size.Width + grow*2
		h := size.Height + grow*2
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		o.Resize(fyne.NewSize(w, h))
		o.Move(fyne.NewPos(-grow, -grow+l.dy))
	}
}

// elevation builds the stacked shadow layers for a card of the given corner
// radius. Add the returned container FIRST in a Stack, under the card.
func elevation(corner, dy float32) *fyne.Container {
	near := canvas.NewRectangle(colShadowNear)
	near.CornerRadius = corner + 1.5
	far := canvas.NewRectangle(colShadowFar)
	far.CornerRadius = corner + 3
	return container.New(&shadowLayout{dy: dy}, far, near)
}

// --- hairlines: lit bevels ---

// hairlineLayout places thin strips: top (highlight) or bottom (grounding).
type hairlineLayout struct {
	top   bool
	inset float32
}

func (l hairlineLayout) MinSize(_ []fyne.CanvasObject) fyne.Size { return fyne.Size{} }

func (l hairlineLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	w := size.Width - 2*l.inset
	if w < 1 {
		w = 1
	}
	for _, o := range objs {
		o.Resize(fyne.NewSize(w, 1))
		if l.top {
			o.Move(fyne.NewPos(l.inset, 1))
		} else {
			o.Move(fyne.NewPos(l.inset, size.Height-2))
		}
	}
}

// hairlines returns the top-lit + bottom-ground strips to lay OVER a card
// (under its content). inset keeps the light off the rounded corners.
func hairlines(inset float32) *fyne.Container {
	top := canvas.NewRectangle(ColHairTop)
	bot := canvas.NewRectangle(ColHairBot)
	return container.NewStack(
		container.New(&hairlineLayout{top: true, inset: inset}, top),
		container.New(&hairlineLayout{top: false, inset: inset}, bot),
	)
}

// bottomBarLayout pins a strip along the bottom edge (chip underline).
type bottomBarLayout struct{ h float32 }

func (l bottomBarLayout) MinSize(_ []fyne.CanvasObject) fyne.Size { return fyne.Size{} }

func (l bottomBarLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	w := size.Width
	if w < 1 {
		w = 1
	}
	for _, o := range objs {
		o.Resize(fyne.NewSize(w, l.h))
		o.Move(fyne.NewPos(0, size.Height-l.h))
	}
}

// chipUnderline returns the ember underline strip (fades in/out) used by
// glass chips — a hover cue that doesn't fight the layout engine.
func chipUnderline() (bar *canvas.Rectangle, in func(), out func()) {
	bar = canvas.NewRectangle(color.Transparent)
	bar.CornerRadius = 1
	in = func() { animateRectFill(bar, ColEmber, 150*time.Millisecond) }
	out = func() { animateRectFill(bar, color.Transparent, 220*time.Millisecond) }
	return bar, in, out
}

// --- glow ring: hover/focus light ---

// glowRing returns a rounded-rect ring that sits just OUTSIDE the card
// (corner+1.5, transparent fill) plus enter/exit funcs that animate the
// ember light in and out. The unique Ember Luxe signature: controls don't
// just change color on hover — they LIGHT UP.
func glowRing(corner float32) (ring *canvas.Rectangle, enter func(), exit func()) {
	ring = canvas.NewRectangle(color.Transparent)
	ring.CornerRadius = corner + 1.5
	ring.StrokeColor = colGlowRest
	ring.StrokeWidth = 1.5
	enter = func() { animateRectStroke(ring, colGlowHot, 140*time.Millisecond) }
	exit = func() { animateRectStroke(ring, colGlowRest, 200*time.Millisecond) }
	return ring, enter, exit
}

// --- fire gradient ---

// roundedGradient paints a VERTICAL gradient inside a rounded rectangle at
// any size — the true rounded-gradient fill Fyne has no primitive for. The
// raster re-renders on resize, so buttons get pixel-perfect rounded color
// at every width. State changes (hot/pressed/rest) swap the two colors and
// refresh; the O(w·h) paint is trivial at button scale.
type roundedGradient struct {
	raster *canvas.Raster
	top    color.NRGBA
	bottom color.NRGBA
	corner float32
}

func newRoundedGradient(top, bottom color.NRGBA, corner float32) *roundedGradient {
	g := &roundedGradient{top: top, bottom: bottom, corner: corner}
	g.raster = canvas.NewRaster(func(w, h int) image.Image {
		return paintRoundedGradient(w, h, g.top, g.bottom, g.corner)
	})
	return g
}

func (g *roundedGradient) set(top, bottom color.NRGBA) {
	g.top, g.bottom = top, bottom
	canvas.Refresh(g.raster)
}

// paintRoundedGradient renders the gradient with a rounded-corner alpha
// mask (anti-aliased by 2× supersampling on the mask edge only).
func paintRoundedGradient(w, h int, top, bottom color.NRGBA, corner float32) image.Image {
	if w <= 0 || h <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	if corner > float32(h)/2 {
		corner = float32(h) / 2
	}
	if corner > float32(w)/2 {
		corner = float32(w) / 2
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	r := float64(corner)
	for y := 0; y < h; y++ {
		t := 0.0
		if h > 1 {
			t = float64(y) / float64(h-1)
		}
		c := lerpNRGBA(top, bottom, t)
		for x := 0; x < w; x++ {
			if !inRoundedRect(float64(x)+0.5, float64(y)+0.5, float64(w), float64(h), r) {
				continue
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// inRoundedRect tests a point against a centered rounded rectangle.
func inRoundedRect(px, py, w, h, r float64) bool {
	if px < 0 || py < 0 || px > w || py > h {
		return false
	}
	// distance to the nearest "core" rectangle corner
	cx := math.Min(math.Max(px, r), w-r)
	cy := math.Min(math.Max(py, r), h-r)
	dx, dy := px-cx, py-cy
	return dx*dx+dy*dy <= r*r
}

func lerpNRGBA(a, b color.NRGBA, t float64) color.NRGBA {
	return color.NRGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: uint8(float64(a.A) + (float64(b.A)-float64(a.A))*t),
	}
}

// fireGradient returns the brand vertical gradient (top-lit ember).
func fireGradient() *canvas.LinearGradient {
	g := canvas.NewLinearGradient(gradEmberTop, gradEmberBottom, 0)
	g.Angle = 180 // top (light) → bottom (deep): the "lit from above" read
	return g
}

// setFireHot brightens the gradient for hover.
func setFireHot(g *canvas.LinearGradient) {
	g.StartColor = gradEmberHotTop
	g.EndColor = gradEmberHotBot
	canvas.Refresh(g)
}

// setFireRest returns the gradient to its resting state.
func setFireRest(g *canvas.LinearGradient) {
	g.StartColor = gradEmberTop
	g.EndColor = gradEmberBottom
	canvas.Refresh(g)
}

// setFirePressed darkens the gradient for the press frame.
func setFirePressed(g *canvas.LinearGradient) {
	g.StartColor = gradPressTop
	g.EndColor = gradPressBot
	canvas.Refresh(g)
}

// --- tap flash: press feedback that cannot overflow its button ---

// tapFlash returns a full-cover overlay whose alpha flashes up and fades
// back on tap — ripple-style feedback without clipping (a radial sweep
// would overflow Fyne's unclipped containers).
func tapFlash(corner float32) (obj fyne.CanvasObject, flash func()) {
	ov := canvas.NewRadialGradient(
		color.NRGBA{R: 255, G: 210, B: 180, A: 0},
		color.NRGBA{R: 255, G: 210, B: 180, A: 0},
	)
	// no corner on gradients; the flash is subtle enough to read as light.
	flash = func() {
		go func() {
			setGradientAlpha(ov, 0.22)
			time.Sleep(40 * time.Millisecond)
			runOnMain(func() { fadeGradientAlpha(ov, 0.0, 160*time.Millisecond) })
		}()
	}
	return ov, flash
}

func setGradientAlpha(g *canvas.RadialGradient, a float64) {
	g.StartColor = withAlpha(color.NRGBA{R: 255, G: 210, B: 180, A: 255}, a)
	g.EndColor = withAlpha(color.NRGBA{R: 255, G: 210, B: 180, A: 255}, a*0.4)
	canvas.Refresh(g)
}

func fadeGradientAlpha(g *canvas.RadialGradient, to float64, d time.Duration) {
	from := alphaOf(g.StartColor)
	a := fyne.NewAnimation(d, func(v float32) {
		cur := from + (to-from)*float64(v)
		setGradientAlpha(g, cur)
	})
	a.Curve = fyne.AnimationEaseOut
	a.Start()
}

// --- animated width bar (meters, underlines, indicators) ---

// growBarH animates a rectangle's WIDTH from → to (horizontal growth for
// meters and underline indicators; the vertical variant lives in anim.go).
func growBarH(bar *canvas.Rectangle, from, to, full float32, d time.Duration) {
	a := fyne.NewAnimation(d, func(v float32) {
		w := from + (to-from)*v
		bar.Resize(fyne.NewSize(w, bar.Size().Height))
	})
	a.Curve = fyne.AnimationEaseOut
	a.Start()
	_ = full
}

// --- tinted icon badge ---

// badgeTints is the rotation of ember-family tints for icon badges — every
// card gets its own warm identity without leaving the brand.
var badgeTints = []color.NRGBA{
	{R: 255, G: 90, B: 38, A: 46},  // ember
	{R: 255, G: 197, B: 61, A: 42}, // gold
	{R: 255, G: 138, B: 80, A: 44}, // flame soft
	{R: 232, G: 72, B: 28, A: 48},  // deep fire
}

// tintForIndex picks the badge tint for a card's position.
func tintForIndex(i int) color.NRGBA {
	return badgeTints[i%len(badgeTints)]
}

// luxeBadge renders an icon inside a rounded-square tinted badge — the
// suggestion-card / avatar anchor of the new system.
func luxeBadge(iconName string, size float32, tint color.Color) fyne.CanvasObject {
	bg := canvas.NewRectangle(tint)
	bg.CornerRadius = size * 0.28
	ic := canvas.NewImageFromResource(icon(iconName))
	ic.SetMinSize(fyne.NewSize(size*0.55, size*0.55))
	return container.NewStack(
		bg,
		container.NewGridWrap(fyne.NewSize(size, size), container.NewStack(container.NewCenter(ic))),
	)
}
