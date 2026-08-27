package ui

// widgets_aurora.go — v1.0.8 "Aurora Luxe" button system.
//
// THE BUTTON REBUILD. v1.0.7 shipped two button species: the gradient
// fireButton (used twice) and — for everything else — the STOCK
// fyne widget.Button behind primaryButton()/ghostButton(). On a 2026 AAA
// scale that stock button reads as a gray rectangle: flat fill, no depth,
// no motion, no brand. This file replaces every one of those ~30 call
// sites with two real components:
//
//   - actionButton(primary): a pill of true painted gradient with layered
//     elevation, an ember glow ring that breathes in on hover, a lit top
//     hairline, and a press that compresses the whole pill a hair darker —
//     the CTA you find on the marketing pages of the top AI platforms.
//
//   - actionButton(ghost): the quiet operator — transparent at rest with a
//     hairline ember edge; hover warms the fill and brightens the border;
//     press dims it back. The z.ai/ChatGPT secondary-action idiom.
//
// Both share one geometry (pill radius, 13px bold label, 16px icon, the
// same padding rhythm) so any dialog holding a pair of them reads as one
// system. Every tap is routed through safeTap so no handler panic can
// close the app (v1.0.8 crash-proofing).

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// --- actionButton: the v1.0.8 primary/secondary button ---

// actionButton is the pill CTA / secondary button used across every dialog
// and toolbar in the app (primaryButton and ghostButton now return this).
type actionButton struct {
	widget.BaseWidget
	obj     fyne.CanvasObject
	primary bool

	// primary chrome
	grad            *roundedGradient
	ring            fyne.CanvasObject
	ringIn, ringOut func()
	flash           func()

	// ghost chrome
	bg     *canvas.Rectangle
	icon   *canvas.Image
	iconNm string
	label  *canvas.Text

	hover   *hoverFx
	onTap   func()
	enabled bool
}

// newActionButton builds the button; primary=true paints the gradient CTA,
// false the quiet ghost pill.
func newActionButton(label, iconName string, primary bool, onTap func()) *actionButton {
	b := &actionButton{primary: primary, onTap: safeTap("actionButton", onTap), enabled: true, iconNm: iconName}
	b.ExtendBaseWidget(b)

	b.icon = canvas.NewImageFromResource(icon(iconName))
	b.icon.SetMinSize(fyne.NewSize(16, 16))
	b.label = canvas.NewText(label, color.NRGBA{R: 255, G: 250, B: 247, A: 255})
	b.label.TextSize = 13
	b.label.TextStyle.Bold = true

	row := container.NewHBox(
		container.NewPadded(b.icon),
		container.NewPadded(b.label),
	)

	if primary {
		// Gradient CTA: elevation under, glow ring around, painted gradient,
		// lit hairline over, content on top, flash on press.
		b.grad = newRoundedGradient(gradEmberTop, gradEmberBottom, radiusPill)
		b.ring, b.ringIn, b.ringOut = glowRing(radiusPill)
		card := container.NewStack(
			b.grad.raster,
			hairlines(10),
			container.NewPadded(container.NewPadded(row)),
		)
		var overlay fyne.CanvasObject
		overlay, b.flash = tapFlash(radiusPill)
		b.obj = container.NewStack(
			elevation(radiusPill, 3),
			b.ring,
			container.NewPadded(card),
			overlay,
		)
	} else {
		// Ghost: transparent pill, hairline ember edge, warm hover.
		b.bg = canvas.NewRectangle(color.Transparent)
		b.bg.CornerRadius = radiusPill
		b.bg.StrokeColor = ColGlassEdge
		b.bg.StrokeWidth = 1
		b.label.Color = ColText
		b.icon.Resource = iconMuted(iconName)
		b.obj = container.NewStack(
			b.bg,
			hairlines(10),
			container.NewPadded(container.NewPadded(row)),
		)
		b.hover = newHoverFx(b.bg, color.Transparent,
			color.NRGBA{R: 255, G: 90, B: 38, A: 26},
			ColGlassEdge, ColGlassEdgeHi, true)
	}
	return b
}

func (b *actionButton) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(b.obj)
}

