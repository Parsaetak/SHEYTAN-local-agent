// Package ui — v1.0.0 widget library ("Ember Minimal").
//
// The chat.z.ai-inspired components: clickable model chip, circular send
// button, suggestion cards, sidebar nav rows, and the restyled message
// bubbles (user = tinted bubble, assistant = clean full-width text).
// Everything is a REAL fyne.Widget (BaseWidget + SimpleRenderer) so the
// paint walker descends into the tree — opaque wrapper structs render blank
// (a lesson burned in at v0.9).
package ui

import (
        "fmt"
        "image/color"
        "strings"
        "sync"
        "time"

        "fyne.io/fyne/v2"
        "fyne.io/fyne/v2/canvas"
        "fyne.io/fyne/v2/container"
        "fyne.io/fyne/v2/driver/desktop"
        "fyne.io/fyne/v2/widget"

        "github.com/sheytan/local-agent/internal/agent"
)

// --- rounded panel ---

// panel wraps content on a rounded glass card (v1.0.7 Ember Luxe:
// translucent surface + edge light + hairline bevel — every list card in
// the app reads as one material system).
func panel(obj fyne.CanvasObject, corner, pad float32) fyne.CanvasObject {
        bg := canvas.NewRectangle(ColGlass)
        bg.CornerRadius = corner
        bg.StrokeColor = ColGlassEdge
        bg.StrokeWidth = 1
        return container.NewStack(bg, hairlines(10), container.NewPadded(obj))
}

// --- section header (lighter for v1.0.0) ---

// sectionHeader renders icon + bold title (+ optional trailing objects).
func sectionHeader(iconName, title string, trailing ...fyne.CanvasObject) fyne.CanvasObject {
        ic := canvas.NewImageFromResource(icon(iconName))
        ic.SetMinSize(fyne.NewSize(16, 16))
        txt := canvas.NewText(title, ColTextMuted)
        txt.TextSize = 13
        txt.TextStyle.Bold = true
        head := container.NewHBox(container.NewPadded(ic), container.NewPadded(txt))
        if len(trailing) > 0 {
                return container.NewBorder(nil, nil, head, container.NewHBox(trailing...), nil)
        }
        return container.NewPadded(head)
}

// --- status pill ---

// pill renders a small rounded status chip (e.g. "ONLINE").
type pill struct {
        bg   *canvas.Rectangle
        text *canvas.Text
}

func newPill(text string, fg, bg color.Color) *pill {
        p := &pill{}
        p.bg = canvas.NewRectangle(bg)
        p.bg.CornerRadius = 7
        p.text = canvas.NewText(text, fg)
        p.text.TextSize = 11
        p.text.TextStyle.Bold = true
        return p
}

func (p *pill) canvas() fyne.CanvasObject {
        return container.NewStack(p.bg, container.NewPadded(p.text))
}

func (p *pill) SetText(t string) {
        p.text.Text = t
        p.text.Refresh()
        p.bg.Refresh()
}

// SetState updates both the pill label and its colors (used by the
// ONLINE / OFFLINE indicator).
func (p *pill) SetState(t string, fg, bg color.Color) {
        p.text.Text = t
        p.text.Color = fg
        p.bg.FillColor = bg
        p.text.Refresh()
        p.bg.Refresh()
}

// --- chip button (clickable, e.g. the model selector) ---

// chipButton is the v1.0.0 interactive chip: rounded pill with an icon, a
// label, an optional status dot, and a chevron — the whole surface is
// clickable. This is what the model selector in the header is made of; the
// v0.9 model pill was a static canvas the user could click a hundred times
// with nothing happening. v1.0.3: hover chrome (fill + ember edge glow) and
// a Tappable-down press state so the chip reads as a live control.
type chipButton struct {
        widget.BaseWidget
        iconRes   *canvas.Image
        label     *canvas.Text
        dot       *canvas.Circle
        chevron   *canvas.Image
        bg        *canvas.Rectangle
        underline *canvas.Rectangle
        lineIn    func()
        lineOut   func()
        obj       fyne.CanvasObject
        hover     *hoverFx
        onTap     func()
}

func newChipButton(iconName, text string, onTap func()) *chipButton {
        c := &chipButton{onTap: safeTap("chipButton", onTap)}
        c.ExtendBaseWidget(c)
        // v1.0.7 Ember Luxe: translucent glass surface, ember-tinted edge
        // light, hairline bevel, and an ember underline that fades in on
        // hover — the chip reads as lit glass, not flat chrome.
        c.bg = canvas.NewRectangle(ColGlass)
        c.bg.CornerRadius = radiusPill
        c.bg.StrokeColor = ColGlassEdge
        c.bg.StrokeWidth = 1
        c.iconRes = canvas.NewImageFromResource(icon(iconName))
        c.iconRes.SetMinSize(fyne.NewSize(15, 15))
        c.label = canvas.NewText(text, ColText)
        c.label.TextSize = 13
        c.dot = canvas.NewCircle(ColSuccess)
        c.dot.Hide()
        c.chevron = canvas.NewImageFromResource(iconMuted("chevron"))
        c.chevron.SetMinSize(fyne.NewSize(13, 13))
        row := container.NewHBox(
                container.NewPadded(c.iconRes),
                c.label,
        )
        c.underline, c.lineIn, c.lineOut = chipUnderline()
        c.obj = container.NewStack(
                c.bg,
                hairlines(14),
                container.New(&bottomBarLayout{h: 2}, c.underline),
                container.NewPadded(
                        container.NewHBox(row, c.dot, c.chevron)),
        )
        c.hover = newHoverFx(c.bg, ColGlass, color.NRGBA{R: 44, G: 22, B: 17, A: 232}, ColGlassEdge, ColGlassEdgeHi, true)
        return c
}

