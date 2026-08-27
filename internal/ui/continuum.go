// Package ui — v1.0.7 Continuum surfaces: the context meter (live pressure
// gauge above the composer), the chapter divider card (the "context
// extended" moment), the rollover orchestration in the desktop layer, and
// the context dialog (usage detail + manual "Extend now").
//
// The meter is the honest face of the engine: users SEE the context fill
// up, SEE it roll over into a new chapter, and SEE the pressure reset —
// "almost unlimited context" becomes a visible, trustworthy mechanic
// instead of a marketing line.
package ui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/sheytan/local-agent/internal/continuum"
	"github.com/sheytan/local-agent/internal/llm"
	"github.com/sheytan/local-agent/internal/logging"
)

// --- context meter ---

// meterLevelColor maps usage pressure to the fill color (ember → gold →
// hot orange → danger red).
func meterLevelColor(pct float64) color.NRGBA {
	switch {
	case pct >= 95:
		return ColDanger
	case pct >= 75:
		return color.NRGBA{R: 255, G: 120, B: 40, A: 255}
	case pct >= 50:
		return ColGold
	default:
		return ColEmber
	}
}

// contextMeter is the slim live gauge above the composer: label, gradient
// track, chapter chip. Tapping opens the context dialog (usage detail +
// manual chapter extension).
type contextMeter struct {
	widget.BaseWidget
	track   *canvas.Rectangle
	fill    *canvas.Rectangle
	lbl     *canvas.Text
	chip    *canvas.Text
	chipBg  *canvas.Rectangle
	stack   *fyne.Container
	obj     fyne.CanvasObject
	pct     float64
	chapter int
	onTap   func()
}

func newContextMeter(onTap func()) *contextMeter {
	m := &contextMeter{onTap: onTap, chapter: 1}
	m.ExtendBaseWidget(m)

	m.track = canvas.NewRectangle(color.NRGBA{R: 20, G: 10, B: 8, A: 255})
	m.track.CornerRadius = 3
	m.track.StrokeColor = ColBorderSoft
	m.track.StrokeWidth = 1
	m.fill = canvas.NewRectangle(ColEmber)
	m.fill.CornerRadius = 3
	m.fill.Hide() // hidden until first Set

	m.lbl = canvas.NewText("context —", color.NRGBA{R: 216, G: 190, B: 180, A: 255}) // brighter than ColTextMuted: survives the 10px size next to the bar
	m.lbl.TextSize = 10
	m.lbl.TextStyle.Bold = true

	m.chip = canvas.NewText("CH 1", color.NRGBA{R: 255, G: 158, B: 110, A: 255})
	m.chip.TextSize = 10
	m.chip.TextStyle.Bold = true
	m.chipBg = canvas.NewRectangle(color.NRGBA{R: 58, G: 26, B: 18, A: 255})
	m.chipBg.CornerRadius = 7
	chipWrap := container.NewStack(m.chipBg, container.NewPadded(m.chip))

	m.stack = container.New(&meterLayout{m: m}, m.track, m.fill)
	m.obj = container.NewBorder(nil, nil,
		container.NewPadded(m.lbl),
		container.NewPadded(chipWrap),
		container.NewPadded(m.stack))
	return m
}

// meterLayout lays the track full-width and the fill to the usage fraction.
type meterLayout struct {
	m *contextMeter
}

func (l meterLayout) MinSize(_ []fyne.CanvasObject) fyne.Size { return fyne.NewSize(120, 6) }

func (l meterLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	w := size.Width
	if w < 1 {
		w = 1
	}
	h := size.Height
	if h < 1 {
		h = 1
	}
	safe := fyne.NewSize(w, h)
	for _, o := range objs {
		o.Resize(safe)
		o.Move(fyne.NewPos(0, 0))
	}
	// fill sized to the CURRENT fraction (Set animates it directly)
	if l.m.pct > 0 {
		fw := w * float32(clampF64(l.m.pct/100, 0.02, 1))
		if fw < 2 {
			fw = 2
		}
		l.m.fill.Resize(fyne.NewSize(fw, h))
		l.m.fill.Move(fyne.NewPos(0, 0))
	}
}

func clampF64(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m *contextMeter) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(m.obj)
}

func (m *contextMeter) Tapped(*fyne.PointEvent) {
	if m.onTap != nil {
		m.onTap()
	}
}

func (m *contextMeter) MouseIn(*desktop.MouseEvent)    {}
func (m *contextMeter) MouseOut()                      {}
func (m *contextMeter) MouseMoved(*desktop.MouseEvent) {}

