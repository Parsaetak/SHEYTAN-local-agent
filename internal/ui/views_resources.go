// Package ui — v1.0.6 Resources view: a professional accounting of where the
// app's disk goes and what the engine consumes — live storage bars per
// folder, engine + process memory, allocation controls (threads, GPU layers,
// context, pipeline depth) and one-click cleanups that honor the quotas.
package ui

import (
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/resources"
	"github.com/sheytan/local-agent/internal/sysinfo"
)

// usageBar is one labeled horizontal bar (disk / RAM usage).
type usageBar struct {
	name  *canvas.Text
	bar   *canvas.Rectangle
	track *canvas.Rectangle
	val   *canvas.Text
	row   fyne.CanvasObject
}

const usageBarWidth float32 = 260

func newUsageBar() *usageBar {
	u := &usageBar{}
	u.name = canvas.NewText("", ColText)
	u.name.TextSize = 12
	u.track = canvas.NewRectangle(ColBgDeep)
	u.track.CornerRadius = 4
	u.track.SetMinSize(fyne.NewSize(usageBarWidth, 8))
	u.bar = canvas.NewRectangle(ColEmber)
	u.bar.CornerRadius = 4
	u.bar.SetMinSize(fyne.NewSize(0, 8))
	u.val = canvas.NewText("", ColTextMuted)
	u.val.TextSize = 11
	u.row = container.NewVBox(
		container.NewHBox(u.name, layout.NewSpacer(), u.val),
		// a small gap so labels never touch the bar track
		container.NewPadded(container.NewStack(u.track, container.NewVBox(container.NewPadded(u.bar)))),
	)
	return u
}

func (u *usageBar) Set(name string, bytes int64, max int64) {
	u.name.Text = name
	u.val.Text = resources.HumanBytes(bytes)
	frac := 0.0
	if max > 0 {
		frac = float64(bytes) / float64(max)
	}
	if frac > 1 {
		frac = 1
	}
	u.bar.SetMinSize(fyne.NewSize(usageBarWidth*float32(frac), 8))
	// Over quota → danger color; a zero-value bar renders as an empty track.
	u.bar.FillColor = ColEmber
	if max > 0 && bytes > max {
		u.bar.FillColor = color.NRGBA{R: 240, G: 96, B: 72, A: 255}
	}
	u.name.Refresh()
	u.val.Refresh()
	u.bar.Refresh()
}

// resourcesView is the Resources panel state.
type resourcesView struct {
	app     *desktopApp
	bars    []*usageBar
	storage *fyne.Container
	engine  *usageBar
	appRAM  *usageBar
	header  *canvas.Text

	// allocation inputs
	threadsIn *widget.Entry
	gpuIn     *widget.Entry
	ctxIn     *widget.Entry
	depthIn   *widget.Select
	workQuota *widget.Entry
	sessKeep  *widget.Entry
	logCap    *widget.Entry

	obj fyne.CanvasObject
}