func (c *chipButton) CreateRenderer() fyne.WidgetRenderer {
        return widget.NewSimpleRenderer(c.obj)
}

func (c *chipButton) Tapped(*fyne.PointEvent) {
        if c.onTap != nil {
                c.onTap()
        }
}

func (c *chipButton) MouseIn(*desktop.MouseEvent) {
        c.hover.enter()
        if c.lineIn != nil {
                c.lineIn()
        }
}
func (c *chipButton) MouseOut() {
        c.hover.exit()
        if c.lineOut != nil {
                c.lineOut()
        }
}
func (c *chipButton) MouseMoved(*desktop.MouseEvent) {}

// Tappable-down press feedback (v1.0.3).
func (c *chipButton) TappedSecondary(*fyne.PointEvent) {}

// SetText updates the chip label.
func (c *chipButton) SetText(t string) {
        c.label.Text = t
        c.label.Refresh()
}

// SetDot shows a status dot in the given color (nil hides it).
func (c *chipButton) SetDot(col color.Color) {
        if col == nil {
                c.dot.Hide()
        } else {
                c.dot.FillColor = col
                c.dot.Show()
        }
        c.dot.Refresh()
}

// --- circular send button (v1.0.7 Ember Luxe) ---

// sendButton is the round gradient send control in the composer. v1.0.7:
// a true gradient disc (raster-painted, lit from above), a soft drop
// shadow, an ember glow ring that lights on hover, and a press flash —
// the single most-tapped control in the app now carries the most craft.
// Disabled state rests into quiet chrome.
type sendButton struct {
        widget.BaseWidget
        disc    *fyne.Container
        pulse   *fyne.Container
        grad    *roundedGradient
        ring    *canvas.Circle
        shadow  *canvas.Circle
        obj     fyne.CanvasObject
        tapFn   func()
        onTap   func()
        enabled bool
}

func newSendButton(onTap func()) *sendButton {
        s := &sendButton{tapFn: onTap, enabled: true}
        s.onTap = safeTap("sendButton", onTap)
        s.ExtendBaseWidget(s)
        // v1.0.8: 34px disc — the compact modern proportion (ChatGPT/Claude
        // school); the up-arrow glyph centers optically with room to breathe.
        const d = float32(34)

        // gradient disc — full-round raster (corner = d/2)
        s.grad = newRoundedGradient(gradEmberTop, gradEmberBottom, d/2)
        discBg := container.NewGridWrap(fyne.NewSize(d, d), s.grad.raster)

        // glow ring: just outside the disc
        s.ring = canvas.NewCircle(color.Transparent)
        s.ring.StrokeColor = colGlowRest
        s.ring.StrokeWidth = 1.5
        ringWrap := container.NewGridWrap(fyne.NewSize(d+5, d+5), s.ring)

        // soft drop shadow: darker disc sunk 2px
        s.shadow = canvas.NewCircle(color.NRGBA{R: 0, G: 0, B: 0, A: 110})
        shadowWrap := container.NewGridWrap(fyne.NewSize(d, d), s.shadow)

        arrow := canvas.NewImageFromResource(whiteIcon("send"))
        arrow.SetMinSize(fyne.NewSize(18, 18))

        s.disc = container.NewStack(shadowWrap, ringWrap, discBg, container.NewCenter(arrow))
        s.pulse = container.NewGridWrap(fyne.NewSize(d+6, d+6), s.disc)
        s.obj = s.pulse
        return s
}

func (s *sendButton) CreateRenderer() fyne.WidgetRenderer {
        return widget.NewSimpleRenderer(s.obj)
}

func (s *sendButton) Tapped(*fyne.PointEvent) {
        if s.onTap == nil {
                return
        }
        popPulse(s.pulse, s.pulse.MinSize()) // press pop
        s.grad.set(gradPressTop, gradPressBot)
        animateCircleStroke(s.ring, color.NRGBA{R: 255, G: 150, B: 90, A: 200}, 90*time.Millisecond)
        go func() {
                time.Sleep(120 * time.Millisecond)
                runOnMain(func() {
                        if s.enabled {
                                s.grad.set(gradEmberTop, gradEmberBottom)
                        }
                        animateCircleStroke(s.ring, colGlowRest, 260*time.Millisecond)
                })
        }()
        s.onTap()
}

func (s *sendButton) MouseIn(*desktop.MouseEvent) {
        if s.enabled {
                s.grad.set(gradEmberHotTop, gradEmberHotBot)
                animateCircleStroke(s.ring, colGlowHot, 140*time.Millisecond)
        }
}

func (s *sendButton) MouseOut() {
        if s.enabled {
                s.grad.set(gradEmberTop, gradEmberBottom)
                animateCircleStroke(s.ring, colGlowRest, 200*time.Millisecond)
        }
}

func (s *sendButton) MouseMoved(*desktop.MouseEvent) {}

// SetEnabled toggles the ember fill and tap routing.
func (s *sendButton) SetEnabled(on bool) {
        s.enabled = on
        if on {
                s.grad.set(gradEmberTop, gradEmberBottom)
                s.onTap = safeTap("sendButton", s.tapFn)
        } else {
                s.grad.set(
                        color.NRGBA{R: 70, G: 36, B: 28, A: 255},
                        color.NRGBA{R: 52, G: 25, B: 19, A: 255},
                )
                s.onTap = nil
        }
}

