package ui

// views_files.go — v1.0.4 "Files & Artifacts" + "Models" views, the speed
// settings dialog, in-chat created-file chips, and the inline file preview.
//
// The promise: every file the agent creates shows up IN THE APP the moment
// it exists, with big type icons and one-tap actions — Preview (in-app),
// Open (system default app), Reveal (Explorer, pre-selected), Copy Path.
//
// The Models view is the LM Studio-competitor surface: real model cards
// parsed from the GGUF headers (params, quant, context length), memory-fit
// guidance against detected VRAM/RAM, folder-size management, and the
// research-backed Speed Pack switches.

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/sheytan/local-agent/internal/artifacts"
	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/llm"
	"github.com/sheytan/local-agent/internal/logging"
	"github.com/sheytan/local-agent/internal/sysinfo"
)

// newArtifactTracker builds the tracker for the app's portable folders.
func newArtifactTracker(cfg *config.Config) *artifacts.Tracker {
	return artifacts.New([]string{
		cfg.WorkspaceDir(),
		cfg.ChartsDir(),
		cfg.ScreenshotsDir(),
		filepath.Join(cfg.DataDir, "diagnostics"),
	})
}

// --- Files (artifacts) view ---

// buildFilesView renders the artifacts gallery: every file the agent has
// created in workspace/, charts/, screenshots/, diagnostics/.
func (d *desktopApp) buildFilesView() fyne.CanvasObject {
	d.fileList = widget.NewList(
		func() int { return len(d.fileArtifacts) },
		func() fyne.CanvasObject { return newArtifactRow() },
		func(idx widget.ListItemID, item fyne.CanvasObject) {
			if idx < 0 || idx >= len(d.fileArtifacts) {
				return
			}
			row := item.(*artifactRow)
			a := d.fileArtifacts[idx]
			row.Set(a)
			row.actions = d.artifactActions(a)
		},
	)

	d.fileEmpty = widget.NewLabel("No files yet.\n\nEverything the agent creates — reports, charts, scripts,\ndownloads, diagnostics — lands here automatically\nwith preview, open, reveal, and copy-path actions.")
	d.fileEmpty.TextStyle = fyne.TextStyle{Italic: true}

	refresh := primaryButton("Refresh", "refresh", d.refreshFiles)
	openWs := ghostButton("Open workspace", "folder", func() { d.openInExplorer(d.cfg.WorkspaceDir()) })
	hint := widget.NewLabel("Files the agent creates appear here instantly.")
	hint.TextStyle = fyne.TextStyle{Italic: true}

	return container.NewBorder(
		container.NewVBox(
			container.NewPadded(sectionHeader("files", "Files")),
			container.NewPadded(container.NewHScroll(container.NewHBox(refresh, openWs, hint))),
		),
		nil, nil, nil,
		container.NewStack(container.NewCenter(d.fileEmpty), d.fileList),
	)
}

// artifactActions wires one artifact's action row to the desktop helpers.
func (d *desktopApp) artifactActions(a artifacts.Artifact) artifactActions {
	return artifactActions{
		onPreview: func() { d.previewArtifact(a) },
		onOpen:    func() { d.openWithDefaultApp(a.Path) },
		onReveal:  func() { d.openInExplorer(a.Path) },
		onCopy:    func() { d.copyPathToClipboard(a.Path) },
	}
}

// refreshFiles rescans the watched directories.
func (d *desktopApp) refreshFiles() {
	if d.tracker == nil {
		return
	}
	d.fileArtifacts = d.tracker.Scan(200)
	if d.fileList != nil {
		d.fileList.Refresh()
	}
	if d.fileEmpty != nil {
		if len(d.fileArtifacts) == 0 {
			d.fileEmpty.Show()
		} else {
			d.fileEmpty.Hide()
		}
	}
}