// buildResourcesView constructs the Resources panel.
func (d *desktopApp) buildResourcesView() fyne.CanvasObject {
	rv := &resourcesView{app: d}
	rv.header = canvas.NewText("", ColTextMuted)
	rv.header.TextSize = 12

	rv.storage = container.NewVBox()
	rv.engine = newUsageBar()
	rv.appRAM = newUsageBar()

	refreshBtn := newChipButton("refresh", "Refresh", func() { rv.refresh() })
	header := sectionHeader("gauge", "Resources",
		layout.NewSpacer(), refreshBtn.obj)

	// --- live usage section ---
	// RAM bars sit side by side (compact, professional); disk bars stack.
	live := container.NewVBox(
		container.NewPadded(canvasText("LIVE USAGE", ColTextMuted, 11, true)),
		container.NewGridWithColumns(2,
			container.NewPadded(rv.engine.row),
			container.NewPadded(rv.appRAM.row),
		),
		widget.NewSeparator(),
		container.NewPadded(canvasText("DISK — WHERE THE APP'S DATA LIVES", ColTextMuted, 11, true)),
		container.NewPadded(rv.storage),
	)

	// --- engine allocation section (applies on restart) ---
	rv.threadsIn = widget.NewEntry()
	rv.threadsIn.SetText(fmt.Sprintf("%d", d.cfg.LLM.NumThread))
	rv.gpuIn = widget.NewEntry()
	rv.gpuIn.SetText(fmt.Sprintf("%d", d.cfg.LLM.NumGPU))
	rv.ctxIn = widget.NewEntry()
	rv.ctxIn.SetText(fmt.Sprintf("%d", d.cfg.LLM.NumCtx))
	rv.depthIn = widget.NewSelect([]string{"1 (fastest)", "2", "3 (balanced)", "4", "5 (deepest)"}, nil)
	switch d.cfg.EffectiveMultiAgentDepth() {
	case 1:
		rv.depthIn.SetSelectedIndex(0)
	case 2:
		rv.depthIn.SetSelectedIndex(1)
	case 3:
		rv.depthIn.SetSelectedIndex(2)
	case 4:
		rv.depthIn.SetSelectedIndex(3)
	case 5:
		rv.depthIn.SetSelectedIndex(4)
	}
	applyBtn := primaryButton("Apply allocation & restart engine if running", "bolt", func() { rv.apply() })

	alloc := container.NewVBox(
		container.NewPadded(canvasText("ENGINE ALLOCATION — SHARED BY EVERY AGENT", ColTextMuted, 11, true)),
		container.NewGridWithColumns(2,
			labeledEntry("Generation threads (physical cores)", rv.threadsIn),
			labeledEntry("GPU layers (0 = auto-detected)", rv.gpuIn),
			labeledEntry("Context window (tokens)", rv.ctxIn),
			labeledBox("Multi-agent pipeline depth", rv.depthIn),
		),
		container.NewPadded(applyBtn),
	)

	// --- budgets + cleanup section ---
	rv.workQuota = widget.NewEntry()
	rv.workQuota.SetText(fmt.Sprintf("%d", d.cfg.EffectiveMaxWorkspaceMB()))
	rv.sessKeep = widget.NewEntry()
	rv.sessKeep.SetText(fmt.Sprintf("%d", d.cfg.EffectiveMaxSessionsKept()))
	rv.logCap = widget.NewEntry()
	rv.logCap.SetText(fmt.Sprintf("%d", d.cfg.EffectiveMaxLogMB()))

	saveBudgets := primaryButton("Save budgets", "check", func() { rv.saveBudgets() })
	trimSessions := secondaryBtn("Trim old sessions", "delete", func() { rv.trimSessions() })
	trimLogs := secondaryBtn("Rotate logs to budget", "refresh", func() { rv.trimLogs() })
	clearWorkspace := secondaryBtn("Clear workspace", "folder", func() { rv.clearWorkspace() })

	budgets := container.NewVBox(
		container.NewPadded(canvasText("AGENT BUDGETS & CLEANUP", ColTextMuted, 11, true)),
		container.NewGridWithColumns(3,
			labeledEntry("Workspace cap (MB; 0 = no limit)", rv.workQuota),
			labeledEntry("Keep newest sessions", rv.sessKeep),
			labeledEntry("Log budget (MB)", rv.logCap),
		),
		container.NewPadded(saveBudgets),
		container.NewPadded(container.NewHBox(trimSessions, trimLogs, clearWorkspace)),
	)

	content := container.NewVBox(
		container.NewPadded(rv.header),
		live,
		widget.NewSeparator(),
		alloc,
		widget.NewSeparator(),
		budgets,
	)
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(10, 420))

	rv.obj = container.NewBorder(
		container.NewPadded(header),
		nil, nil, nil,
		container.NewPadded(scroll),
	)
	d.resView = rv
	rv.refresh()
	return rv.obj
}

func canvasText(s string, c color.Color, size float32, bold bool) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = size
	t.TextStyle.Bold = bold
	return t
}

func labeledEntry(label string, e *widget.Entry) fyne.CanvasObject {
	lbl := widget.NewLabel(label)
	lbl.TextStyle = fyne.TextStyle{Bold: false}
	return container.NewVBox(lbl, e)
}

func labeledBox(label string, b *widget.Select) fyne.CanvasObject {
	lbl := widget.NewLabel(label)
	return container.NewVBox(lbl, b)
}

// secondaryBtn is a subdued action button (cleanup cluster).
func secondaryBtn(label, iconName string, onTap func()) *widget.Button {
	b := widget.NewButtonWithIcon(label, icon(iconName), onTap)
	b.Importance = widget.LowImportance
	return b
}

// refresh rescans disk + live process memory. Synchronous by design: the
// folder walk is fast (same discipline as the Models view storage line) and
// synchronous updates can never race the renderer mid-measure.
func (rv *resourcesView) refresh() {
	d := rv.app
	usage := resources.Scan(d.cfg.DataDir)
	var total int64
	for _, u := range usage {
		total += u.Bytes
	}
	engineRAM := int64(0)
	if d.llama != nil {
		if pid := d.llama.Pid(); pid > 0 {
			if ram, err := resources.ProcRAM(pid); err == nil {
				engineRAM = ram
			}
		}
	}
	appRAM := int64(0)
	if ram, err := resources.ProcRAM(os.Getpid()); err == nil {
		appRAM = ram
	}
	// RAM bars are proportional to the machine's physical memory (probed
	// once per session, cached in sysinfo) — an honest visual scale.
	maxRAM := int64(0)
	if info := sysinfo.Probe(); info != nil && info.RAM.TotalBytes > 0 {
		maxRAM = int64(info.RAM.TotalBytes)
	}
	rv.header.Text = fmt.Sprintf("App folder: %s total · %s engine · %s SHEYTAN — rescanned %s",
		resources.HumanBytes(total), resources.HumanBytes(engineRAM), resources.HumanBytes(appRAM),
		time.Now().Format("15:04:05"))
	rv.header.Refresh()
	rv.engine.Set("llama.cpp engine (RAM)", engineRAM, maxRAM)
	rv.appRAM.Set("SHEYTAN app (RAM)", appRAM, maxRAM)
	rv.storage.RemoveAll()
	max := int64(1)
	for _, u := range usage {
		if u.Bytes > max {
			max = u.Bytes
		}
	}
	for _, u := range usage {
		bar := newUsageBar()
		bar.Set(u.Name, u.Bytes, max)
		rv.storage.Add(bar.row)
	}
	rv.storage.Refresh()
}