// --- suggestion card (empty state) ---

// suggestionCard is one of the four first-run action cards: tinted icon
// badge, title, one-line description. Tapping fills the composer. v1.0.7
// Ember Luxe: glass surface + hairline bevel + elevation shadow, and each
// card carries its own warm badge tint from the brand rotation — the hero
// reads as a set of collectible tiles, not a form.
type suggestionCard struct {
        widget.BaseWidget
        bg      *canvas.Rectangle
        badge   fyne.CanvasObject
        title   *canvas.Text
        desc    *canvas.Text
        obj     fyne.CanvasObject
        ringIn  func()
        ringOut func()
        hover   *hoverFx
        onTap   func()
}

// suggestionCardIndex assigns each created card the next badge tint in the
// rotation (per-build order = display order in the hero grid).
var suggestionCardIndex int

func newSuggestionCard(iconName, title, desc string, onTap func()) *suggestionCard {
        s := &suggestionCard{onTap: safeTap("suggestionCard", onTap)}
        s.ExtendBaseWidget(s)
        tint := tintForIndex(suggestionCardIndex)
        suggestionCardIndex++

        s.bg = canvas.NewRectangle(ColGlass)
        s.bg.CornerRadius = radiusMd
        s.bg.StrokeColor = ColGlassEdge
        s.bg.StrokeWidth = 1

        s.badge = luxeBadge(iconName, 34, tint)
        s.title = canvas.NewText(title, ColText)
        s.title.TextSize = 14
        s.title.TextStyle.Bold = true
        s.desc = canvas.NewText(desc, ColTextMuted)
        s.desc.TextSize = 12
        body := container.NewVBox(
                container.NewPadded(s.badge),
                container.NewPadded(s.title),
                container.NewPadded(s.desc),
        )
        ring, ringIn, ringOut := glowRing(radiusMd)
        s.ringIn, s.ringOut = ringIn, ringOut
        s.obj = container.NewStack(
                elevation(radiusMd, 2),
                ring,
                s.bg,
                hairlines(12),
                container.NewPadded(body),
        )
        s.hover = newHoverFx(s.bg, ColGlass, color.NRGBA{R: 46, G: 22, B: 16, A: 238}, ColGlassEdge, ColGlassEdgeHi, true)
        return s
}

func (s *suggestionCard) CreateRenderer() fyne.WidgetRenderer {
        return widget.NewSimpleRenderer(s.obj)
}

func (s *suggestionCard) Tapped(*fyne.PointEvent) {
        if s.onTap != nil {
                s.onTap()
        }
}

func (s *suggestionCard) MouseIn(*desktop.MouseEvent) {
        s.hover.enter()
        if s.ringIn != nil {
                s.ringIn()
        }
}
func (s *suggestionCard) MouseOut() {
        s.hover.exit()
        if s.ringOut != nil {
                s.ringOut()
        }
}
func (s *suggestionCard) MouseMoved(*desktop.MouseEvent) {}

// --- sidebar nav row ---

// navRow is one navigation entry in the sidebar: icon + label, ember accent
// bar and bright icon when active. (v0.9's icon-only rail became labeled
// rows — minimal does not mean cryptic.) v1.0.3 adds a hover chrome and an
// animated accent bar so navigation feels alive under the cursor.
type navRow struct {
        widget.BaseWidget
        name    string
        iconRes *canvas.Image
        label   *canvas.Text
        bar     *canvas.Rectangle
        bg      *canvas.Rectangle
        obj     fyne.CanvasObject
        active  bool
        hover   *hoverFx
        onTap   func()
}

func newNavRow(iconName, label string, onTap func()) *navRow {
        n := &navRow{name: iconName, onTap: safeTap("navRow", onTap)}
        n.ExtendBaseWidget(n)
        n.bg = canvas.NewRectangle(color.Transparent)
        n.bg.CornerRadius = 8
        n.iconRes = canvas.NewImageFromResource(iconMuted(iconName))
        n.iconRes.SetMinSize(fyne.NewSize(16, 16))
        n.label = canvas.NewText(label, ColTextMuted)
        n.label.TextSize = 13
        n.bar = canvas.NewRectangle(ColEmber)
        n.bar.CornerRadius = 1.5
        n.bar.SetMinSize(fyne.NewSize(3, 18))
        n.bar.Hide()
        row := container.NewHBox(
                container.NewPadded(n.bar),
                container.NewPadded(n.iconRes),
                container.NewPadded(n.label),
        )
        n.obj = container.NewStack(n.bg, container.NewPadded(row))
        n.hover = newHoverFx(n.bg, color.Transparent, color.NRGBA{R: 36, G: 15, B: 11, A: 255}, color.Transparent, color.Transparent, false)
        return n
}

func (n *navRow) CreateRenderer() fyne.WidgetRenderer {
        return widget.NewSimpleRenderer(n.obj)
}

func (n *navRow) Tapped(*fyne.PointEvent) {
        if n.onTap != nil {
                n.onTap()
        }
}

// MouseIn/MouseOut/MouseMoved implement fyne.Hoverable (v1.0.3 hover chrome).
func (n *navRow) MouseIn(*desktop.MouseEvent)    { n.hover.enter() }
func (n *navRow) MouseOut()                      { n.hover.exit() }
func (n *navRow) MouseMoved(*desktop.MouseEvent) {}