// Set updates the meter: animates the fill from the current fraction to
// the new one, recolors by level, refreshes the labels.
func (m *contextMeter) Set(pct float64, chapter int, estTokens, budgetTokens int) {
	from := m.pct
	m.pct = pct
	m.chapter = chapter

	m.fill.Show()
	m.fill.FillColor = meterLevelColor(pct)

	m.lbl.Text = fmt.Sprintf("context %.0f%%", pct)
	if estTokens > 0 {
		m.lbl.Text = fmt.Sprintf("context %.0f%% · %dk/%dk", pct, (estTokens+999)/1000, (budgetTokens+999)/1000)
	}
	m.chip.Text = fmt.Sprintf("CH %d", chapter)
	if chapter > 1 {
		m.chipBg.FillColor = color.NRGBA{R: 84, G: 32, B: 14, A: 255}
	} else {
		m.chipBg.FillColor = color.NRGBA{R: 58, G: 26, B: 18, A: 255}
	}

	// animate fill width from → to
	trackW := m.stack.Size().Width
	if trackW <= 0 {
		trackW = 160
	}
	fromW := trackW * float32(clampF64(from/100, 0, 1))
	toW := trackW * float32(clampF64(pct/100, 0.02, 1))
	if from <= 0 {
		m.fill.Resize(fyne.NewSize(toW, m.stack.Size().Height))
	} else {
		a := fyne.NewAnimation(360*time.Millisecond, func(v float32) {
			w := fromW + (toW-fromW)*v
			m.fill.Resize(fyne.NewSize(w, m.stack.Size().Height))
			canvas.Refresh(m.fill)
		})
		a.Curve = fyne.AnimationEaseOut
		a.Start()
	}

	m.lbl.Refresh()
	m.chip.Refresh()
	m.chipBg.Refresh()
	canvas.Refresh(m.fill)
}

// --- chapter divider card ---

// newChapterDivider renders the in-chat "context extended" card: the visible
// moment the Continuum engine rolls the conversation into a new chapter.
// Slim, centered, glass — a milestone marker, not an interruption.
func newChapterDivider(chapter, facts, files, carried int) fyne.CanvasObject {
	badge := luxeBadge("layers", 26, color.NRGBA{R: 255, G: 120, B: 50, A: 52})
	title := canvas.NewText(fmt.Sprintf("Chapter %d — context extended", chapter), color.NRGBA{R: 255, G: 158, B: 110, A: 255})
	title.TextSize = 12
	title.TextStyle.Bold = true

	details := []string{fmt.Sprintf("%d facts carried", facts)}
	if files > 0 {
		details = append(details, fmt.Sprintf("%d files", files))
	}
	if carried > 0 {
		details = append(details, fmt.Sprintf("%d recent messages", carried))
	}
	sub := canvas.NewText("Memory carried forward · "+strings.Join(details, " · "), ColTextMuted)
	sub.TextSize = 10

	row := container.NewHBox(
		container.NewPadded(badge),
		container.NewVBox(
			container.NewPadded(title),
			container.NewPadded(sub),
		),
	)

	bg := canvas.NewRectangle(color.NRGBA{R: 38, G: 18, B: 13, A: 235})
	bg.CornerRadius = radiusPill
	bg.StrokeColor = color.NRGBA{R: 255, G: 130, B: 70, A: 46}
	bg.StrokeWidth = 1
	return container.NewStack(bg, hairlines(16), container.NewPadded(row))
}

// --- desktop integration ---

// updateContextMeter recomputes usage for the active session and drives the
// meter (called after every turn, on session switch, on config change).
func (d *desktopApp) updateContextMeter() {
	if d.ctxMeter == nil {
		return
	}
	if d.active == nil {
		d.ctxMeter.Set(0, 1, 0, d.cfg.HistoryWindowTokens())
		return
	}
	u := continuum.SessionUsage(d.active, d.cfg)
	chapter := d.active.Chapter
	if chapter == 0 {
		chapter = 1
	}
	d.ctxMeter.Set(u.Pct, chapter, u.EstTokens, u.BudgetTokens)
}

// maybeRollover is the Continuum heartbeat: after every completed turn it
// checks pressure and, at the threshold, transparently rolls the
// conversation into the next chapter — a NEW session is created in the
// background, seeded with the distilled Framework + recent tail, and
// swapped in as active before the user types the next word.
func (d *desktopApp) maybeRollover() {
	if !d.cfg.ContinuumEnabled || d.active == nil {
		return
	}
	mgr := continuum.NewManager(d.store, d.cfg.SessionsDir)
	if !mgr.ShouldRollover(d.active, d.cfg) {
		d.updateContextMeter()
		return
	}
	d.performRollover(mgr, false)
}

