// Package ui — smooth animation system for the SHEYTAN GUI.
//
// Everything the user perceives as "alive" lives here:
//   - pulse:     the breathing flame indicator while the agent thinks
//   - typing:    the classic three-dot typing indicator, ember-tinted
//   - emberLine: a thin line that gently glows under the activity strip
//   - crossFade: view-switch transitions (veil that dissolves over new view)
//   - splash:    the boot animation — the flame grows, glows, and burns away
//
// All animations use fyne.Animation with easing curves. Opacity is driven
// through color alpha channels (canvas.Text/Rectangle/Circle) and
// canvas.Image.Translucency; every animation can be started and stopped
// cleanly (no leaks, no paint after hide).
package ui

import (
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// --- opacity plumbing ---

// baseCols remembers each animated object's original color. Opacity must be
// applied ABSOLUTELY (base.alpha × v): applying it to the current color
// compounds multiplicatively on every animation tick, which exponentially
// faded pulsing objects (typing dots, veil, splash text) to black — the
// v0.9.1 "elements disappear/overwrite" ghost.
var baseCols sync.Map // fyne.CanvasObject -> color.Color

func baseColor(obj fyne.CanvasObject, cur color.Color) color.Color {
	if v, ok := baseCols.Load(obj); ok {
		return v.(color.Color)
	}
	baseCols.Store(obj, cur)
	return cur
}

// opacityOf reads the current opacity of a canvas object (0..1; 1 = solid).
func opacityOf(obj fyne.CanvasObject) float64 {
	switch o := obj.(type) {
	case *canvas.Image:
		return 1 - o.Translucency
	case *canvas.Text:
		return alphaOf(o.Color)
	case *canvas.Rectangle:
		return alphaOf(o.FillColor)
	case *canvas.Circle:
		return alphaOf(o.FillColor)
	case *canvas.Line:
		return alphaOf(o.StrokeColor)
	}
	return 1
}

// setOpacity writes the opacity of a canvas object (0..1), derived from the
// object's remembered BASE color so repeated calls never compound.
func setOpacity(obj fyne.CanvasObject, v float64) {
	switch o := obj.(type) {
	case *canvas.Image:
		o.Translucency = clamp01(1 - v)
	case *canvas.Text:
		o.Color = withAlpha(baseColor(obj, o.Color), v)
	case *canvas.Rectangle:
		o.FillColor = withAlpha(baseColor(obj, o.FillColor), v)
	case *canvas.Circle:
		o.FillColor = withAlpha(baseColor(obj, o.FillColor), v)
	case *canvas.Line:
		o.StrokeColor = withAlpha(baseColor(obj, o.StrokeColor), v)
	default:
		return
	}
	canvas.Refresh(obj)
}

// ResetOpacityBase forgets the stored base color (used when a color is
// intentionally replaced).
func ResetOpacityBase(obj fyne.CanvasObject) {
	baseCols.Delete(obj)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func alphaOf(c color.Color) float64 {
	if c == nil {
		return 1
	}
	_, _, _, a := c.RGBA()
	return float64(a) / 65535
}

func withAlpha(c color.Color, opacity float64) color.Color {
	if c == nil {
		return c
	}
	r, g, b, a := c.RGBA()
	na := uint32(clamp01(float64(a)/65535*opacity) * 65535)
	if na > 65535 {
		na = 65535
	}
	return color.NRGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: uint16(na)}
}

// fadeCanvasObject animates an object's opacity from its current value to
// `to` over `d` (one-shot), calling done() after the duration.
func fadeCanvasObject(obj fyne.CanvasObject, to float64, d time.Duration, done func()) {
	start := opacityOf(obj)
	anim := fyne.NewAnimation(d, func(v float32) {
		setOpacity(obj, start+(to-start)*float64(v))
	})
	anim.Curve = fyne.AnimationEaseOut
	if done != nil {
		go func() {
			time.Sleep(d)
			done()
		}()
	}
	anim.Start()
}

// --- pulsing objects ---

// pulse starts a looping opacity pulse (1 → low → 1) and returns a stop func.
func pulse(obj fyne.CanvasObject, period time.Duration, low float64) func() {
	anim := fyne.NewAnimation(period, func(v float32) {
		var t float64
		if v < 0.5 {
			t = float64(v) * 2
		} else {
			t = float64(1-v) * 2
		}
		setOpacity(obj, 1-(1-low)*t)
	})
	anim.RepeatCount = -1 // forever
	anim.Curve = fyne.AnimationEaseInOut
	anim.Start()
	return func() { fyne.CurrentApp().Driver().StopAnimation(anim) }
}

// --- typing dots ---

// typingDots is the animated "…" indicator: three circles that light up in
// sequence like embers.
type typingDots struct {
	obj   fyne.CanvasObject
	dots  []*canvas.Circle
	stops []func()
}

func newTypingDots() *typingDots {
	td := &typingDots{}
	row := container.NewHBox()
	for i := 0; i < 3; i++ {
		c := canvas.NewCircle(color.NRGBA{R: 255, G: 90, B: 38, A: 255})
		// v0.9.1: canvas primitives have MinSize 0 and circles cannot set
		// one — a GridWrap cell pins the dot to exactly 6x6 (and reports
		// that min size upward) so it survives layout passes as a round dot
		// instead of collapsing or stretching.
		row.Add(container.NewPadded(container.NewGridWrap(fyne.NewSize(6, 6), c)))
		td.dots = append(td.dots, c)
	}
	td.obj = row
	return td
}

// start animates the dots with staggered phase offsets.
func (td *typingDots) start() {
	td.stop()
	for i, d := range td.dots {
		dot := d
		// Begin at full brightness so the indicator is instantly visible;
		// the pulse then breathes it down and back up.
		setOpacity(dot, 1)
		anim := fyne.NewAnimation(900*time.Millisecond, func(v float32) {
			var t float64
			if v < 0.5 {
				t = float64(v) * 2
			} else {
				t = float64(1-v) * 2
			}
			setOpacity(dot, 0.2+0.8*t)
		})
		anim.RepeatCount = -1
		anim.Curve = fyne.AnimationEaseInOut
		go func(delay time.Duration, a *fyne.Animation) {
			time.Sleep(delay)
			a.Start()
		}(time.Duration(i)*220*time.Millisecond, anim)
		td.stops = append(td.stops, func() { fyne.CurrentApp().Driver().StopAnimation(anim) })
	}
}

func (td *typingDots) stop() {
	for _, s := range td.stops {
		s()
	}
	td.stops = nil
	for _, d := range td.dots {
		setOpacity(d, 0.2)
	}
}

func (td *typingDots) hide() {
	td.stop()
	td.obj.Hide()
}

func (td *typingDots) show() {
	td.obj.Show()
	td.start()
}

// --- breathing ember line ---

// emberLine returns a thin rectangle whose color breathes between dim and
// bright ember. The returned func stops the animation.
func emberLine(width float32) (line *canvas.Rectangle, stop func()) {
	line = canvas.NewRectangle(color.NRGBA{R: 255, G: 90, B: 38, A: 60})
	line.CornerRadius = 1
	line.SetMinSize(fyne.NewSize(width, 2))
	anim := fyne.NewAnimation(1400*time.Millisecond, func(v float32) {
		var t float64
		if v < 0.5 {
			t = float64(v) * 2
		} else {
			t = float64(1-v) * 2
		}
		alpha := uint8(40 + 170*t)
		line.FillColor = color.NRGBA{R: 255, G: 90, B: 38, A: alpha}
		canvas.Refresh(line)
	})
	anim.RepeatCount = -1
	anim.Curve = fyne.AnimationEaseInOut
	anim.Start()
	return line, func() { fyne.CurrentApp().Driver().StopAnimation(anim) }
}

// --- view cross-fade ---

// crossFader overlays a veil rectangle that can instantly hide the content
// beneath and then dissolve to reveal it — used when switching main views.
type crossFader struct {
	veil *canvas.Rectangle
}

func newCrossFader() *crossFader {
	veil := canvas.NewRectangle(ColBg)
	veil.Hide()
	return &crossFader{veil: veil}
}

// cover flashes the veil to opaque (instantly hiding old content) so the
// caller can swap content beneath.
func (c *crossFader) cover() {
	setOpacity(c.veil, 1)
	c.veil.Show()
	canvas.Refresh(c.veil)
}

// reveal dissolves the veil over `d`, ending hidden.
func (c *crossFader) reveal(d time.Duration) {
	fadeCanvasObject(c.veil, 0, d, func() {
		c.veil.Hide()
		setOpacity(c.veil, 1)
	})
}

// --- v1.0.3 micro-interaction kit ---

// lerpColor linearly interpolates between two colors (t in [0,1]).
func lerpColor(a, b color.Color, t float64) color.Color {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return color.NRGBA64{
		R: uint16(float64(ar) + float64(br-ar)*t),
		G: uint16(float64(ag) + float64(bg-ag)*t),
		B: uint16(float64(ab) + float64(bb-ab)*t),
		A: uint16(float64(aa) + float64(ba-aa)*t),
	}
}

// animateRectFill smoothly transitions a rectangle's fill color over d.
// The animation is tracked on the rect so a rapid enter/leave restarts from
// the CURRENT color instead of jumping (the shared animation registry lives
// in rectAnims).
var rectAnims sync.Map // *canvas.Rectangle -> *fyne.Animation

func animateRectFill(rect *canvas.Rectangle, to color.Color, d time.Duration) {
	if rect == nil {
		return
	}
	if prev, ok := rectAnims.Load(rect); ok {
		if a, ok := prev.(*fyne.Animation); ok {
			fyne.CurrentApp().Driver().StopAnimation(a)
		}
	}
	from := rect.FillColor
	if from == nil {
		from = color.Transparent
	}
	anim := fyne.NewAnimation(d, func(v float32) {
		rect.FillColor = lerpColor(from, to, float64(v))
		canvas.Refresh(rect)
	})
	anim.Curve = fyne.AnimationEaseOut
	rectAnims.Store(rect, &anim)
	anim.Start()
}

// animateRectStroke smoothly transitions a rectangle's stroke color.
var strokeAnims sync.Map // *canvas.Rectangle -> *fyne.Animation

func animateRectStroke(rect *canvas.Rectangle, to color.Color, d time.Duration) {
	if rect == nil {
		return
	}
	if prev, ok := strokeAnims.Load(rect); ok {
		if a, ok := prev.(*fyne.Animation); ok {
			fyne.CurrentApp().Driver().StopAnimation(a)
		}
	}
	from := rect.StrokeColor
	if from == nil {
		from = color.Transparent
	}
	anim := fyne.NewAnimation(d, func(v float32) {
		rect.StrokeColor = lerpColor(from, to, float64(v))
		canvas.Refresh(rect)
	})
	anim.Curve = fyne.AnimationEaseOut
	strokeAnims.Store(rect, &anim)
	anim.Start()
}

// animateCircleStroke smoothly transitions a circle's fill color.
var circleAnims sync.Map // *canvas.Circle -> *fyne.Animation

func animateCircleFill(c *canvas.Circle, to color.Color, d time.Duration) {
	if c == nil {
		return
	}
	if prev, ok := circleAnims.Load(c); ok {
		if a, ok := prev.(*fyne.Animation); ok {
			fyne.CurrentApp().Driver().StopAnimation(a)
		}
	}
	from := c.FillColor
	if from == nil {
		from = color.Transparent
	}
	anim := fyne.NewAnimation(d, func(v float32) {
		c.FillColor = lerpColor(from, to, float64(v))
		canvas.Refresh(c)
	})
	anim.Curve = fyne.AnimationEaseOut
	circleAnims.Store(c, &anim)
	anim.Start()
}

// animateCircleStroke smoothly transitions a circle's stroke color
// (v1.0.7 — glow rings on circular controls).
var circleStrokeAnims sync.Map // *canvas.Circle -> *fyne.Animation

func animateCircleStroke(c *canvas.Circle, to color.Color, d time.Duration) {
	if c == nil {
		return
	}
	if prev, ok := circleStrokeAnims.Load(c); ok {
		if a, ok := prev.(*fyne.Animation); ok {
			fyne.CurrentApp().Driver().StopAnimation(a)
		}
	}
	from := c.StrokeColor
	if from == nil {
		from = color.Transparent
	}
	anim := fyne.NewAnimation(d, func(v float32) {
		c.StrokeColor = lerpColor(from, to, float64(v))
		canvas.Refresh(c)
	})
	anim.Curve = fyne.AnimationEaseOut
	circleStrokeAnims.Store(c, &anim)
	anim.Start()
}

// hoverFx is the reusable hover-chrome driver: it fades a rectangle's fill
// and stroke between the resting and hovered palettes. Widgets embed it and
// forward MouseIn/MouseOut.
type hoverFx struct {
	bg       *canvas.Rectangle
	restFill color.Color
	hotFill  color.Color
	restEdge color.Color
	hotEdge  color.Color
	edgeOn   bool
}

func newHoverFx(bg *canvas.Rectangle, restFill, hotFill, restEdge, hotEdge color.Color, edgeOn bool) *hoverFx {
	return &hoverFx{bg: bg, restFill: restFill, hotFill: hotFill, restEdge: restEdge, hotEdge: hotEdge, edgeOn: edgeOn}
}

func (h *hoverFx) enter() {
	animateRectFill(h.bg, h.hotFill, 120*time.Millisecond)
	if h.edgeOn {
		animateRectStroke(h.bg, h.hotEdge, 120*time.Millisecond)
	}
}

func (h *hoverFx) exit() {
	animateRectFill(h.bg, h.restFill, 180*time.Millisecond)
	if h.edgeOn {
		animateRectStroke(h.bg, h.restEdge, 180*time.Millisecond)
	}
}

// growVertical animates a rectangle's MinSize height from 0 to full — used
// by the sidebar nav accent bar so activation feels physical.
func growVertical(bar *canvas.Rectangle, full float32, d time.Duration) {
	anim := fyne.NewAnimation(d, func(v float32) {
		h := 2 + (full-2)*v
		bar.SetMinSize(fyne.NewSize(3, h))
		canvas.Refresh(bar)
	})
	anim.Curve = fyne.AnimationEaseOut
	anim.Start()
}

// shrinkVertical collapses the accent bar back down.
func shrinkVertical(bar *canvas.Rectangle, d time.Duration) {
	anim := fyne.NewAnimation(d, func(v float32) {
		h := 18 - 16*v
		bar.SetMinSize(fyne.NewSize(3, h))
		canvas.Refresh(bar)
	})
	anim.Curve = fyne.AnimationEaseIn
	anim.Start()
}

// revealIn is the v1.0.3 entrance animation for any widget: a cover rect
// in the ambient background color starts fully opaque ON TOP of the object
// and dissolves away, producing a smooth fade-in that works for widgets
// whose own colors cannot be animated (RichText, Buttons...). The wrapper
// stack stays in the tree (cover hidden) — swapping the child back out
// later raced the layout pass and panicked.
func revealIn(obj fyne.CanvasObject, parent *fyne.Container, over color.Color, delay time.Duration) {
	if obj == nil || parent == nil {
		return
	}
	cover := canvas.NewRectangle(over)
	cover.CornerRadius = 10
	stack := container.NewStack(obj, cover)
	// Replace obj with the stack in the parent at the same position.
	replaced := false
	for i, o := range parent.Objects {
		if o == obj {
			parent.Objects[i] = stack
			replaced = true
			break
		}
	}
	if !replaced {
		return
	}
	parent.Refresh()
	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		fadeCanvasObject(cover, 0, 200*time.Millisecond, func() {
			runOnMain(func() {
				cover.Hide()
				cover.Refresh()
			})
		})
	}()
}

