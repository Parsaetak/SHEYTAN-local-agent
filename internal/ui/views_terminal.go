// Package ui — v1.0.6 Terminal view: the app's own Linux-like console. The
// same termshell engine the agent drives through the `linux` tool, rendered
// as an interactive panel: monospace output with prompt coloring, command
// history chips, and a live prompt. Fully jailed to the app folder — nothing
// outside it is reachable, on any machine.
package ui

import (
	"image/color"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/sheytan/local-agent/internal/termshell"
)

// termColorPalette — the classic console look: light text on near-black,
// ember prompt, red errors. Reads instantly as "a real terminal".
var (
	termBg      = color.NRGBA{R: 18, G: 10, B: 8, A: 255}
	termTextCol = color.NRGBA{R: 232, G: 224, B: 218, A: 255}
	termPrompt  = color.NRGBA{R: 255, G: 138, B: 80, A: 255} // ember
	termErr     = color.NRGBA{R: 240, G: 96, B: 72, A: 255}
	termDim     = color.NRGBA{R: 150, G: 130, B: 120, A: 255}
)

// termMaxLines caps the rendered stream — a terminal that ran ten thousand
// commands must stay instant (the chunking mindset, applied to the console).
const termMaxLines = 500

// terminalView is the interactive terminal panel.
type terminalView struct {
	app     *desktopApp
	eng     *termshell.Engine
	mu      sync.Mutex
	lines   *fyne.Container // VBox of canvas.Text rows
	scroll  *container.Scroll
	in      *widget.Entry
	histBox *fyne.Container
}

// buildTerminalView constructs the Terminal panel around the shared engine
// (the agent's `linux` tool and this view see the same cwd + history).
func (d *desktopApp) buildTerminalView() fyne.CanvasObject {
	var eng *termshell.Engine
	if d.stack != nil && d.stack.Linux != nil {
		eng = d.stack.Linux.Engine()
	}
	if eng == nil {
		eng = termshell.New(d.cfg.DataDir)
	}
	tv := &terminalView{app: d, eng: eng}

	tv.lines = container.NewVBox()

	outBg := canvas.NewRectangle(termBg)
	outBg.CornerRadius = 10
	tv.scroll = container.NewVScroll(container.NewStack(outBg, container.NewPadded(tv.lines)))
	tv.scroll.SetMinSize(fyne.NewSize(10, 360))

	tv.in = widget.NewEntry()
	tv.in.TextStyle = fyne.TextStyle{Monospace: true}
	tv.in.SetPlaceHolder("type a command — try: neofetch, ls -l, tree, help")
	tv.in.OnSubmitted = func(s string) { tv.run(s) }

	promptTxt := canvas.NewText("user@sheytan", termPrompt)
	promptTxt.TextStyle.Bold = true
	promptTxt.TextSize = 13
	dotTxt := canvas.NewText(":~$ ", termDim)
	dotTxt.TextSize = 13
	inRow := container.NewBorder(nil, nil,
		container.NewHBox(container.NewPadded(promptTxt), container.NewPadded(dotTxt)),
		nil,
		container.NewPadded(tv.in),
	)

	tv.histBox = container.NewHBox()
	histScroll := container.NewHScroll(tv.histBox)

	clearBtn := newChipButton("close", "Clear", func() { tv.clear() })
	helpBtn := newChipButton("info", "Help", func() { tv.run("help") })
	fetchBtn := newChipButton("spark", "Neofetch", func() { tv.run("neofetch") })
	header := sectionHeader("terminal", "Terminal",
		layout.NewSpacer(), fetchBtn.obj, helpBtn.obj, clearBtn.obj)

	body := container.NewBorder(
		container.NewPadded(header),
		container.NewPadded(container.NewVBox(
			widget.NewSeparator(),
			tv.scroll,
			container.NewPadded(histScroll),
			inRow,
		)),
		nil, nil,
		nil,
	)

	d.term = tv
	tv.run("neofetch")
	return body
}

// renderHistory rebuilds the click-to-rerun chips (newest first, max 8).
func (tv *terminalView) renderHistory() {
	tv.histBox.RemoveAll()
	hist := tv.eng.History()
	n := len(hist)
	start := n - 8
	if start < 0 {
		start = 0
	}
	for i := n - 1; i >= start; i-- {
		cmd := hist[i]
		chip := newChipButton("terminal", cmd, func() {
			tv.in.SetText(cmd)
		})
		tv.histBox.Add(chip.obj)
	}
	tv.histBox.Refresh()
}

// appendLine adds one colored row and trims the stream at the budget.
func (tv *terminalView) appendLine(text string, col color.Color, bold, mono bool) {
	if text == "" {
		text = " "
	}
	t := canvas.NewText(text, col)
	t.TextSize = 13
	t.TextStyle.Monospace = mono
	t.TextStyle.Bold = bold
	tv.lines.Add(t)
	for len(tv.lines.Objects) > termMaxLines {
		tv.lines.Remove(tv.lines.Objects[0])
	}
}

// isErrLine heuristic for red output rows.
func isErrLine(l string) bool {
	return strings.Contains(l, "command not found") ||
		strings.Contains(l, "permission denied") ||
		strings.Contains(l, "no such file or directory") ||
		strings.HasPrefix(l, "rm: ") || strings.HasPrefix(l, "cd: ") ||
		strings.HasPrefix(l, "cat: ") || strings.HasPrefix(l, "ls: ")
}

// run executes one command: prompt echo, output, history refresh.
func (tv *terminalView) run(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	tv.mu.Lock()
	defer tv.mu.Unlock()

	tv.appendLine(tv.eng.Prompt()+line, termPrompt, true, true)
	out := tv.eng.Exec(line)
	if out == termshell.ClearMarker {
		tv.clearLocked()
		tv.renderHistory()
		return
	}
	for _, l := range strings.Split(out, "\n") {
		col := termTextCol
		if isErrLine(l) {
			col = termErr
		}
		tv.appendLine(l, col, false, true)
	}
	tv.renderHistory()
	tv.in.SetText("")
	tv.lines.Refresh()
	tv.scroll.ScrollToBottom()
}

// clear wipes the output stream.
func (tv *terminalView) clear() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.clearLocked()
}

func (tv *terminalView) clearLocked() {
	tv.lines.RemoveAll()
	tv.appendLine("— cleared —", termDim, false, true)
	tv.lines.Refresh()
	tv.scroll.ScrollToBottom()
}