// forceRollover is the manual entry point (context dialog + palette).
func (d *desktopApp) forceRollover() {
	if d.turnRunning {
		d.setStatus("Wait for the current reply — then extend the context")
		return
	}
	if d.rolling {
		return
	}
	if d.active == nil || len(d.active.Messages) < 2 {
		d.setStatus("Nothing to extend yet — send a message first")
		return
	}
	mgr := continuum.NewManager(d.store, d.cfg.SessionsDir)
	d.performRollover(mgr, true)
}

// performRollover executes the chapter transition and updates the UI.
func (d *desktopApp) performRollover(mgr *continuum.Manager, forced bool) {
	if d.rolling {
		return
	}
	d.rolling = true
	defer func() { d.rolling = false }()

	parent := d.active
	u := continuum.SessionUsage(parent, d.cfg)
	if !forced && u.Pct < float64(d.cfg.EffectiveContinuumThreshold()) {
		return
	}

	child, fw, err := mgr.Rollover(parent, d.cfg)
	if err != nil {
		d.setStatus("Continuum: " + err.Error())
		logging.Default().Warn("gui", "continuum rollover failed: %v", err)
		return
	}

	// Keep the visual stream — append the divider, swap the session under
	// it. The user's chat never re-renders; the next turn simply continues
	// in the new chapter.
	carried := len(child.Messages) - 1
	if carried < 0 {
		carried = 0
	}
	if d.chatBox != nil {
		d.chatBox.Add(newChapterDivider(child.Chapter, fw.FactCount(), len(fw.Artifacts), carried))
		d.chatScroll.ScrollToBottom()
	}
	d.active = child
	d.reloadSessions()
	d.updateContextMeter()
	d.setStatus(fmt.Sprintf("Context extended — chapter %d · memory carried forward", child.Chapter))
	logging.Default().Info("gui", "continuum: rolled session %s (ch %d) into %s (ch %d): %d facts, %d artifacts",
		parent.ID, parent.Chapter, child.ID, child.Chapter, fw.FactCount(), len(fw.Artifacts))

	// Best-effort LLM refinement of the framework in the background — the
	// extractive snapshot is already live; a smarter one lands before the
	// next turn whenever the engine can produce it.
	parentMsgs := append([]llm.Message{}, parent.Messages...)
	childID := child.ID
	go func() {
		if d.client == nil {
			return
		}
		refined := continuum.Enhance(context.Background(), d.client, d.cfg.EffectiveModel(), fw, parentMsgs)
		if refined == nil {
			return
		}
		_ = continuum.SaveFramework(d.cfg.SessionsDir, childID, refined)
		logging.Default().Info("gui", "continuum: framework LLM-refined for %s", childID)
	}()
}

// showContextDialog is the meter's tap target: live pressure, chapter
// state, framework summary, manual extend, settings link.
func (d *desktopApp) showContextDialog() {
	u := d.lastUsage
	chapter := 1
	threadFacts := 0
	if d.active != nil {
		u = continuum.SessionUsage(d.active, d.cfg)
		chapter = d.active.Chapter
		if chapter == 0 {
			chapter = 1
		}
		if fw := continuum.LoadFramework(d.cfg.SessionsDir, d.active.ID); fw != nil {
			threadFacts = fw.FactCount()
		}
	}

	pressure := widget.NewProgressBar()
	pressure.Min = 0
	pressure.Max = 100
	pressure.SetValue(clampF64(u.Pct, 0, 100))

	stats := widget.NewLabel(fmt.Sprintf(
		"History: ~%d / %d tokens (%.0f%%)\nChapter: %d in this thread\nFramework memory: %d items\nThreshold: %d%% · carry %d messages · briefing budget %d tokens\n\nChapters roll over automatically at the threshold. Each new chapter starts with a distilled state briefing (mission, facts, decisions, open threads) plus the most recent messages — the conversation effectively never runs out of context.",
		u.EstTokens, u.BudgetTokens, u.Pct, chapter, threadFacts,
		d.cfg.EffectiveContinuumThreshold(), d.cfg.EffectiveContinuumCarry(), d.cfg.EffectiveContinuumFrameworkTokens()))
	stats.Wrapping = fyne.TextWrapWord

	extendBtn := widget.NewButtonWithIcon("Extend now — start next chapter", icon("layers"), d.forceRollover)
	extendBtn.Importance = widget.HighImportance

	body := container.NewVBox(
		container.NewPadded(pressure),
		container.NewPadded(stats),
		widget.NewSeparator(),
		container.NewPadded(extendBtn),
	)
	d.bigDialog("Context", scrollDialogContent(body, fyne.NewSize(520, 380)), 580, 470)
}