func (n *navRow) SetActive(active bool) {
        if n.active == active {
                return
        }
        n.active = active
        if active {
                n.iconRes.Resource = icon(n.name)
                n.label.Color = ColText
                n.label.TextStyle.Bold = true
                n.bar.Show()
                // v1.0.3: the accent bar grows in — activation feels physical.
                growVertical(n.bar, 18, 180*time.Millisecond)
                animateRectFill(n.bg, color.NRGBA{R: 42, G: 18, B: 14, A: 255}, 160*time.Millisecond)
        } else {
                n.iconRes.Resource = iconMuted(n.name)
                n.label.Color = ColTextMuted
                n.label.TextStyle.Bold = false
                shrinkVertical(n.bar, 140*time.Millisecond)
                animateRectFill(n.bg, color.Transparent, 160*time.Millisecond)
                go func() {
                        time.Sleep(160 * time.Millisecond)
                        runOnMain(func() {
                                if !n.active {
                                        n.bar.Hide()
                                        n.bar.Refresh()
                                }
                        })
                }()
        }
        n.iconRes.Refresh()
        n.label.Refresh()
        n.bar.Refresh()
}

// --- model picker row (v1.0.0) ---

// modelRow is one selectable model in the picker: model icon, file name,
// size hint, and a check mark on the active model. Tapping it applies the
// model immediately (the dialog closes and the engine reloads). v1.0.3:
// hover chrome on the row.
type modelRow struct {
        widget.BaseWidget
        bg      *canvas.Rectangle
        iconRes *canvas.Image
        name    *canvas.Text
        meta    *canvas.Text
        check   *canvas.Image
        obj     fyne.CanvasObject
        hover   *hoverFx
        onTap   func()
}

func newModelRow(name, meta string, current bool, onTap func()) *modelRow {
        m := &modelRow{onTap: safeTap("modelRow", onTap)}
        m.ExtendBaseWidget(m)
        m.bg = canvas.NewRectangle(color.Transparent)
        m.bg.CornerRadius = 8
        m.bg.StrokeWidth = 0
        m.iconRes = canvas.NewImageFromResource(icon("model"))
        m.iconRes.SetMinSize(fyne.NewSize(16, 16))
        m.name = canvas.NewText(name, ColText)
        m.name.TextSize = 13
        m.meta = canvas.NewText(meta, ColTextMuted)
        m.meta.TextSize = 11
        m.check = canvas.NewImageFromResource(icon("check"))
        m.check.SetMinSize(fyne.NewSize(14, 14))
        if !current {
                m.check.Hide()
        }
        texts := container.NewVBox(m.name, m.meta)
        row := container.NewHBox(
                container.NewPadded(m.iconRes),
                container.NewPadded(texts),
        )
        m.obj = container.NewStack(m.bg, container.NewBorder(nil, nil,
                row, container.NewPadded(m.check), nil))
        m.hover = newHoverFx(m.bg, color.Transparent, ColHover, color.Transparent, color.Transparent, false)
        return m
}

func (m *modelRow) CreateRenderer() fyne.WidgetRenderer {
        return widget.NewSimpleRenderer(m.obj)
}

func (m *modelRow) Tapped(*fyne.PointEvent) {
        if m.onTap != nil {
                m.onTap()
        }
}

func (m *modelRow) MouseIn(*desktop.MouseEvent)    { m.hover.enter() }
func (m *modelRow) MouseOut()                      { m.hover.exit() }
func (m *modelRow) MouseMoved(*desktop.MouseEvent) {}

// --- chat message bubble (v1.0.0 restyle, v1.0.2 extended) ---

// messageBubble is one chat message. chat.z.ai pattern: the user speaks in a
// compact tinted bubble (the parent right-aligns it); the assistant answers
// as clean full-width text under a small ember role marker — no heavy card.
// v1.0.2: optional collapsible "Thought process" section (thinking mode)
// and attachment chips (files added to the message).
type messageBubble struct {
        widget.BaseWidget
        obj       fyne.CanvasObject
        bg        *canvas.Rectangle
        bar       *canvas.Rectangle
        roleIco   *canvas.Image
        roleBadge fyne.CanvasObject
        roleTxt   *canvas.Text
        body      *widget.RichText
        role      string

        // v1.0.2 optional sections
        think     *widget.Accordion
        thinkBody *widget.Label
        chips     *fyne.Container // attachment chips row

        // v1.0.6: image thumbnails + timestamp + feedback row
        thumbs   *fyne.Container
        timeTxt  *canvas.Text
        feedback *feedbackRow
        meta     *fyne.Container // time + feedback (assistant) / time (user)
}

func (m *messageBubble) CreateRenderer() fyne.WidgetRenderer {
        return widget.NewSimpleRenderer(m.obj)
}