// previewArtifact shows a file inside the app: images/SVG render, text and
// code stream in a monospace reader, everything else gets an info card.
func (d *desktopApp) previewArtifact(a artifacts.Artifact) {
	// Images + SVG: render.
	switch strings.ToLower(filepath.Ext(a.Path)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".svg":
		if res, err := fyne.LoadResourceFromPath(a.Path); err == nil {
			img := canvas.NewImageFromResource(res)
			img.FillMode = canvas.ImageFillContain
			meta := widget.NewLabel(fmt.Sprintf("%s · %s · %s", a.Name, llm.FormatBytes(a.Size), humanize(a.ModTime)))
			meta.Importance = widget.LowImportance
			open := primaryButton("Open with default app", "open", func() { d.openWithDefaultApp(a.Path) })
			reveal := ghostButton("Reveal in Explorer", "folder", func() { d.openInExplorer(a.Path) })
			copyBtn := ghostButton("Copy path", "copy", func() { d.copyPathToClipboard(a.Path) })
			dlg := dialog.NewCustom("Preview — "+clipStrMemory(a.Name, 40), "Close",
				container.NewBorder(nil,
					container.NewVBox(meta, container.NewHBox(open, reveal, copyBtn)),
					nil, nil,
					container.NewScroll(img)), d.win)
			dlg.Resize(fyne.NewSize(720, 520))
			dlg.Show()
			return
		}
	}

	// Text-ish: read up to 512 KB.
	if isPreviewableText(a.Path) && a.Size <= 512*1024 {
		if data, err := os.ReadFile(a.Path); err == nil {
			txt := widget.NewRichTextFromMarkdown("```\n" + string(data) + "\n```")
			txt.Wrapping = fyne.TextWrapOff
			meta := widget.NewLabel(fmt.Sprintf("%s · %s · %s", a.Name, llm.FormatBytes(a.Size), humanize(a.ModTime)))
			meta.Importance = widget.LowImportance
			open := primaryButton("Open with default app", "open", func() { d.openWithDefaultApp(a.Path) })
			copyBtn := ghostButton("Copy path", "copy", func() { d.copyPathToClipboard(a.Path) })
			dlg := dialog.NewCustom("Preview — "+clipStrMemory(a.Name, 40), "Close",
				container.NewBorder(nil,
					container.NewVBox(meta, container.NewHBox(open, copyBtn)),
					nil, nil,
					container.NewScroll(txt)), d.win)
			dlg.Resize(fyne.NewSize(720, 520))
			dlg.Show()
			return
		}
	}

	// Binary / other: info card with every action.
	info := widget.NewLabel(fmt.Sprintf(
		"Name: %s\nSize: %s\nCreated: %s\nFolder: %s\n\nNo in-app preview for this file type —\nopen it with the default application or reveal it in Explorer.",
		a.Name, llm.FormatBytes(a.Size), humanize(a.ModTime), a.Dir))
	info.Wrapping = fyne.TextWrapWord
	open := primaryButton("Open with default app", "open", func() { d.openWithDefaultApp(a.Path) })
	reveal := ghostButton("Reveal in Explorer", "folder", func() { d.openInExplorer(a.Path) })
	copyBtn := ghostButton("Copy path", "copy", func() { d.copyPathToClipboard(a.Path) })
	d.bigDialog("File — "+clipStrMemory(a.Name, 40), container.NewVBox(container.NewPadded(info),
		container.NewHBox(open, reveal, copyBtn)), 560, 380)
}

// isPreviewableText lists extensions the inline reader can show.
func isPreviewableText(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md", ".markdown", ".csv", ".tsv", ".json", ".jsonl", ".log",
		".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".env", ".xml",
		".html", ".htm", ".css", ".js", ".ts", ".go", ".py", ".rs", ".java",
		".rb", ".php", ".sh", ".ps1", ".bat", ".sql", ".diff", ".patch":
		return true
	}
	return false
}