// popPulse runs one quick scale-breathe on a fixed-size disc container
// (the send button press feedback): the disc grows 6% and settles back.
func popPulse(disc *fyne.Container, base fyne.Size) {
	if disc == nil {
		return
	}
	anim := fyne.NewAnimation(220*time.Millisecond, func(v float32) {
		var s float32
		if v < 0.5 {
			s = 1 + 0.06*(v*2)
		} else {
			s = 1.06 - 0.06*((v-0.5)*2)
		}
		w := base.Width * s
		h := base.Height * s
		disc.Resize(fyne.NewSize(w, h))
		disc.Move(fyne.NewPos((base.Width-w)/2, (base.Height-h)/2))
	})
	anim.Curve = fyne.AnimationEaseOut
	anim.Start()
}

// pulseCircle starts a looping glow pulse on a circle (the engine "starting"
// dot) and returns the stop func.
func pulseCircle(c *canvas.Circle, period time.Duration, low float64) func() {
	anim := fyne.NewAnimation(period, func(v float32) {
		var t float64
		if v < 0.5 {
			t = float64(v) * 2
		} else {
			t = float64(1-v) * 2
		}
		setOpacity(c, 1-(1-low)*t)
	})
	anim.RepeatCount = -1
	anim.Curve = fyne.AnimationEaseInOut
	anim.Start()
	return func() {
		fyne.CurrentApp().Driver().StopAnimation(anim)
		setOpacity(c, 1)
	}
}