func newMessageBubble() *messageBubble {
        m := &messageBubble{}
        m.ExtendBaseWidget(m)
        m.bg = canvas.NewRectangle(color.NRGBA{R: 34, G: 17, B: 13, A: 255}) // user tint
        m.bg.CornerRadius = 12
        m.bar = canvas.NewRectangle(ColEmber)
        m.bar.CornerRadius = 1.5
        m.bar.SetMinSize(fyne.NewSize(3, 10))
        m.roleIco = canvas.NewImageFromResource(icon("agent"))
        m.roleIco.SetMinSize(fyne.NewSize(13, 13))
        m.roleTxt = canvas.NewText("SHEYTAN", ColEmber)
        m.roleTxt.TextSize = 10
        m.roleTxt.TextStyle.Bold = true
        m.body = widget.NewRichTextWithText("")
        m.body.Wrapping = fyne.TextWrapWord

        // v1.0.2: collapsible reasoning (hidden until SetFull provides it)
        m.thinkBody = widget.NewLabel("")
        m.thinkBody.Wrapping = fyne.TextWrapWord
        m.think = widget.NewAccordion(widget.NewAccordionItem("Thought process", m.thinkBody))
        m.think.CloseAll()
        m.think.Hide()

        // v1.0.2: attachment chips (hidden until SetFull provides them)
        m.chips = container.NewHBox()
        m.chips.Hide()

        // v1.0.6: image thumbnails + meta row (timestamp, feedback)
        m.thumbs = container.NewHBox()
        m.thumbs.Hide()
        m.timeTxt = canvas.NewText("", ColTextMuted)
        m.timeTxt.TextSize = 9
        m.feedback = newFeedbackRow(0, nil)
        m.meta = container.NewHBox(m.timeTxt)
        m.meta.Hide()

        roleRow := container.NewHBox(container.NewPadded(m.roleIco), container.NewPadded(m.roleTxt))
        _ = roleRow
        // v1.0.7: the assistant speaks under a small flame badge — the brand
        // avatar; the user speaks in a glass card with an ember left accent.
        m.roleBadge = luxeBadge("agent", 18, color.NRGBA{R: 255, G: 90, B: 38, A: 46})
        badgedRow := container.NewHBox(container.NewPadded(m.roleBadge), container.NewPadded(m.roleTxt))
        // user layout: card only. assistant layout: role marker + clean text.
        inner := container.NewVBox(badgedRow, m.think, m.body, m.chips, m.thumbs, m.meta)
        m.obj = container.NewStack(m.bg, hairlines(12), container.NewPadded(container.NewPadded(inner)))
        return m
}

// Set updates the bubble for a message. role: "user" | "assistant".
func (m *messageBubble) Set(role, content string) {
        m.SetFull(role, content, "", nil)
}

// SetFull updates the bubble including the v1.0.2 reasoning trace and
// attachment names.
func (m *messageBubble) SetFull(role, content, reasoning string, attachments []string) {
        m.SetMessage(bubbleInfo{Role: role, Content: content, Reasoning: reasoning, Attachments: attachments})
}

// bubbleInfo carries everything a bubble can render (v1.0.6).
type bubbleInfo struct {
        Role        string
        Content     string
        Reasoning   string
        Attachments []string
        Images      []string          // image file paths → thumbnails
        Feedback    int               // -1 / 0 / +1
        At          time.Time         // timestamp (zero = hide)
        OnFeedback  func(int)         // nil = hide feedback buttons
        OnZoom      func(path string) // nil = thumbnails not tappable
}

// SetMessage renders the full v1.0.6 bubble: text, reasoning, attachment
// chips, image thumbnails, timestamp and the feedback row.
func (m *messageBubble) SetMessage(info bubbleInfo) {
        role, content, reasoning, attachments := info.Role, info.Content, info.Reasoning, info.Attachments
        m.role = role
        if role == "user" {
                m.roleIco.Resource = icon("user")
                m.roleTxt.Text = "YOU"
                m.roleTxt.Color = ColTextMuted
                m.bar.FillColor = color.NRGBA{R: 122, G: 86, B: 72, A: 255}
                m.bg.FillColor = ColGlass
                m.bg.CornerRadius = radiusMd
                m.bg.StrokeColor = ColGlassEdge
                m.bg.StrokeWidth = 1
                m.bg.Show()
                m.roleIco.Hide()
                m.roleBadge.Hide()
        } else {
                m.roleIco.Resource = icon("agent")
                m.roleTxt.Text = "SHEYTAN"
                m.roleTxt.Color = ColEmber
                m.bar.FillColor = ColEmber
                m.bg.FillColor = color.Transparent
                m.bg.Hide()
                m.roleIco.Show()
                m.roleBadge.Show()
        }
        if role == "assistant" {
                m.body.ParseMarkdown(safeMarkdown(content))
        } else {
                m.body.ParseMarkdown(strings.ReplaceAll(escapeMD(content), "\n", "  \n"))
        }
        // v1.0.2: reasoning accordion (assistant only)
        if role == "assistant" && reasoning != "" {
                m.thinkBody.SetText(reasoning)
                m.think.Items[0].Title = fmt.Sprintf("Thought process (%d chars)", len(reasoning))
                m.think.Show()
                m.think.Refresh()
        } else {
                m.think.Hide()
        }
        // v1.0.2: attachment chips
        m.chips.RemoveAll()
        if len(attachments) > 0 {
                for _, a := range attachments {
                        m.chips.Add(newAttachChip(a, false, nil))
                }
                m.chips.Show()
        } else {
                m.chips.Hide()
        }
        // v1.0.6: image thumbnails
        m.thumbs.RemoveAll()
        if len(info.Images) > 0 {
                for _, p := range info.Images {
                        path := p
                        zoom := info.OnZoom
                        m.thumbs.Add(newImageThumb(path, func() {
                                if zoom != nil {
                                        zoom(path)
                                }
                        }))
                }
                m.thumbs.Show()
        } else {
                m.thumbs.Hide()
        }
        // v1.0.6: meta row — timestamp always, feedback buttons on assistant
        // messages that have a vote callback.
        if !info.At.IsZero() {
                m.timeTxt.Text = info.At.Format("15:04")
        }
        m.meta.RemoveAll()
        if !info.At.IsZero() {
                m.meta.Add(container.NewPadded(m.timeTxt))
        }
        if role == "assistant" && info.OnFeedback != nil {
                m.feedback.SetState(info.Feedback)
                m.feedback.onVote = info.OnFeedback
                // a hair of separation between timestamp and the vote buttons
                m.meta.Add(container.NewPadded(canvas.NewText("·", ColTextMuted)))
                m.meta.Add(m.feedback)
        }
        if len(m.meta.Objects) > 0 {
                m.meta.Show()
        } else {
                m.meta.Hide()
        }
        m.body.Refresh()
        m.roleIco.Refresh()
        m.roleTxt.Refresh()
        m.bar.Refresh()
        m.bg.Refresh()
        m.chips.Refresh()
        m.thumbs.Refresh()
        m.meta.Refresh()
}

