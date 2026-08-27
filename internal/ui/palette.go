// Package ui — v1.0.6 command palette (Ctrl+K): the modern "search everything,
// do anything" surface. Every navigation target, tool toggle and frequent
// action is one keystroke away — the interaction top-market products are
// judged by.
package ui

import (
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/sheytan/local-agent/internal/brand"
)

// paletteAction is one runnable command.
type paletteAction struct {
	Title    string
	Keywords string // lowercased haystack for filtering
	Icon     string
	Run      func()
}

// paletteActions returns the live command list (order = display order).
func (d *desktopApp) paletteActions() []paletteAction {
	acts := []paletteAction{
		{Title: "New chat", Keywords: "new chat session start", Icon: "new", Run: d.newSession},
		{Title: "Capture screen & ask", Keywords: "screenshot vision camera screen capture", Icon: "camera", Run: d.captureScreen},
		{Title: "Extend context now — next chapter", Keywords: "continuum context extend chapter rollover unlimited memory", Icon: "layers", Run: d.forceRollover},
		{Title: "Context & memory…", Keywords: "context usage meter memory thread state framework", Icon: "context", Run: d.showContextDialog},
		{Title: "Choose model…", Keywords: "model gguf pick select engine", Icon: "model", Run: d.pickModel},
		{Title: "Open Terminal", Keywords: "terminal console linux shell sim", Icon: "terminal", Run: func() { d.showView("terminal") }},
		{Title: "Open Resources", Keywords: "resources disk usage memory quota cleanup gauge", Icon: "gauge", Run: func() { d.showView("resources") }},
		{Title: "Models manager", Keywords: "models storage vram fit delete", Icon: "gguf", Run: func() { d.showView("models") }},
		{Title: "Files", Keywords: "files artifacts created preview explorer", Icon: "files", Run: func() { d.showView("files") }},
		{Title: "Charts", Keywords: "charts data visualization svg", Icon: "data", Run: func() { d.showView("data") }},
		{Title: "Settings…", Keywords: "settings preferences config", Icon: "settings", Run: d.showSettings},
		{Title: "LLM provider…", Keywords: "provider remote api endpoint openai compatible", Icon: "provider", Run: d.showProviderDialog},
		{Title: "Multi-agent pipeline…", Keywords: "multi agent pipeline planner critic executor", Icon: "agent", Run: d.runPipeline},
		{Title: "System info", Keywords: "system info cpu gpu ram specs", Icon: "system", Run: d.showSysinfo},
		{Title: "Check for engine update…", Keywords: "update engine llama cpp latest check", Icon: "update", Run: d.checkUpdatesNow},
		{Title: "Run stress tests", Keywords: "stress test selftest qa", Icon: "bolt", Run: d.runStress},
		{Title: "Export diagnostics…", Keywords: "diagnostics export logs zip support", Icon: "export", Run: d.exportDiagnostics},
		{Title: "About " + brand.Trademark, Keywords: "about license version trademark", Icon: "info", Run: d.showAbout},
	}
	if d.cfg != nil {
		if d.cfg.ProMode {
			acts = append(acts,
				paletteAction{Title: "Memory", Keywords: "memory recall facts brain", Icon: "memory", Run: func() { d.showView("memory") }},
				paletteAction{Title: "Logs", Keywords: "logs pro telemetry activity", Icon: "logs", Run: func() { d.showView("logs") }},
				paletteAction{Title: "Disable Pro mode", Keywords: "pro mode simple minimal toggle off", Icon: "panel", Run: func() { d.setProMode(false) }},
			)
		} else {
			acts = append(acts, paletteAction{Title: "Enable Pro mode", Keywords: "pro mode dock tabs advanced toggle", Icon: "panel", Run: func() { d.setProMode(true) }})
		}
		if d.cfg.ThinkingMode {
			acts = append(acts, paletteAction{Title: "Disable thinking mode", Keywords: "thinking mode reason toggle off", Icon: "brain", Run: d.toggleThinking})
		} else {
			acts = append(acts, paletteAction{Title: "Enable thinking mode", Keywords: "thinking mode reason toggle", Icon: "brain", Run: d.toggleThinking})
		}
	}
	if d.llama != nil {
		if d.llama.IsRunning() {
			acts = append(acts, paletteAction{Title: "Stop llama.cpp engine", Keywords: "engine stop llama kill shutdown", Icon: "stop", Run: d.stopLlama})
		} else {
			acts = append(acts, paletteAction{Title: "Start llama.cpp engine", Keywords: "engine start llama boot", Icon: "engine", Run: d.startLlama})
		}
	}
	return acts
}