// appendArtifactChips renders the compact "Created files" row under a chat
// turn: big-icon chips, each opening the full action sheet.
func (d *desktopApp) appendArtifactChips(arts []artifacts.Artifact) {
	if len(arts) == 0 {
		return
	}
	cap := 4
	shown := arts
	more := 0
	if len(arts) > cap {
		shown = arts[:cap]
		more = len(arts) - cap
	}

	title := canvas.NewText(fmt.Sprintf("Created %d file(s)", len(arts)), ColGold)
	title.TextSize = 12
	title.TextStyle.Bold = true

	chips := container.NewHBox()
	for _, a := range shown {
		a := a // capture
		chip := newArtifactChip(a, func() { d.artifactSheet(a) })
		chips.Add(container.NewPadded(chip))
	}
	if more > 0 {
		moreBtn := ghostButton(fmt.Sprintf("+%d more", more), "files", func() { d.showView("files") })
		chips.Add(container.NewPadded(moreBtn))
	}

	row := panel(container.NewVBox(title, chips), 8, 6)
	revealIn(row, d.chatBox, ColBg, 160*time.Millisecond)
	d.chatBox.Add(row)
	d.chatBox.Refresh()
	d.chatScroll.ScrollToBottom()
}

// artifactSheet is the one-tap action sheet for one created file.
func (d *desktopApp) artifactSheet(a artifacts.Artifact) {
	head := widget.NewLabelWithStyle(a.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	head.Truncation = fyne.TextTruncateClip
	meta := widget.NewLabel(fmt.Sprintf("%s · %s · %s", llm.FormatBytes(a.Size), humanize(a.ModTime), clipStrMemory(a.Dir, 52)))
	meta.Importance = widget.LowImportance
	meta.Truncation = fyne.TextTruncateClip

	preview := primaryButton("Preview in app", "eye", func() { d.previewArtifact(a) })
	open := ghostButton("Open (default app)", "open", func() { d.openWithDefaultApp(a.Path) })
	reveal := ghostButton("Reveal in Explorer", "folder", func() { d.openInExplorer(a.Path) })
	copyBtn := ghostButton("Copy path", "copy", func() { d.copyPathToClipboard(a.Path) })

	d.bigDialog("Created file", container.NewVBox(
		container.NewPadded(container.NewVBox(head, meta)),
		widget.NewSeparator(),
		container.NewVBox(preview, open, reveal, copyBtn),
	), 520, 440)
}

// --- Models view (model manager + storage + speed pack) ---

// modelEntry is one local model row in the manager.
type modelEntry struct {
	Card *llm.ModelCard // nil when the GGUF header could not be parsed
	Name string
	Path string
	Size int64
}

// buildModelsView renders the model manager: every local .gguf with its
// real card (params · quant · context · size), memory-fit guidance, the
// folder's total size, engine info, and the Speed Pack switches.
func (d *desktopApp) buildModelsView() fyne.CanvasObject {
	d.modelsBox = container.NewVBox()
	d.modelsScroll = container.NewVScroll(d.modelsBox)
	d.modelsEmpty = widget.NewLabel("No models installed.\n\nDrop .gguf files into the models folder —\nthey appear here the moment you refresh.")
	d.modelsEmpty.TextStyle = fyne.TextStyle{Italic: true}

	refresh := primaryButton("Refresh", "refresh", d.refreshModels)
	openDir := ghostButton("Open models folder", "folder", func() { d.openInExplorer(d.cfg.ModelsDir) })
	speed := ghostButton("Speed settings…", "bolt", d.showSpeedDialog)
	hint := widget.NewLabel("Local models · memory fit · speed")
	hint.TextStyle = fyne.TextStyle{Italic: true}

	d.renderModels()

	return container.NewBorder(
		container.NewVBox(
			container.NewPadded(sectionHeader("model", "Models")),
			container.NewPadded(container.NewHScroll(container.NewHBox(refresh, openDir, speed, hint))),
		),
		nil, nil, nil,
		container.NewStack(container.NewCenter(d.modelsEmpty), container.NewPadded(d.modelsScroll)),
	)
}

// refreshModels + renderModels rebuild the model card list.
func (d *desktopApp) refreshModels() {
	d.renderModels()
	if d.modelsScroll != nil {
		d.modelsScroll.Refresh()
	}
}

func (d *desktopApp) renderModels() {
	if d.modelsBox == nil {
		return
	}
	d.modelsBox.RemoveAll()

	entries := d.localModelEntries()
	sys := sysinfo.Probe()

	// Folder header: count + total size.
	var total int64
	for _, e := range entries {
		total += e.Size
	}
	header := widget.NewLabel(fmt.Sprintf("%d model(s) · %s total", len(entries), llm.FormatBytes(total)))
	header.TextStyle = fyne.TextStyle{Bold: true}
	d.modelsBox.Add(panel(container.NewVBox(header, d.storageLine()), 10, 6))

	if len(entries) == 0 {
		d.modelsEmpty.Show()
	} else {
		d.modelsEmpty.Hide()
	}

	for _, e := range entries {
		e := e
		current := strings.EqualFold(e.Name, d.cfg.Model)
		card := d.modelCardWidget(e, current, sys)
		d.modelsBox.Add(panel(card, 8, 8))
	}
	d.modelsBox.Refresh()
}

// localModelEntries lists the models folder with parsed GGUF cards.
func (d *desktopApp) localModelEntries() []modelEntry {
	var out []modelEntry
	for _, name := range llm.ListLocalModels(d.cfg.ModelsDir) {
		path := filepath.Join(d.cfg.ModelsDir, name)
		entry := modelEntry{Name: name, Path: path}
		if fi, err := os.Stat(path); err == nil {
			entry.Size = fi.Size()
		}
		if card, err := llm.ReadModelCard(path); err == nil {
			entry.Card = card
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// modelCardWidget builds one model's card row.
func (d *desktopApp) modelCardWidget(e modelEntry, current bool, sys *sysinfo.SysInfo) fyne.CanvasObject {
	name := strings.TrimSuffix(e.Name, ".gguf")
	title := widget.NewLabel(name)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Truncation = fyne.TextTruncateClip

	meta := e.Name
	if e.Card != nil {
		meta = e.Card.Meta()
	} else {
		meta = llm.FormatBytes(e.Size)
	}
	metaLbl := widget.NewLabel(meta)
	metaLbl.Importance = widget.LowImportance
	metaLbl.Truncation = fyne.TextTruncateClip

	// Fit guidance (data management, LM Studio competitor).
	fit := fitGuidance(e, sys)
	var fitLbl *widget.Label
	if fit != "" {
		fitLbl = widget.NewLabel(fit)
		fitLbl.TextStyle = fyne.TextStyle{Italic: true}
		fitLbl.Truncation = fyne.TextTruncateClip
	}

	var head fyne.CanvasObject = container.NewVBox(title, metaLbl)
	if fitLbl != nil {
		head = container.NewVBox(title, metaLbl, fitLbl)
	}

	// Actions: select / reveal / copy / delete.
	useBtn := primaryButton("Use", "check", func() { d.applyModel(e.Name) })
	if current {
		useBtn = ghostButton("Active", "check", nil)
		useBtn.Disable()
	}
	revealBtn := ghostButton("", "folder", func() { d.openInExplorer(e.Path) })
	copyBtn := ghostButton("", "copy", func() { d.copyPathToClipboard(e.Path) })
	delBtn := ghostButton("", "delete", func() { d.confirmDeleteModel(e) })
	delBtn.SetDanger()

	return container.NewBorder(nil, nil, nil,
		container.NewVBox(useBtn,
			container.NewHBox(revealBtn, copyBtn, delBtn)),
		head)
}

// fitGuidance compares model size against detected VRAM/RAM.
func fitGuidance(e modelEntry, sys *sysinfo.SysInfo) string {
	if e.Size <= 0 || sys == nil {
		return ""
	}
	var bestVRAM uint64
	for _, g := range sys.GPU {
		if g.VRAMBytes > bestVRAM {
			bestVRAM = g.VRAMBytes
		}
	}
	ram := sys.RAM.TotalBytes
	switch {
	case bestVRAM > 0 && uint64(e.Size) <= bestVRAM*8/10:
		return "Fits GPU VRAM — full GPU speed"
	case bestVRAM > 0 && uint64(e.Size) <= bestVRAM:
		return "Fits GPU VRAM tightly — consider a smaller quant or q8_0 KV cache"
	case ram > 0 && uint64(e.Size) <= ram*6/10:
		return "Larger than VRAM — GPU/RAM split or CPU inference"
	case ram > 0:
		return "Larger than RAM — too big for this machine"
	default:
		return ""
	}
}

// confirmDeleteModel asks before removing a model file.
func (d *desktopApp) confirmDeleteModel(e modelEntry) {
	dialog.ShowConfirm("Delete model?",
		fmt.Sprintf("Delete %s (%s) from disk?\nThis cannot be undone.", e.Name, llm.FormatBytes(e.Size)),
		func(ok bool) {
			if !ok {
				return
			}
			if err := os.Remove(e.Path); err != nil {
				dialog.ShowError(err, d.win)
				return
			}
			logging.Default().Info("gui", "model deleted: %s", e.Path)
			// Snap selection to another model if the deleted one was active.
			if strings.EqualFold(e.Name, d.cfg.Model) {
				d.cfg.Model = ""
				if models := llm.ListLocalModels(d.cfg.ModelsDir); len(models) > 0 {
					d.cfg.Model = models[0]
				}
				_ = config.Save(d.cfg.ConfigPath(), d.cfg)
				if d.modelChip != nil {
					d.modelChip.SetText(clipStrMemory(d.cfg.DisplayModel(), 24))
				}
			}
			d.refreshModels()
			d.setStatus("Model deleted: " + e.Name)
		}, d.win)
}

// storageLine renders the app-folder storage breakdown (data management).
func (d *desktopApp) storageLine() fyne.CanvasObject {
	sizes := map[string]int64{
		"models":    dirSize(d.cfg.ModelsDir),
		"sessions":  dirSize(d.cfg.SessionsDir),
		"logs":      dirSize(d.cfg.LogsDir()),
		"charts":    dirSize(d.cfg.ChartsDir()),
		"workspace": dirSize(d.cfg.WorkspaceDir()),
	}
	var total int64
	for _, v := range sizes {
		total += v
	}
	parts := []string{}
	for _, k := range []string{"models", "sessions", "workspace", "charts", "logs"} {
		parts = append(parts, fmt.Sprintf("%s %s", k, llm.FormatBytes(sizes[k])))
	}
	line := widget.NewLabel("Storage: " + strings.Join(parts, " · ") + fmt.Sprintf(" — total %s", llm.FormatBytes(total)))
	line.Importance = widget.LowImportance
	line.Truncation = fyne.TextTruncateClip
	return line
}

func dirSize(dir string) int64 {
	var total int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil {
			total += info.Size()
		}
	}
	return total
}

// --- Speed Pack dialog ---

// showSpeedDialog exposes the v1.0.4 engine speed switches.
func (d *desktopApp) showSpeedDialog() {
	flash := widget.NewCheck("Flash Attention kernels (recommended, on by default)", nil)
	flash.SetChecked(d.cfg.FlashAttention)

	kvOptions := widget.NewSelect([]string{"off (default, safest)", "q8_0 — halve KV memory", "q4_0 — quarter KV memory"}, nil)
	switch d.cfg.EffectiveKVCacheQuant() {
	case "q8_0":
		kvOptions.SetSelected("q8_0 — halve KV memory")
	case "q4_0":
		kvOptions.SetSelected("q4_0 — quarter KV memory")
	default:
		kvOptions.SetSelected("off (default, safest)")
	}

	cacheReuse := widget.NewSelect([]string{"on — reuse prefix across turns (recommended)", "off"}, nil)
	if d.cfg.EffectiveCacheReuse() > 0 {
		cacheReuse.SetSelected("on — reuse prefix across turns (recommended)")
	} else {
		cacheReuse.SetSelected("off")
	}

	mlock := widget.NewCheck("Lock model in RAM (needs free memory beyond the model)", nil)
	mlock.SetChecked(d.cfg.Mlock)

	perfHUD := widget.NewCheck("Show speed HUD (tokens/sec after each reply)", nil)
	perfHUD.SetChecked(d.cfg.ShowPerfHUD)

	// Draft model: small local models only (a draft must be much smaller
	// than the main model to help).
	draftNames := []string{"(none)"}
	for _, name := range llm.ListLocalModels(d.cfg.ModelsDir) {
		if fi, err := os.Stat(filepath.Join(d.cfg.ModelsDir, name)); err == nil && fi.Size() < 2<<30 {
			draftNames = append(draftNames, name)
		}
	}
	draft := widget.NewSelect(draftNames, nil)
	if d.cfg.DraftModel != "" {
		draft.SetSelected(d.cfg.DraftModel)
	} else {
		draft.SetSelected("(none)")
	}

	info := widget.NewLabel(
		"Speed Pack — research-backed llama.cpp tuning (Aug 2026):\n" +
			"· prefix cache reuse collapses the repeated agent prompt\n" +
			"· flash attention raises throughput on every backend\n" +
			"· a small draft model adds 20-50% tokens/sec (speculative decoding)\n" +
			"· q8_0 KV cache halves context memory at <5% speed cost")
	info.Wrapping = fyne.TextWrapWord
	info.Importance = widget.LowImportance

	form := widget.NewForm(
		widget.NewFormItem("Flash Attention", flash),
		widget.NewFormItem("Prompt cache reuse", cacheReuse),
		widget.NewFormItem("KV cache", kvOptions),
		widget.NewFormItem("Draft model", draft),
		widget.NewFormItem("Memory", mlock),
		widget.NewFormItem("Telemetry", perfHUD),
	)

	var dlg *dialog.CustomDialog
	save := primaryButton("Apply & restart engine if running", "bolt", func() {
		d.cfg.FlashAttention = flash.Checked
		d.cfg.Mlock = mlock.Checked
		d.cfg.ShowPerfHUD = perfHUD.Checked
		switch kvOptions.Selected {
		case "q8_0 — halve KV memory":
			d.cfg.KVCacheQuant = "q8_0"
		case "q4_0 — quarter KV memory":
			d.cfg.KVCacheQuant = "q4_0"
		default:
			d.cfg.KVCacheQuant = ""
		}
		if strings.HasPrefix(cacheReuse.Selected, "on") {
			d.cfg.CacheReuse = 32
		} else {
			d.cfg.CacheReuse = 0
		}
		if draft.Selected == "(none)" || draft.Selected == "" {
			d.cfg.DraftModel = ""
		} else {
			d.cfg.DraftModel = draft.Selected
		}
		// v1.0.5: a fresh engine start after speed-settings changes
		// retests the fastest profile — a stale compat level must
		// never stick around after the user deliberately changed the
		// launch flags.
		d.cfg.EngineCompat = 0
		_ = config.Save(d.cfg.ConfigPath(), d.cfg)
		logging.Default().Info("gui", "speed pack updated (fa=%v kv=%q reuse=%d draft=%q mlock=%v)",
			d.cfg.FlashAttention, d.cfg.KVCacheQuant, d.cfg.CacheReuse, d.cfg.DraftModel, d.cfg.Mlock)
		if dlg != nil {
			dlg.Hide()
		}
		// Restart the engine so the flags take effect immediately.
		if !d.cfg.IsRemote() && d.llama != nil && d.llama.IsRunning() {
			d.setStatus("Restarting engine with new speed settings…")
			go func() {
				if err := d.llama.Restart(); err != nil {
					runOnMain(func() { d.setStatus("Engine restart failed: " + clipStrMemory(err.Error(), 50)) })
					return
				}
				runOnMain(func() { d.setStatus("Speed settings applied — engine ready") })
			}()
		} else {
			d.setStatus("Speed settings saved (apply on next engine start)")
		}
	})

	dlg = dialog.NewCustom("Engine speed", "Close",
		container.NewVScroll(container.NewVBox(info, form,
			container.NewHBox(save))), d.win)
	dlg.Resize(fyne.NewSize(560, 560))
	dlg.Show()
}

// colTextDim is a muted text color alias (avoids an import cycle in this
// file's helper funcs that predate the theme split).
var colTextDim = color.NRGBA{R: 0xB8, G: 0xA9, B: 0xA2, A: 0xFF}