// --- attach tile (v1.0.3 file attachments, icon-first redesign) ---

// attachTile is one attached-file card. v1.0.3 redesign: the tile is
// ICON-FIRST — a large type-specific glyph (image / audio / video / archive
// / code / doc / gguf) is the anchor, with the clipped file name and a
// type/size hint beneath. Removable tiles live above the composer; compact
// ones render inside message bubbles. Hover glows the ember edge.
type attachTile struct {
        widget.BaseWidget
        bg      *canvas.Rectangle
        iconRes *canvas.Image
        name    *canvas.Text
        hint    *canvas.Text
        close   *canvas.Image
        obj     fyne.CanvasObject
        hover   *hoverFx
        onTap   func()
}

// newAttachTile builds a tile for the given file. When removable is true a
// remove ✕ renders in the corner; hint overrides the auto type hint.
func newAttachTile(fileName, hint string, removable bool, onRemove func()) *attachTile {
        a := &attachTile{onTap: onRemove}
        a.ExtendBaseWidget(a)
        a.bg = canvas.NewRectangle(ColBgDeep)
        a.bg.CornerRadius = 12
        a.bg.StrokeColor = ColBorderSoft
        a.bg.StrokeWidth = 1
        if hint == "" {
                hint = strings.TrimPrefix(filepathExt(fileName), ".")
                if hint == "" {
                        hint = "file"
                }
        }
        a.iconRes = canvas.NewImageFromResource(icon(iconForFile(fileName)))
        a.iconRes.SetMinSize(fyne.NewSize(26, 26))
        a.name = canvas.NewText(clipStrMemory(fileName, 22), ColText)
        a.name.TextSize = 11
        a.name.Alignment = fyne.TextAlignCenter
        a.hint = canvas.NewText(strings.ToUpper(hint), ColTextMuted)
        a.hint.TextSize = 9
        a.hint.TextStyle.Bold = true
        a.hint.Alignment = fyne.TextAlignCenter

        body := container.NewVBox(
                container.NewPadded(a.iconRes),
                container.NewPadded(a.name),
                container.NewPadded(a.hint),
        )
        if removable {
                a.close = canvas.NewImageFromResource(iconMuted("close"))
                a.close.SetMinSize(fyne.NewSize(12, 12))
                a.obj = container.NewStack(a.bg,
                        container.NewPadded(body),
                        container.NewBorder(nil, nil, nil, container.NewPadded(a.close), nil))
        } else {
                a.obj = container.NewStack(a.bg, container.NewPadded(body))
        }
        a.hover = newHoverFx(a.bg, ColBgDeep, color.NRGBA{R: 40, G: 18, B: 13, A: 255}, ColBorderSoft, color.NRGBA{R: 255, G: 90, B: 38, A: 120}, true)
        return a
}

func (a *attachTile) CreateRenderer() fyne.WidgetRenderer {
        return widget.NewSimpleRenderer(a.obj)
}

func (a *attachTile) Tapped(*fyne.PointEvent) {
        if a.onTap != nil {
                a.onTap()
        }
}

func (a *attachTile) MouseIn(*desktop.MouseEvent)    { a.hover.enter() }
func (a *attachTile) MouseOut()                      { a.hover.exit() }
func (a *attachTile) MouseMoved(*desktop.MouseEvent) {}

// newAttachChip keeps the compact horizontal form for message bubbles
// (smaller icon, single line).
func newAttachChip(fileName string, removable bool, onRemove func()) *attachTile {
        a := &attachTile{onTap: onRemove}
        a.ExtendBaseWidget(a)
        a.bg = canvas.NewRectangle(ColBgDeep)
        a.bg.CornerRadius = 9
        a.bg.StrokeColor = ColBorderSoft
        a.bg.StrokeWidth = 1
        a.iconRes = canvas.NewImageFromResource(icon(iconForFile(fileName)))
        a.iconRes.SetMinSize(fyne.NewSize(14, 14))
        a.name = canvas.NewText(clipStrMemory(fileName, 30), ColText)
        a.name.TextSize = 12
        a.hint = nil
        row := container.NewHBox(
                container.NewPadded(a.iconRes),
                container.NewPadded(a.name),
        )
        if removable {
                a.close = canvas.NewImageFromResource(iconMuted("close"))
                a.close.SetMinSize(fyne.NewSize(11, 11))
                row.Add(container.NewPadded(a.close))
        }
        a.obj = container.NewStack(a.bg, container.NewPadded(row))
        a.hover = newHoverFx(a.bg, ColBgDeep, color.NRGBA{R: 40, G: 18, B: 13, A: 255}, ColBorderSoft, color.NRGBA{R: 255, G: 90, B: 38, A: 120}, true)
        return a
}