func (b *actionButton) Tapped(*fyne.PointEvent) {
	if !b.enabled || b.onTap == nil {
		return
	}
	if b.primary && b.grad != nil {
		b.grad.set(gradPressTop, gradPressBot)
		if b.flash != nil {
			b.flash()
		}
		go func() {
			time.Sleep(130 * time.Millisecond)
			runOnMain(func() {
				if b.enabled && b.primary {
					b.grad.set(gradEmberTop, gradEmberBottom)
				}
			})
		}()
	} else if b.bg != nil {
		animateRectFill(b.bg, color.NRGBA{R: 20, G: 9, B: 7, A: 200}, 70*time.Millisecond)
		go func() {
			time.Sleep(120 * time.Millisecond)
			runOnMain(func() {
				if b.enabled {
					animateRectFill(b.bg, color.NRGBA{R: 255, G: 90, B: 38, A: 26}, 140*time.Millisecond)
				}
			})
		}()
	}
	b.onTap()
}

func (b *actionButton) MouseIn(*desktop.MouseEvent) {
	if !b.enabled {
		return
	}
	if b.primary {
		if b.grad != nil {
			b.grad.set(gradEmberHotTop, gradEmberHotBot)
		}
		if b.ringIn != nil {
			b.ringIn()
		}
	} else if b.hover != nil {
		b.hover.enter()
		b.icon.Resource = icon(b.iconNm)
		b.icon.Refresh()
	}
}

func (b *actionButton) MouseOut() {
	if !b.enabled {
		return
	}
	if b.primary {
		if b.grad != nil {
			b.grad.set(gradEmberTop, gradEmberBottom)
		}
		if b.ringOut != nil {
			b.ringOut()
		}
	} else if b.hover != nil {
		b.hover.exit()
		b.icon.Resource = iconMuted(b.iconNm)
		b.icon.Refresh()
	}
}

func (b *actionButton) MouseMoved(*desktop.MouseEvent) {}

// SetEnabled toggles the resting chrome. Disabled primary cools to quiet
// stone; disabled ghost dims its label.
func (b *actionButton) SetEnabled(on bool) {
	b.enabled = on
	if b.primary && b.grad != nil {
		if on {
			b.grad.set(gradEmberTop, gradEmberBottom)
			b.label.Color = color.NRGBA{R: 255, G: 250, B: 247, A: 255}
		} else {
			b.grad.set(
				color.NRGBA{R: 52, G: 26, B: 20, A: 255},
				color.NRGBA{R: 38, G: 18, B: 14, A: 255},
			)
			b.label.Color = color.NRGBA{R: 150, G: 122, B: 112, A: 255}
		}
		b.label.Refresh()
	}
}

// Disable turns the button off (stock-widget API compatibility).
func (b *actionButton) Disable() { b.SetEnabled(false) }

// Enable turns the button on.
func (b *actionButton) Enable() { b.SetEnabled(true) }

// SetDanger retints a GHOST button to the destructive red family (delete
// actions) — the ember hover fill becomes a danger fill, the muted icon a
// red one. Primary buttons ignore this (they stay brand).
func (b *actionButton) SetDanger() {
	if b.primary || b.hover == nil {
		return
	}
	dangerGhost := color.NRGBA{R: 255, G: 59, B: 48, A: 30}
	b.hover = newHoverFx(b.bg, color.Transparent, dangerGhost,
		color.NRGBA{R: 255, G: 90, B: 80, A: 60}, color.NRGBA{R: 255, G: 99, B: 88, A: 120}, true)
	b.label.Color = color.NRGBA{R: 255, G: 138, B: 128, A: 255}
	b.icon.Resource = resource("sheytan-"+b.iconNm+"-danger", iconSVG(iconBodies[b.iconNm], "#FF6B5E"))
	b.label.Refresh()
	b.icon.Refresh()
}

// --- composer icon button (ChatGPT-style control) ---