// --- boot splash ---

// splashLayer builds the boot splash: the SHEYTAN flame grows in, the word
// mark appears, then everything burns away to reveal the app.
// The returned object sits on TOP of the main UI in a Stack.
func splashLayer(onFinished func()) fyne.CanvasObject {
	bg := canvas.NewRectangle(ColBg)

	flame := canvas.NewImageFromResource(Logo)
	flame.FillMode = canvas.ImageFillContain
	flame.Translucency = 1
	flame.SetMinSize(fyne.NewSize(72, 72))

	title := canvas.NewText("SHEYTAN", ColText)
	title.TextSize = 34
	title.Alignment = fyne.TextAlignCenter
	title.TextStyle.Bold = true
	setOpacity(title, 0)

	sub := canvas.NewText("LOCAL AGENT — FORGE DARK", ColTextMuted)
	sub.TextSize = 13
	sub.Alignment = fyne.TextAlignCenter
	setOpacity(sub, 0)

	center := container.NewVBox(
		container.NewPadded(flame),
		container.NewPadded(title),
		container.NewPadded(sub),
	)
	layer := container.NewStack(bg, container.NewCenter(center))

	// Timeline: flame grows 0→1 (700ms), text fades in (400ms at +300ms),
	// hold, then the whole splash dissolves (500ms at +1500ms).
	flameAnim := fyne.NewAnimation(700*time.Millisecond, func(v float32) {
		flame.Translucency = 1 - float64(v)
		s := 72 + 108*float32(v) // 72px → 180px
		flame.SetMinSize(fyne.NewSize(s, s))
		flame.Resize(fyne.NewSize(s, s))
		canvas.Refresh(flame)
	})
	flameAnim.Curve = fyne.AnimationEaseOut

	titleAnim := fyne.NewAnimation(400*time.Millisecond, func(v float32) {
		setOpacity(title, float64(v))
		setOpacity(sub, float64(v))
	})
	titleAnim.Curve = fyne.AnimationEaseOut

	outAnim := fyne.NewAnimation(500*time.Millisecond, func(v float32) {
		t := 1 - float64(v)
		setOpacity(bg, t)
		flame.Translucency = 1 - t
		canvas.Refresh(flame)
		setOpacity(title, t)
		setOpacity(sub, t)
	})
	outAnim.Curve = fyne.AnimationEaseIn

	flameAnim.Start()
	go func() {
		time.Sleep(300 * time.Millisecond)
		titleAnim.Start()
		time.Sleep(1200 * time.Millisecond)
		outAnim.Start()
		time.Sleep(520 * time.Millisecond)
		layer.Hide()
		if onFinished != nil {
			onFinished()
		}
	}()
	return layer
}