// --- fire button (v1.0.7 Ember Luxe CTA) ---

// fireButton is the primary call-to-action button, rebuilt on the Ember
// Luxe system: a TRUE rounded gradient fill (raster-painted — Fyne has no
// rounded gradient primitive), layered elevation shadow, top-lit hairline
// bevel, an animated ember glow ring on hover, a press frame that darkens
// the gradient AND flashes a soft light, and a disabled state that rests
// the gradient into the chrome. Used for "New chat" and hero actions.
type fireButton struct {
        widget.BaseWidget
        obj     fyne.CanvasObject
        grad    *roundedGradient
        label   *canvas.Text
        iconRes *canvas.Image
        ringIn  func()
        ringOut func()
        flash   func()
        onTap   func()
        enabled bool
}

func newFireButton(label, iconName string, onTap func()) *fireButton {
        f := &fireButton{onTap: safeTap("fireButton", onTap), enabled: true}
        f.ExtendBaseWidget(f)

        f.grad = newRoundedGradient(gradEmberTop, gradEmberBottom, radiusSm+2)
        ring, ringIn, ringOut := glowRing(radiusSm + 2)

        f.iconRes = canvas.NewImageFromResource(whiteIcon(iconName))
        f.iconRes.SetMinSize(fyne.NewSize(16, 16))
        f.label = canvas.NewText(label, color.NRGBA{R: 255, G: 250, B: 247, A: 255})
        f.label.TextSize = 13
        f.label.TextStyle.Bold = true
        row := container.NewHBox(
                container.NewPadded(f.iconRes),
                container.NewPadded(f.label),
        )

        // Content inset (2) leaves room for the glow ring stroke around the
        // card; the shadow layers sink below it.
        card := container.NewStack(
                f.grad.raster,
                hairlines(10),
                container.NewPadded(row),
        )
        f.ringIn, f.ringOut = ringIn, ringOut
        overlay, flash := tapFlash(radiusSm + 2)
        f.flash = flash

        f.obj = container.NewStack(
                elevation(radiusSm+2, 2),
                ring,
                container.NewPadded(card), // 4px default padding = ring room
                overlay,
        )
        return f
}

func (f *fireButton) CreateRenderer() fyne.WidgetRenderer {
        return widget.NewSimpleRenderer(f.obj)
}

func (f *fireButton) Tapped(*fyne.PointEvent) {
        if f.onTap == nil || !f.enabled {
                return
        }
        // press frame: gradient darkens, light flashes, then fire
        f.grad.set(gradPressTop, gradPressBot)
        if f.flash != nil {
                f.flash()
        }
        go func() {
                time.Sleep(130 * time.Millisecond)
                runOnMain(func() {
                        if f.enabled {
                                f.grad.set(gradEmberTop, gradEmberBottom)
                        }
                })
        }()
        f.onTap()
}

func (f *fireButton) MouseIn(*desktop.MouseEvent) {
        if !f.enabled {
                return
        }
        f.grad.set(gradEmberHotTop, gradEmberHotBot)
        if f.ringIn != nil {
                f.ringIn()
        }
}

func (f *fireButton) MouseOut() {
        if !f.enabled {
                return
        }
        f.grad.set(gradEmberTop, gradEmberBottom)
        if f.ringOut != nil {
                f.ringOut()
        }
}

func (f *fireButton) MouseMoved(*desktop.MouseEvent) {}

// SetEnabled toggles the luxe rest state (disabled = quiet chrome).
func (f *fireButton) SetEnabled(on bool) {
        f.enabled = on
        if on {
                f.grad.set(gradEmberTop, gradEmberBottom)
                f.label.Color = color.NRGBA{R: 255, G: 250, B: 247, A: 255}
        } else {
                f.grad.set(
                        color.NRGBA{R: 52, G: 26, B: 20, A: 255},
                        color.NRGBA{R: 38, G: 18, B: 14, A: 255},
                )
                f.label.Color = color.NRGBA{R: 150, G: 122, B: 112, A: 255}
        }
        f.label.Refresh()
}