// paletteFilter filters actions by the query (title or keywords contain) and
// ranks title hits first.
func paletteFilter(acts []paletteAction, query string) []paletteAction {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return acts
	}
	var out []paletteAction
	for _, a := range acts {
		if strings.Contains(strings.ToLower(a.Title), q) || strings.Contains(a.Keywords, q) {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti := strings.Contains(strings.ToLower(out[i].Title), q)
		tj := strings.Contains(strings.ToLower(out[j].Title), q)
		return ti && !tj
	})
	return out
}

// paletteRow adapts a paletteAction to a tappable list row with hover glow.
type paletteRow struct {
	widget.BaseWidget
	obj fyne.CanvasObject
	hov *hoverFx
	act paletteAction
}

func newPaletteRow(a paletteAction) *paletteRow {
	r := &paletteRow{act: a}
	r.ExtendBaseWidget(r)
	bg := canvas.NewRectangle(color.Transparent)
	r.hov = newHoverFx(bg, color.Transparent, color.NRGBA{R: 40, G: 18, B: 13, A: 255}, color.Transparent, color.Transparent, false)

	ic := canvas.NewImageFromResource(icon(a.Icon))
	ic.SetMinSize(fyne.NewSize(16, 16))
	title := canvas.NewText(a.Title, ColText)
	title.TextSize = 13
	r.obj = container.NewStack(bg, container.NewPadded(container.NewHBox(container.NewPadded(ic), container.NewPadded(title))))
	return r
}

func (r *paletteRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.obj)
}

func (r *paletteRow) Tapped(*fyne.PointEvent) {
	run := r.act.Run
	if dlg := paletteOpen; dlg != nil {
		paletteOpen = nil
		dlg.Hide()
	}
	if run != nil {
		run()
	}
}

func (r *paletteRow) MouseIn(*desktop.MouseEvent)    { r.hov.enter() }
func (r *paletteRow) MouseOut()                      { r.hov.exit() }
func (r *paletteRow) MouseMoved(*desktop.MouseEvent) {}

// paletteOpen tracks the currently-open palette dialog (at most one).
var paletteOpen *dialog.CustomDialog

// showPalette opens the Ctrl+K command palette.
func (d *desktopApp) showPalette() {
	if d.win == nil {
		return
	}
	acts := d.paletteActions()

	search := widget.NewEntry()
	search.SetPlaceHolder("Search commands…")

	list := container.NewVBox()
	var filtered []paletteAction

	render := func() {
		filtered = paletteFilter(acts, search.Text)
		list.RemoveAll()
		for _, a := range filtered {
			list.Add(newPaletteRow(a))
		}
		if len(filtered) == 0 {
			list.Add(widget.NewLabel("No commands match — try 'model', 'terminal', 'screen'…"))
		}
		list.Refresh()
	}
	search.OnChanged = func(string) { render() }
	search.OnSubmitted = func(string) {
		// Enter runs the first match.
		if len(filtered) > 0 && filtered[0].Run != nil {
			run := filtered[0].Run
			if dlg := paletteOpen; dlg != nil {
				paletteOpen = nil
				dlg.Hide()
			}
			run()
		}
	}

	render()

	hintTxt := canvas.NewText("Enter runs the first match · Esc closes", ColTextMuted)
	hintTxt.TextSize = 10
	body := container.NewVBox(
		container.NewPadded(search),
		widget.NewSeparator(),
		container.NewVScroll(list),
		container.NewPadded(hintTxt),
	)
	dlg := d.bigDialog("Command palette", body, 560, 460)
	paletteOpen = dlg
	dlg.Show()
	d.win.Canvas().Focus(search)
}