// apply persists the engine allocation and restarts the engine if running.
func (rv *resourcesView) apply() {
	d := rv.app
	if n, err := parseInt(rv.threadsIn.Text); err == nil && n > 0 {
		d.cfg.LLM.NumThread = n
	}
	if n, err := parseInt(rv.gpuIn.Text); err == nil && n >= 0 {
		d.cfg.LLM.NumGPU = n
	}
	if n, err := parseInt(rv.ctxIn.Text); err == nil && n >= 512 {
		d.cfg.LLM.NumCtx = n
	}
	if rv.depthIn.SelectedIndex() >= 0 {
		// options: 1..5 mapped by index (labels carry the number)
		d.cfg.MultiAgentDepth = rv.depthIn.SelectedIndex() + 1
	}
	if err := d.saveCfg(); err != nil {
		d.setStatus("Could not save allocation: " + clipStrMemory(err.Error(), 40))
		return
	}
	d.setStatus("Allocation saved — takes effect on the next engine start")
	if d.llama != nil && d.llama.IsRunning() {
		d.setStatus("Restarting engine with the new allocation…")
		go func() {
			if err := d.llama.Restart(); err != nil {
				runOnMain(func() { d.setStatus("Engine restart failed: " + clipStrMemory(err.Error(), 50)) })
				return
			}
			runOnMain(func() { d.setStatus("Engine restarted with the new allocation") })
		}()
	}
}

// saveBudgets persists the cleanup budgets.
func (rv *resourcesView) saveBudgets() {
	d := rv.app
	if n, err := parseInt(rv.workQuota.Text); err == nil && n >= 0 {
		d.cfg.MaxWorkspaceMB = n
	}
	if n, err := parseInt(rv.sessKeep.Text); err == nil && n >= 0 {
		d.cfg.MaxSessionsKept = n
	}
	if n, err := parseInt(rv.logCap.Text); err == nil && n >= 0 {
		d.cfg.MaxLogMB = n
	}
	if err := d.saveCfg(); err != nil {
		d.setStatus("Could not save budgets: " + clipStrMemory(err.Error(), 40))
		return
	}
	d.setStatus("Budgets saved")
}

// trimSessions deletes sessions beyond the newest N (with confirmation).
func (rv *resourcesView) trimSessions() {
	d := rv.app
	keep := d.cfg.EffectiveMaxSessionsKept()
	if keep <= 0 {
		d.setStatus("Set 'Keep newest sessions' to a number first")
		return
	}
	dialog.ShowConfirm("Trim old sessions",
		fmt.Sprintf("Delete every session except the newest %d? This cannot be undone.", keep),
		func(ok bool) {
			if !ok {
				return
			}
			removed := resources.TrimSessions(keep,
				func() ([]string, error) {
					list, err := d.store.List()
					if err != nil {
						return nil, err
					}
					ids := make([]string, 0, len(list))
					for _, s := range list {
						ids = append(ids, s.ID)
					}
					return ids, nil
				},
				func(id string) error { return d.store.Delete(id) },
			)
			d.reloadSessions()
			if removed > 0 {
				d.setStatus(fmt.Sprintf("Trimmed %d old session(s)", removed))
			} else {
				d.setStatus("Nothing to trim — under the budget")
			}
			rv.refresh()
		}, d.win)
}

// trimLogs rotates the log folder down to the budget.
func (rv *resourcesView) trimLogs() {
	d := rv.app
	budget := int64(d.cfg.EffectiveMaxLogMB())
	if budget <= 0 {
		d.setStatus("Set a log budget (MB) first")
		return
	}
	freed := resources.TrimLogs(d.cfg.LogsDir(), budget)
	d.refreshLogs()
	d.setStatus(fmt.Sprintf("Logs rotated — %s freed", resources.HumanBytes(freed)))
	rv.refresh()
}

// clearWorkspace empties the agent workspace (with confirmation).
func (rv *resourcesView) clearWorkspace() {
	d := rv.app
	dialog.ShowConfirm("Clear workspace",
		"Delete everything inside the agent's workspace folder? Files the agent created (reports, CSVs, charts) will be gone.",
		func(ok bool) {
			if !ok {
				return
			}
			n, err := resources.ClearDir(d.cfg.WorkspaceDir())
			if err != nil {
				d.setStatus("Clear failed: " + err.Error())
				return
			}
			if d.tracker != nil {
				d.refreshFiles()
			}
			d.setStatus(fmt.Sprintf("Workspace cleared (%d items)", n))
			rv.refresh()
		}, d.win)
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

// saveCfg writes the config through the app's canonical path.
func (d *desktopApp) saveCfg() error {
	return config.Save(d.cfg.ConfigPath(), d.cfg)
}