// safeMarkdown rewrites markdown TABLES into fenced code blocks. Fyne's
// RichText table renderer draws cells as free-floating objects that overlap
// surrounding paragraphs inside constrained columns (a real v1.0.0 audit
// finding) — monospace blocks render reliably and stay inside their bounds.
func safeMarkdown(md string) string {
        if !strings.Contains(md, "|") {
                return md
        }
        lines := strings.Split(md, "\n")
        var out []string
        var table []string
        flush := func() {
                if len(table) > 0 {
                        out = append(out, "```", strings.Join(table, "\n"), "```")
                        table = nil
                }
        }
        for _, ln := range lines {
                trimmed := strings.TrimSpace(ln)
                isRow := strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")
                isSep := isRow && !strings.ContainsAny(strings.Trim(trimmed, "| -:"), "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
                if isRow && !isSep {
                        table = append(table, trimmed)
                } else if isSep {
                        continue // separator rows add nothing in monospace form
                } else {
                        flush()
                        out = append(out, ln)
                }
        }
        flush()
        return strings.Join(out, "\n")
}

// escapeMD neutralizes markdown significance in user text so it renders
// exactly as typed.
func escapeMD(s string) string {
        s = strings.ReplaceAll(s, "*", "\\*")
        s = strings.ReplaceAll(s, "_", "\\_")
        s = strings.ReplaceAll(s, "`", "\\`")
        return s
}

// --- activity row (pro mode) ---

// newActivityRowWidget builds one agent-activity line: timestamp + caption.
// v1.0.0: the caption is the CENTER of a Border so it stretches to the
// remaining row width — a Truncation=Clip label in an HBox is sized by its
// (collapsing) MinSize and can shrink to one glyph after a relayout
// (the footer "SHE…" bug's sibling).
func newActivityRowWidget(a agent.Activity) fyne.CanvasObject {
        ts := widget.NewLabel(a.Timestamp.Format("15:04:05"))
        ts.TextStyle = fyne.TextStyle{Monospace: true}
        ts.Importance = widget.LowImportance
        caption := a.Caption
        if len(caption) > 120 {
                caption = caption[:120] + "…"
        }
        cap := widget.NewLabel(caption)
        if a.Type == "error" {
                cap.Importance = widget.DangerImportance
        }
        return container.NewBorder(nil, nil, container.NewPadded(ts), nil, cap)
}

// newLiveResponseRow builds the streaming-response row for the Pro activity
// stream (v1.0.1). It returns the row and its live caption label so
// appendActivity can update the text in place — one widget per streamed
// answer instead of one per token delta.
func newLiveResponseRow(caption string) (fyne.CanvasObject, *widget.Label) {
        marker := widget.NewLabel("»")
        marker.TextStyle = fyne.TextStyle{Monospace: true}
        marker.Importance = widget.LowImportance
        if len(caption) > 120 {
                caption = caption[:120] + "…"
        }
        lbl := widget.NewLabel(caption)
        return container.NewBorder(nil, nil, container.NewPadded(marker), nil, lbl), lbl
}

// newLiveReasoningRow builds the streaming THINKING row for the Pro activity
// stream (v1.0.2): dimmed brain marker + live label, updated in place.
func newLiveReasoningRow(caption string) (fyne.CanvasObject, *widget.Label) {
        ic := canvas.NewImageFromResource(iconMuted("brain"))
        ic.SetMinSize(fyne.NewSize(13, 13))
        if len(caption) > 120 {
                caption = caption[:120] + "…"
        }
        lbl := widget.NewLabel(caption)
        lbl.Importance = widget.LowImportance
        lbl.TextStyle = fyne.TextStyle{Italic: true}
        return container.NewBorder(nil, nil, container.NewPadded(ic), nil, lbl), lbl
}

// --- session row ---

type sessionRow struct {
        widget.BaseWidget
        obj   fyne.CanvasObject
        title *widget.Label
        meta  *widget.Label
}

func (s *sessionRow) CreateRenderer() fyne.WidgetRenderer {
        return widget.NewSimpleRenderer(s.obj)
}

func newSessionRow() *sessionRow {
        s := &sessionRow{}
        s.ExtendBaseWidget(s)
        s.title = widget.NewLabel("")
        s.title.TextStyle = fyne.TextStyle{Bold: true}
        s.title.Truncation = fyne.TextTruncateClip
        s.meta = widget.NewLabel("")
        s.meta.Importance = widget.LowImportance
        s.meta.Truncation = fyne.TextTruncateClip
        s.obj = container.NewVBox(s.title, s.meta)
        return s
}

func (s *sessionRow) Set(title, meta string, active bool) {
        if active {
                s.title.Importance = widget.HighImportance
        } else {
                s.title.Importance = widget.MediumImportance
        }
        s.title.SetText(title)
        s.meta.SetText(meta)
        // v1.0.0: force a re-measure AFTER SetText — Labels created empty and
        // later filled can keep a stale one-glyph MinSize after Refresh.
        s.title.Refresh()
        s.meta.Refresh()
}

// --- chart grid cell (real widget so the painter walks into it) ---

type chartCell struct {
        widget.BaseWidget
        obj  fyne.CanvasObject
        img  *canvas.Image
        name *widget.Label
        meta *widget.Label
}

func (c *chartCell) CreateRenderer() fyne.WidgetRenderer {
        return widget.NewSimpleRenderer(c.obj)
}

// --- fire button helpers (v1.0.8: factories return the Aurora system) ---

// primaryButton returns the v1.0.8 Aurora gradient CTA pill — a real
// painted gradient with elevation, glow ring and press motion (replaces
// the stock widget.Button that read as a flat gray rectangle).
func primaryButton(label, iconName string, onTap func()) *actionButton {
        return newActionButton(label, iconName, true, onTap)
}

// ghostButton returns the v1.0.8 Aurora quiet pill — transparent at rest,
// warm ember hover, hairline edge. The z.ai/ChatGPT secondary idiom.
func ghostButton(label, iconName string, onTap func()) *actionButton {
        return newActionButton(label, iconName, false, onTap)
}

// whiteIcon returns a white variant of an icon (used inside the ember send
// circle). v1.0.8: results are cached — these fire on every hover/active
// toggle and each call previously re-rendered the SVG string + resource.
var whiteIconCache sync.Map // name → fyne.Resource

func whiteIcon(name string) fyne.Resource {
        if cached, ok := whiteIconCache.Load(name); ok {
                return cached.(fyne.Resource)
        }
        var res fyne.Resource
        if body, ok := iconBodies[name]; ok {
                res = resource("sheytan-"+name+"-white", iconSVG(body, "#FFFFFF"))
        } else {
                res = icon(name)
        }
        whiteIconCache.Store(name, res)
        return res
}

// fmtPillText builds the model/provider status label.
func fmtPillText(provider, model string) string {
        return fmt.Sprintf("%s · %s", strings.ToUpper(provider), model)
}