// composerButton is the quiet rounded-square icon control that lives inside
// the composer pill: invisible chrome at rest, warm fill on hover, molten
// gradient when active (thinking on / tools armed). It replaces the
// circular controlChip of v1.0.7 — the squared 32px tile with a 9px radius
// is the idiom the leading AI chat surfaces use for composer actions.
type composerButton struct {
	widget.BaseWidget
	obj     fyne.CanvasObject
	tile    *canvas.Rectangle
	grad    *roundedGradient
	iconRes *canvas.Image
	name    string
	onTap   func()
	active  bool
	hot     bool
}

func newComposerButton(iconName string, onTap func()) *composerButton {
	c := &composerButton{name: iconName, onTap: safeTap("composerButton:"+iconName, onTap)}
	c.ExtendBaseWidget(c)
	const d = float32(32)

	c.tile = canvas.NewRectangle(color.Transparent)
	c.tile.CornerRadius = 9
	c.tile.StrokeWidth = 0

	c.grad = newRoundedGradient(gradEmberTop, gradEmberBottom, 9)
	c.grad.raster.Hide()

	c.iconRes = canvas.NewImageFromResource(iconMuted(iconName))
	c.iconRes.SetMinSize(fyne.NewSize(16, 16))

	tileWrap := container.NewGridWrap(fyne.NewSize(d, d), c.tile)
	gradWrap := container.NewGridWrap(fyne.NewSize(d, d), c.grad.raster)
	c.obj = container.NewStack(
		tileWrap,
		gradWrap,
		container.NewGridWrap(fyne.NewSize(d, d), container.NewCenter(c.iconRes)),
	)
	return c
}

func (c *composerButton) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.obj)
}

func (c *composerButton) Tapped(*fyne.PointEvent) {
	if c.onTap == nil {
		return
	}
	// press: quick dim of the tile, then back
	animateRectFill(c.tile, color.NRGBA{R: 255, G: 90, B: 38, A: 46}, 40*time.Millisecond)
	go func() {
		time.Sleep(90 * time.Millisecond)
		runOnMain(func() {
			c.rest()
		})
	}()
	c.onTap()
}

func (c *composerButton) rest() {
	if c.active {
		return // gradient owns the surface
	}
	if c.hot {
		animateRectFill(c.tile, color.NRGBA{R: 255, G: 90, B: 38, A: 30}, 120*time.Millisecond)
	} else {
		animateRectFill(c.tile, color.Transparent, 160*time.Millisecond)
	}
}

func (c *composerButton) MouseIn(*desktop.MouseEvent) {
	c.hot = true
	if !c.active {
		animateRectFill(c.tile, color.NRGBA{R: 255, G: 90, B: 38, A: 30}, 130*time.Millisecond)
		c.iconRes.Resource = icon(c.name)
		c.iconRes.Refresh()
	}
}

func (c *composerButton) MouseOut() {
	c.hot = false
	if !c.active {
		animateRectFill(c.tile, color.Transparent, 180*time.Millisecond)
		c.iconRes.Resource = iconMuted(c.name)
		c.iconRes.Refresh()
	}
}

func (c *composerButton) MouseMoved(*desktop.MouseEvent) {}

// SetActive switches the molten state: gradient tile + white glyph.
func (c *composerButton) SetActive(on bool) {
	c.active = on
	if on {
		c.grad.raster.Show()
		animateRectFill(c.tile, color.Transparent, 90*time.Millisecond)
		c.iconRes.Resource = whiteIcon(c.name)
	} else {
		c.grad.raster.Hide()
		if c.hot {
			animateRectFill(c.tile, color.NRGBA{R: 255, G: 90, B: 38, A: 30}, 120*time.Millisecond)
		}
		c.iconRes.Resource = iconMuted(c.name)
	}
	c.iconRes.Refresh()
	canvas.Refresh(c.obj)
}

// SetIcon swaps the glyph (tools chip state changes).
func (c *composerButton) SetIcon(name string) {
	c.name = name
	if c.active {
		c.iconRes.Resource = whiteIcon(name)
	} else {
		c.iconRes.Resource = iconMuted(name)
	}
	c.iconRes.Refresh()
}

// obj32 exposes the 32px footprint for tight toolbars.
func (c *composerButton) obj32() fyne.CanvasObject { return c.obj }
