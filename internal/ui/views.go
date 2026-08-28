// Package ui — v1.0.0 main views: Chat (hero empty state + composer), Data
// (charts), Memory, Logs, and the Pro dock tabs (Context / Params / System /
// Tools).
package ui

import (
        "fmt"
        "image/color"
        "os"
        "path/filepath"
        "runtime"
        "sort"
        "strings"
        "time"

        "fyne.io/fyne/v2"
        "fyne.io/fyne/v2/canvas"
        "fyne.io/fyne/v2/container"
        "fyne.io/fyne/v2/dialog"
        "fyne.io/fyne/v2/layout"
        "fyne.io/fyne/v2/widget"

        "github.com/sheytan/local-agent/internal/agent"
        "github.com/sheytan/local-agent/internal/config"
        "github.com/sheytan/local-agent/internal/continuum"
        "github.com/sheytan/local-agent/internal/llm"
        "github.com/sheytan/local-agent/internal/logging"
        "github.com/sheytan/local-agent/internal/recall"
        "github.com/sheytan/local-agent/internal/sessions"
        "github.com/sheytan/local-agent/internal/sysinfo"
)

// chatColumnMax is the width of the centered conversation column (the
// chat.z.ai reading measure — wide enough for code blocks, narrow enough to
// keep prose comfortable).
const chatColumnMax float32 = 780

// maxWidthLayout centers its single child horizontally at
// min(child, max) — emulates the chat.z.ai content measure.
type maxWidthLayout struct{ max float32 }

func (m maxWidthLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
        for _, o := range objs {
                w := size.Width
                if w > m.max {
                        w = m.max
                }
                o.Resize(fyne.NewSize(w, size.Height))
                o.Move(fyne.NewPos((size.Width-w)/2, 0))
        }
}

func (m maxWidthLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
        var min fyne.Size
        for _, o := range objs {
                ms := o.MinSize()
                if ms.Width > min.Width {
                        min.Width = ms.Width
                }
                if ms.Height > min.Height {
                        min.Height = ms.Height
                }
        }
        if min.Width > m.max {
                min.Width = m.max
        }
        return min
}

// chatRowLayout sizes a message bubble by a FRACTION of the available row
// width (never by the bubble's MinSize). This matters because a RichText's
// MinSize is dynamic — it reflects the width it was last given — so an HBox
// + spacer row lets a freshly-laid-out bubble collapse to a degenerate
// ~60px column that then self-reinforces (a real v1.0.0 pixel-scan finding).
// User bubbles are right-aligned at 86%; assistant text spans the measure.
type chatRowLayout struct {
        right bool
        frac  float32
}

func (l chatRowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
        if len(objs) == 0 {
                return
        }
        o := objs[0]
        w := size.Width * l.frac
        if w < 1 {
                w = size.Width
        }
        h := o.MinSize().Height
        if h < size.Height {
                h = size.Height
        }
        o.Resize(fyne.NewSize(w, h))
        x := float32(0)
        if l.right {
                x = size.Width - w
        }
        o.Move(fyne.NewPos(x, 0))
}

func (l chatRowLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
        if len(objs) == 0 {
                return fyne.NewSize(0, 0)
        }
        // Rows claim only a modest width so the column can shrink on small
        // windows; height comes from the bubble's current (wrapped) min height.
        return fyne.NewSize(160, objs[0].MinSize().Height)
}

// --- Chat view ---

// buildChatView assembles the chat view. v1.0.0 "Ember Minimal" + v1.0.2:
//   - empty state is a hero: flame, greeting, four suggestion cards, and a
//     first-run card when no model is installed yet
//   - messages live in a centered max-width column; user messages are tinted
//     bubbles right-aligned, assistant replies are clean full-width text
//     with an optional collapsible "Thought process" section
//   - ONE status line summarizes background processing; the full activity
//     stream only appears in Pro mode
//   - the composer is a rounded panel with a circular send button, a 📎
//     attach control, a tools selector, and a thinking-mode toggle
func (d *desktopApp) buildChatView() fyne.CanvasObject {
        // Empty-state hero.
        d.chatEmpty = d.buildEmptyHero()

        // Message stream: bubbles in a vertical scroll inside the measure column.
        d.chatBox = container.NewVBox()
        measure := container.New(&maxWidthLayout{max: chatColumnMax}, d.chatBox)
        d.chatScroll = container.NewScroll(container.NewStack(d.chatEmpty, measure))

        // --- Live status line (simple mode summary) ---
        d.flame = canvas.NewImageFromResource(Logo)
        d.flame.FillMode = canvas.ImageFillContain
        d.flame.SetMinSize(fyne.NewSize(16, 16))
        d.flame.Hide()

        d.dots = newTypingDots()
        d.dots.obj.Hide()

        d.activityLbl = widget.NewLabel("")
        d.activityLbl.TextStyle = fyne.TextStyle{Italic: true}
        d.activityLbl.Truncation = fyne.TextTruncateClip

        d.abortBtn = widget.NewButtonWithIcon("Stop", icon("stop"), d.abortRun)
        d.abortBtn.Importance = widget.DangerImportance
        d.abortBtn.Hide()

        var breathStop func()
        d.breathLine, breathStop = emberLine(240)
        d.breathStop = breathStop
        d.breathLine.Hide()

        strip := container.NewBorder(
                nil, nil,
                container.NewHBox(d.flame, d.dots.obj),
                container.NewPadded(d.abortBtn),
                container.NewPadded(d.activityLbl),
        )

        // --- Activity log (pro mode detail) ---
        d.activityBox = container.NewVBox()
        d.activityScroll = container.NewVScroll(d.activityBox)
        d.activityScroll.SetMinSize(fyne.NewSize(10, 116))
        d.activitySection = container.NewVBox(
                strip,
                d.breathLine,
                d.activityScroll,
        )
        d.activitySection.Hide()
        if d.cfg.ProMode {
                // section itself only shows once activities exist (appendActivity)
        }

        // --- Composer (v1.0.8 Aurora: the unified ChatGPT/z.ai pill) ---
        // ONE surface: input on top, action row inside the bottom — attach,
        // camera, tools and thinking live IN the composer like the top AI
        // chat platforms, not in a toolbar bolted beside it. The send disc
        // anchors the bottom-right corner. Attachments stage INSIDE the
        // pill above the input.
        d.msgInput = widget.NewMultiLineEntry()
        d.msgInput.SetPlaceHolder("Message SHEYTAN…")
        d.msgInput.SetMinRowsVisible(2)
        d.msgInput.OnSubmitted = func(string) { d.sendMessage() }

        inputScroll := container.NewVScroll(d.msgInput)
        inputScroll.SetMinSize(fyne.NewSize(10, 74))

        d.sendCirc = newSendButton(d.sendMessage)

        // v1.0.7: Continuum context meter — v1.0.8 moves it into the
        // composer's action row (live pressure + chapter, tappable).
        d.ctxMeter = newContextMeter(d.showContextDialog)

        // v1.0.8 composer controls: quiet tiles INSIDE the pill (ChatGPT
        // idiom) — invisible at rest, warm on hover, molten when active.
        d.attachBtn = newComposerButton("attach", d.attachFiles)
        d.camBtn = newComposerButton("camera", d.captureScreen)
        d.toolsBtn = newComposerButton("tools", d.showToolsPopup)
        d.thinkBtn = newComposerButton("brain", d.toggleThinking)
        d.thinkBtn.SetActive(d.cfg.ThinkingMode)
        d.updateToolsChip()

        controls := container.NewHBox(
                container.NewPadded(d.attachBtn),
                container.NewPadded(d.camBtn),
                container.NewPadded(d.toolsBtn),
                container.NewPadded(d.thinkBtn),
        )

        // Bottom action row: controls left, context meter center-right,
        // send disc far right — one breathing line of controls.
        actionRow := container.NewBorder(nil, nil,
                controls,
                container.NewHBox(container.NewPadded(d.ctxMeter),
                        container.NewPadded(container.NewPadded(d.sendCirc))),
                nil)

        // Attachment chips row (staged files) — INSIDE the pill, above the
        // input, exactly where ChatGPT stages its uploads.
        d.attachRow = container.NewHScroll(container.NewHBox())
        d.attachRow.Hide()

        // The pill: elevation shadow under, glass body, lit hairline, and
        // the stacked content — attachments, input, action row.
        composerBg := canvas.NewRectangle(ColGlass)
        composerBg.CornerRadius = radiusPill + 2
        composerBg.StrokeColor = ColGlassEdge
        composerBg.StrokeWidth = 1
        composer := container.NewStack(
                elevation(radiusPill+2, 3),
                composerBg,
                hairlines(14),
                container.NewPadded(container.NewPadded(container.NewVBox(
                        container.NewPadded(d.attachRow),
                        inputScroll,
                        container.NewPadded(actionRow),
                ))),
        )

        d.chatBottom = container.NewVBox(
                d.activitySection,
                container.NewPadded(composer),
        )

        return container.NewBorder(
                nil,
                d.chatBottom,
                nil, nil,
                container.NewPadded(d.chatScroll),
        )
}

// buildEmptyHero renders the first-run / empty-chat hero: flame, greeting,
// suggestion cards, and (when no local model exists) a get-started card.
// v1.0.3: the flame sits on a soft radial ember glow, and the suggestion
// cards stagger in with a reveal animation.
func (d *desktopApp) buildEmptyHero() *fyne.Container {
        glow := canvas.NewRadialGradient(color.NRGBA{R: 255, G: 90, B: 38, A: 40}, color.Transparent)
        glow.SetMinSize(fyne.NewSize(160, 160))
        flame := canvas.NewImageFromResource(Logo)
        flame.FillMode = canvas.ImageFillContain
        flame.SetMinSize(fyne.NewSize(56, 56))

        hello := canvas.NewText("What shall we forge today?", ColText)
        hello.TextSize = 26
        hello.TextStyle.Bold = true
        hello.Alignment = fyne.TextAlignCenter

        hint := canvas.NewText("Ask anything — files, data, code, the web, your screen. Everything runs locally in this folder.", ColTextMuted)
        hint.TextSize = 13
        hint.Alignment = fyne.TextAlignCenter

        // Suggestion cards (chat.z.ai-style quick starts). v1.0.6: the
        // "see my screen" card leads — vision is the headline feature.
        suggest := func(title, desc, prompt, iconName string) *suggestionCard {
                return newSuggestionCard(iconName, title, desc, func() {
                        d.msgInput.SetText(prompt)
                        d.msgInput.FocusGained()
                        if d.currentView != "chat" {
                                d.showView("chat")
                        }
                })
        }
        cards := container.NewGridWithRows(2,
                suggest("See my screen", "Capture the display and ask about it (vision)", "Take a screenshot and describe what you see on it.", "camera"),
                suggest("Analyze data", "Create a sample CSV, profile it, and chart revenue", "Create sales.csv with sample data, analyze it, and chart revenue by region.", "data"),
                suggest("Research the web", "Search, open pages, and summarize findings", "Search the web for the latest Go release and summarize the key changes.", "browser"),
                suggest("Write & run code", "Sandboxed Python/Go, output captured", "Write a Python script that renames all .txt files in workspace/ and run it.", "files"),
        )

        hero := container.NewVBox(
                container.NewStack(glow, container.NewPadded(flame)),
                container.NewPadded(hello),
                container.NewPadded(hint),
                widget.NewSeparator(),
                cards,
        )

        // First-run guidance when no local model and no remote provider —
        // ABOVE the grid so the suggestion cards stay a clean, unbroken 2x2.
        if !d.cfg.IsRemote() && len(llm.ListLocalModels(d.cfg.ModelsDir)) == 0 {
                warn := canvas.NewText("No local model installed yet", ColGold)
                warn.TextSize = 14
                warn.TextStyle.Bold = true
                warn.Alignment = fyne.TextAlignCenter
                lead := widget.NewLabel("Drop a .gguf model file (Qwen, Gemma, Llama…) into the models folder — or connect a remote provider. Everything else is ready.")
                lead.Wrapping = fyne.TextWrapWord
                lead.Alignment = fyne.TextAlignCenter
                openBtn := ghostButton("Open models folder", "folder", func() { d.openInExplorer(d.cfg.ModelsDir) })
                provBtn := ghostButton("Connect provider", "provider", d.showProviderDialog)
                getStarted := panel(container.NewVBox(
                        warn,
                        container.NewCenter(lead),
                        container.NewHBox(layout.NewSpacer(), openBtn, provBtn, layout.NewSpacer()),
                ), 12, 10)
                hero.Add(container.NewPadded(getStarted))
                hero.Add(widget.NewSeparator())
        }

        // v1.0.3: staggered card entrance — each card reveals in turn
        // (80ms apart) so the empty state lands with rhythm.
        go func() {
                time.Sleep(350 * time.Millisecond)
                for i, o := range cards.Objects {
                        card := o
                        runOnMain(func() {
                                revealIn(card, cards, ColBg, 0)
                        })
                        time.Sleep(80 * time.Millisecond)
                        _ = i
                }
        }()

        return container.NewVBox(
                layout.NewSpacer(),
                container.NewCenter(hero),
                layout.NewSpacer(),
        )
}

// appendMessage adds one bubble to the chat stream (main thread only).
func (d *desktopApp) appendMessage(role, content string) {
        d.appendMessageFull(role, content, "", nil)
}

// appendMessageFull adds one bubble with optional reasoning + attachments
// (v1.0.2). v1.0.3: new bubbles REVEAL in — a cover in the ambient
// background dissolves away so the message fades into the stream instead
// of popping.
func (d *desktopApp) appendMessageFull(role, content, reasoning string, attachments []string) {
        d.appendBubble(bubbleInfo{Role: role, Content: content, Reasoning: reasoning, Attachments: attachments})
}

// appendBubble renders one v1.0.6 bubble — text, reasoning, attachments,
// image thumbnails, timestamp, feedback buttons — with the reveal entrance.
func (d *desktopApp) appendBubble(info bubbleInfo) {
        b := newMessageBubble()
        b.SetMessage(info)
        role := info.Role
        var row fyne.CanvasObject
        if role == "user" {
                // right-aligned tinted bubble at 86% of the measure
                row = container.New(&chatRowLayout{right: true, frac: 0.86}, b)
        } else {
                // assistant: clean, full-measure text
                row = container.New(&chatRowLayout{frac: 1.0}, b)
        }
        d.chatBox.Add(row)
        d.chatEmpty.Hide()
        d.chatBox.Refresh()
        // v1.0.3 entrance: reveal the fresh bubble (re-rendered histories skip
        // the animation to keep session switches instant).
        if !d.renderingHistory {
                revealIn(row, d.chatBox, ColBg, 0)
        }
        d.chatScroll.ScrollToBottom()
}

// zoomImage opens a tap-to-zoom preview of an attached image (v1.0.6).
func (d *desktopApp) zoomImage(path string) {
        img := canvas.NewImageFromFile(path)
        img.FillMode = canvas.ImageFillContain
        img.SetMinSize(fyne.NewSize(620, 420))
        d.bigDialog("Image preview", scrollDialogContent(container.NewStack(img), fyne.NewSize(640, 440)), 700, 500)
}

// --- Data view (charts gallery) ---

func (d *desktopApp) buildDataView() fyne.CanvasObject {
        d.chartGrid = widget.NewGridWrap(
                func() int { return len(d.chartFiles) },
                func() fyne.CanvasObject {
                        img := canvas.NewImageFromResource(nil)
                        img.FillMode = canvas.ImageFillContain
                        img.SetMinSize(fyne.NewSize(220, 150)) // give grid cells a preview area
                        name := widget.NewLabel("")
                        name.Alignment = fyne.TextAlignCenter
                        name.TextStyle = fyne.TextStyle{Bold: true}
                        name.Truncation = fyne.TextTruncateClip // long filenames must not inflate cells
                        meta := widget.NewLabel("")
                        meta.Alignment = fyne.TextAlignCenter
                        meta.Truncation = fyne.TextTruncateClip
                        cell := &chartCell{img: img, name: name, meta: meta}
                        cell.ExtendBaseWidget(cell)
                        cell.obj = container.NewVBox(container.NewPadded(img), name, meta)
                        return container.NewPadded(cell)
                },
                func(idx widget.ListItemID, item fyne.CanvasObject) {
                        if idx < 0 || idx >= len(d.chartFiles) {
                                return
                        }
                        cf := d.chartFiles[idx]
                        ci := item.(*fyne.Container).Objects[0].(*chartCell)
                        if res, err := fyne.LoadResourceFromPath(cf.Path); err == nil {
                                ci.img.Resource = res
                        } else {
                                ci.img.Resource = icon("data")
                        }
                        ci.img.Refresh()
                        ci.name.SetText(cf.Name)
                        ci.meta.SetText(cf.Meta)
                },
        )
        d.chartGrid.OnSelected = func(id widget.ListItemID) {
                if id < 0 || id >= len(d.chartFiles) {
                        return
                }
                d.openInExplorer(d.chartFiles[id].Path)
                // v1.0.0 dead-click fix: deselect so clicking the same chart again
                // reopens the folder.
                d.chartGrid.UnselectAll()
        }

        refresh := primaryButton("Refresh", "refresh", d.refreshCharts)
        open := ghostButton("Open folder", "folder", func() { d.openInExplorer(d.cfg.ChartsDir()) })
        hint := widget.NewLabel("Charts the agent renders land here automatically.")
        hint.TextStyle = fyne.TextStyle{Italic: true}

        d.chartEmpty = widget.NewLabel("No charts yet.\n\nAsk the agent something like:\n\"Create sales.csv with sample data, analyze it, and chart revenue by region.\"")
        d.chartEmpty.TextStyle = fyne.TextStyle{Italic: true}

        return container.NewBorder(
                container.NewVBox(
                        container.NewPadded(sectionHeader("data", "Data & Charts")),
                        container.NewPadded(container.NewHScroll(container.NewHBox(refresh, open, hint))),
                ),
                nil, nil, nil,
                container.NewStack(d.chartEmpty, d.chartGrid),
        )
}

// chartFile is one rendered SVG chart.
type chartFile struct {
        Name string
        Path string
        Meta string
}

func (d *desktopApp) refreshCharts() {
        entries, err := os.ReadDir(d.cfg.ChartsDir())
        if err != nil {
                d.chartFiles = nil
        } else {
                var files []chartFile
                for _, e := range entries {
                        if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".svg") {
                                continue
                        }
                        info, _ := e.Info()
                        files = append(files, chartFile{
                                Name: e.Name(),
                                Path: filepath.Join(d.cfg.ChartsDir(), e.Name()),
                                Meta: fmt.Sprintf("%d KB · %s", maxInt64(info.Size()/1024, 1), info.ModTime().Format("Jan 02 15:04")),
                        })
                }
                sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
                d.chartFiles = files
        }
        if len(d.chartFiles) == 0 {
                d.chartEmpty.Show()
        } else {
                d.chartEmpty.Hide()
        }
        if d.chartGrid != nil {
                d.chartGrid.Refresh()
        }
}

func maxInt64(a, b int64) int64 {
        if a > b {
                return a
        }
        return b
}

// --- Memory view ---

func (d *desktopApp) buildMemoryView() fyne.CanvasObject {
        d.memEntries, _ = d.mem.All()
        d.memBox = container.NewVBox()
        d.memScroll = container.NewVScroll(d.memBox)
        d.memEmpty = widget.NewLabel("Memory is empty — the agent stores durable facts here as you work.")
        d.memEmpty.TextStyle = fyne.TextStyle{Italic: true}
        d.renderMemory()

        search := widget.NewEntry()
        search.SetPlaceHolder("Search memory…")
        search.OnChanged = func(q string) {
                if q == "" {
                        d.memEntries, _ = d.mem.All()
                } else {
                        d.memEntries, _ = d.mem.Search(q, 50)
                }
                d.renderMemory()
        }
        clearBtn := widget.NewButtonWithIcon("Clear all", icon("warn"), func() {
                dialog.ShowConfirm("Clear all memory?", "This deletes every stored fact.", func(ok bool) {
                        if !ok {
                                return
                        }
                        _ = d.mem.Clear()
                        d.memEntries, _ = d.mem.All()
                        d.renderMemory()
                }, d.win)
        })
        clearBtn.Importance = widget.DangerImportance

        return container.NewBorder(
                container.NewVBox(
                        container.NewPadded(sectionHeader("memory", "Agent Memory")),
                        container.NewPadded(container.NewBorder(nil, nil, search, clearBtn, nil)),
                ),
                nil, nil, nil,
                container.NewPadded(container.NewStack(container.NewCenter(d.memEmpty), d.memScroll)),
        )
}

// renderMemory rebuilds the memory card list from d.memEntries. v1.0.7: the
// active thread's Continuum Framework renders first — the living state the
// agent carries across chapters (mission, facts, decisions, open threads).
func (d *desktopApp) renderMemory() {
        if d.memBox == nil {
                return
        }
        d.memBox.RemoveAll()
        if card := d.frameworkCard(); card != nil {
                d.memBox.Add(card)
        }
        for _, e := range d.memEntries {
                title := widget.NewLabel(fmt.Sprintf("%s  ·  %v", clipStrMemory(e.ID, 18), e.Tags))
                title.TextStyle = fyne.TextStyle{Bold: true}
                title.Truncation = fyne.TextTruncateClip
                body := widget.NewLabel(clipStrMemory(e.Content, 280))
                body.Wrapping = fyne.TextWrapWord
                d.memBox.Add(panel(container.NewVBox(title, body), 8, 8))
        }
        if len(d.memEntries) == 0 && d.frameworkCard() == nil {
                d.memEmpty.Show()
        } else {
                d.memEmpty.Hide()
        }
        d.memBox.Refresh()
}

// frameworkCard renders the active thread's Continuum Framework as the
// first card of the Memory view (nil when no framework exists yet).
func (d *desktopApp) frameworkCard() fyne.CanvasObject {
        if d.active == nil {
                return nil
        }
        fw := continuum.LoadFramework(d.cfg.SessionsDir, d.active.ID)
        if fw == nil || fw.Empty() {
                return nil
        }
        chapter := d.active.Chapter
        if chapter == 0 {
                chapter = 1
        }
        head := container.NewHBox(
                container.NewPadded(luxeBadge("layers", 24, color.NRGBA{R: 255, G: 120, B: 50, A: 52})),
                container.NewPadded(canvas.NewText(fmt.Sprintf("Thread state — Chapter %d · %d items", chapter, fw.FactCount()), ColEmber)),
        )
        headTxt := head.Objects[1].(*fyne.Container).Objects[0].(*canvas.Text)
        headTxt.TextStyle.Bold = true

        var body strings.Builder
        if fw.Mission != "" {
                body.WriteString("Mission: " + fw.Mission + "\n\n")
        }
        writeSection := func(title string, items []string) {
                if len(items) == 0 {
                        return
                }
                body.WriteString(title + "\n")
                for i, it := range items {
                        if i >= 5 { // keep the card scannable; full state rides the prompt
                                body.WriteString(fmt.Sprintf("… and %d more\n", len(items)-5))
                                break
                        }
                        body.WriteString("• " + it + "\n")
                }
                body.WriteString("\n")
        }
        writeSection("Facts", fw.Facts)
        writeSection("Decisions", fw.Decisions)
        writeSection("Open threads", fw.OpenThreads)
        writeSection("Files", fw.Artifacts)
        writeSection("Preferences", fw.Preferences)

        lbl := widget.NewLabel(strings.TrimRight(body.String(), "\n"))
        lbl.Wrapping = fyne.TextWrapWord
        return panel(container.NewVBox(head, widget.NewSeparator(), lbl), 10, 10)
}

func clipStrMemory(s string, n int) string {
        if len(s) <= n {
                return s
        }
        return s[:n] + "…"
}

// --- Logs view ---

func (d *desktopApp) buildLogsView() fyne.CanvasObject {
        d.logsList = widget.NewList(
                func() int { return len(d.logLines) },
                func() fyne.CanvasObject { return widget.NewLabel("log") },
                func(idx widget.ListItemID, item fyne.CanvasObject) {
                        l := item.(*widget.Label)
                        if idx >= 0 && idx < len(d.logLines) {
                                l.SetText(d.logLines[idx])
                        }
                        l.Wrapping = fyne.TextWrapOff
                        l.Truncation = fyne.TextTruncateClip // long lines clip at the pane edge
                        l.TextStyle = fyne.TextStyle{Monospace: true}
                },
        )
        statsBtn := ghostButton("Stats", "tools", func() {
                st := logging.Default().ComputeStats()
                dialog.ShowInformation("Tool stats (from tools.jsonl)",
                        fmt.Sprintf("Tool calls:  %d (%d errors)\nLLM calls:   %d (%d errors)\nAvg LLM latency: %.0f ms\n\nPer tool:\n%s",
                                st.ToolCalls, st.ToolErrors, st.LLMCalls, st.LLMErrors, st.AvgLLMLatency,
                                formatToolStats(st)), d.win)
        })
        exportBtn := ghostButton("Export", "export", d.exportDiagnostics)
        openBtn := ghostButton("Folder", "folder", func() { d.openInExplorer(d.cfg.LogsDir()) })
        refreshBtn := primaryButton("Refresh", "refresh", d.refreshLogs)

        d.refreshLogs()
        return container.NewBorder(
                container.NewVBox(
                        container.NewPadded(sectionHeader("logs", "Log Catcher")),
                        container.NewPadded(container.NewHScroll(container.NewHBox(refreshBtn, statsBtn, exportBtn, openBtn))),
                        container.NewPadded(widget.NewLabel("app.log · tools.jsonl · llm.jsonl · crash reports — live capture.")),
                ),
                nil, nil, nil,
                container.NewPadded(d.logsList),
        )
}

// --- Pro dock tabs ---

func (d *desktopApp) buildRightTabs() *container.AppTabs {
        tabs := container.NewAppTabs(
                container.NewTabItemWithIcon("Context", icon("context"), d.buildContextTab()),
                container.NewTabItemWithIcon("Params", icon("settings"), d.buildParamsTab()),
                container.NewTabItemWithIcon("System", icon("system"), d.buildSystemTab()),
                container.NewTabItemWithIcon("Tools", icon("tools"), d.buildToolsTab()),
        )
        tabs.SetTabLocation(container.TabLocationTop)
        return tabs
}

func (d *desktopApp) buildContextTab() fyne.CanvasObject {
        sysPrompt := widget.NewMultiLineEntry()
        sysPrompt.SetPlaceHolder("System prompt…")
        sysPrompt.SetMinRowsVisible(4)
        if d.active != nil && d.active.Context.SystemPrompt != "" {
                sysPrompt.SetText(d.active.Context.SystemPrompt)
        }
        d.sysPromptEntry = sysPrompt

        selectedFile := -1
        attachedFiles := widget.NewList(
                func() int {
                        if d.active != nil && d.active.Context.AttachedFiles != nil {
                                return len(d.active.Context.AttachedFiles)
                        }
                        return 0
                },
                func() fyne.CanvasObject { return widget.NewLabel("file") },
                func(idx widget.ListItemID, item fyne.CanvasObject) {
                        if d.active != nil && idx < len(d.active.Context.AttachedFiles) {
                                item.(*widget.Label).SetText(d.active.Context.AttachedFiles[idx])
                        }
                },
        )
        attachedFiles.OnSelected = func(id widget.ListItemID) { selectedFile = int(id) }
        // v1.0.5: pin a minimum height for the attached-files list — a bare
        // widget.List inside a VBox collapses to a one-row sliver, which made
        // the Context tab feel broken ("panel too small / not functional").
        filesFloor := canvas.NewRectangle(color.Transparent)
        filesFloor.SetMinSize(fyne.NewSize(280, 120))
        filesList := container.NewStack(filesFloor, attachedFiles)
        addFile := widget.NewButtonWithIcon("Add file", icon("files"), func() {
                dialog.ShowEntryDialog("Attach file", "Absolute path:", func(path string) {
                        if path == "" || d.active == nil {
                                return
                        }
                        d.active.Context.AttachedFiles = append(d.active.Context.AttachedFiles, path)
                        _ = d.store.UpdateContext(d.active.ID, d.active.Context)
                        attachedFiles.Refresh()
                }, d.win)
        })
        // v1.0.0 dead-click fix: attached files could be added but never removed.
        removeFile := widget.NewButtonWithIcon("Remove selected", icon("warn"), func() {
                if d.active == nil || selectedFile < 0 || selectedFile >= len(d.active.Context.AttachedFiles) {
                        return
                }
                files := d.active.Context.AttachedFiles
                d.active.Context.AttachedFiles = append(files[:selectedFile], files[selectedFile+1:]...)
                _ = d.store.UpdateContext(d.active.ID, d.active.Context)
                selectedFile = -1
                attachedFiles.UnselectAll()
                attachedFiles.Refresh()
        })
        maxIter := widget.NewEntry()
        maxIter.SetText(fmt.Sprintf("%d", d.cfg.MaxIterations))
        d.maxIterEntry = maxIter

        save := primaryButton("Save context", "export", func() {
                if d.active == nil {
                        return
                }
                ctx := sessions.Context{
                        SystemPrompt:  sysPrompt.Text,
                        AttachedFiles: d.active.Context.AttachedFiles,
                }
                if n, err := fmt.Sscanf(maxIter.Text, "%d", &ctx.MaxIterations); err == nil && n == 1 {
                        // ok
                }
                _ = d.store.UpdateContext(d.active.ID, ctx)
                d.active.Context = ctx
                dialog.ShowInformation("Context saved", "System prompt, files, and max-iterations saved.", d.win)
        })

        return container.NewVScroll(container.NewVBox(
                sectionHeader("context", "Session context"),
                sysPrompt,
                widget.NewSeparator(),
                sectionHeader("files", "Attached files"),
                filesList,
                container.NewHBox(addFile, removeFile),
                widget.NewSeparator(),
                sectionHeader("agent", "Max iterations"),
                maxIter,
                save,
        ))
}

func (d *desktopApp) buildParamsTab() fyne.CanvasObject {
        llmOpts := d.cfg.LLM
        temp := newSliderRow("Temperature", llmOpts.Temperature, 0, 2, 0.05)
        topP := newSliderRow("Top-p", llmOpts.TopP, 0, 1, 0.01)
        topK := newSliderRow("Top-k", float64(llmOpts.TopK), 0, 200, 1)
        maxTok := newSliderRow("Max tokens", float64(llmOpts.MaxTokens), 64, 8192, 64)
        repP := newSliderRow("Repeat penalty", llmOpts.RepeatPenalty, 0.5, 2.0, 0.01)
        ctxN := newSliderRow("Num ctx", float64(llmOpts.NumCtx), 512, 32768, 512)
        batch := newSliderRow("Num batch", float64(llmOpts.NumBatch), 128, 4096, 64)
        thrN := newSliderRow("Num thread", float64(llmOpts.NumThread), 1, 128, 1)
        gpuN := newSliderRow("Num GPU layers", float64(llmOpts.NumGPU), 0, 999, 1)
        miro := newSliderRow("Mirostat", float64(llmOpts.Mirostat), 0, 2, 1)

        save := primaryButton("Save params", "export", func() {
                d.cfg.LLM.Temperature = temp.Value
                d.cfg.LLM.TopP = topP.Value
                d.cfg.LLM.TopK = int(topK.Value)
                d.cfg.LLM.MaxTokens = int(maxTok.Value)
                d.cfg.LLM.RepeatPenalty = repP.Value
                d.cfg.LLM.NumCtx = int(ctxN.Value)
                d.cfg.LLM.NumBatch = int(batch.Value)
                d.cfg.LLM.NumThread = int(thrN.Value)
                d.cfg.LLM.NumGPU = int(gpuN.Value)
                d.cfg.LLM.Mirostat = int(miro.Value)
                _ = config.Save(d.cfg.ConfigPath(), d.cfg)
                dialog.ShowInformation("Params saved", "Sampling + runtime knobs updated.\n\nRunning engine: changes apply after the next model switch or engine restart.", d.win)
        })

        return container.NewVScroll(container.NewVBox(
                sectionHeader("settings", "Sampling"),
                temp.Box(), topP.Box(), topK.Box(), maxTok.Box(), repP.Box(),
                widget.NewSeparator(),
                sectionHeader("settings", "Runtime"),
                ctxN.Box(), batch.Box(), thrN.Box(), gpuN.Box(), miro.Box(),
                save,
        ))
}

func (d *desktopApp) buildSystemTab() fyne.CanvasObject {
        info := sysinfo.Probe()
        rows := []string{
                fmt.Sprintf("OS:         %s/%s", info.OS, info.Arch),
                fmt.Sprintf("Hostname:   %s", info.Hostname),
                fmt.Sprintf("CPU:        %s", info.CPU.Name),
                fmt.Sprintf("Cores:      %d physical / %d logical", info.CPU.PhysicalCores, info.CPU.LogicalCores),
                fmt.Sprintf("RAM total:  %s", sysinfo.FormatBytes(info.RAM.TotalBytes)),
                fmt.Sprintf("RAM free:   %s", sysinfo.FormatBytes(info.RAM.FreeBytes)),
                fmt.Sprintf("RAM avail:  %s", sysinfo.FormatBytes(info.RAM.Available)),
                fmt.Sprintf("Disk free:  %s", sysinfo.FormatBytes(info.Disk.FreeBytes)),
        }
        if len(info.GPU) > 0 {
                for _, g := range info.GPU {
                        rows = append(rows, fmt.Sprintf("GPU:        %s %s", g.Vendor, g.Name))
                        if g.VRAMBytes > 0 {
                                rows = append(rows, "  VRAM:     "+sysinfo.FormatBytes(g.VRAMBytes))
                        }
                }
        } else {
                rows = append(rows, "GPU:        none detected")
        }
        rows = append(rows,
                fmt.Sprintf("WSL2:       %v", info.WSL2),
                fmt.Sprintf("Docker:     %v", info.Docker),
        )
        for _, w := range info.Recommended.Warnings {
                rows = append(rows, "⚠ "+w)
        }

        applyBtn := primaryButton("Apply recommended", "system", func() {
                d.cfg.LLM.NumThread = info.Recommended.NumThread
                d.cfg.LLM.NumGPU = info.Recommended.NumGPU
                d.cfg.LLM.NumCtx = info.Recommended.NumCtx
                d.cfg.LLM.NumBatch = info.Recommended.NumBatch
                d.cfg.LLM.MaxTokens = info.Recommended.MaxTokens
                _ = config.Save(d.cfg.ConfigPath(), d.cfg)
                dialog.ShowInformation("Applied", "Recommended knobs applied.", d.win)
        })

        rec := widget.NewLabel(fmt.Sprintf(
                "num_thread=%d\nnum_gpu=%d\nnum_ctx=%d\nnum_batch=%d\nmax_tokens=%d\ncan_run_cpu=%v\ncan_run_gpu=%v",
                info.Recommended.NumThread, info.Recommended.NumGPU, info.Recommended.NumCtx,
                info.Recommended.NumBatch, info.Recommended.MaxTokens,
                info.Recommended.CanRunCPU, info.Recommended.CanRunGPU,
        ))
        rec.TextStyle = fyne.TextStyle{Monospace: true}

        // wrap long host lines — an unwrapped label's min width is its longest
        // line, which at small window sizes squeezes the chat column shut.
        host := widget.NewLabel(strings.Join(rows, "\n"))
        host.Wrapping = fyne.TextWrapWord
        rec.Wrapping = fyne.TextWrapWord

        return container.NewVScroll(container.NewVBox(
                sectionHeader("system", "Host"),
                host,
                widget.NewSeparator(),
                sectionHeader("system", "Recommended llama.cpp knobs"),
                rec,
                applyBtn,
        ))
}

func (d *desktopApp) buildToolsTab() fyne.CanvasObject {
        cards := container.NewVBox()

        // v1.0.2: per-tool enable/disable — the same control as the composer
        // tools popover, mirrored here with full descriptions.
        d.toolsChecks = map[string]*widget.Check{}
        checks := container.NewVBox()
        names := make([]string, 0, len(d.orch.Tools()))
        for name := range d.orch.Tools() {
                names = append(names, name)
        }
        sort.Strings(names)
        for _, name := range names {
                t := d.orch.Tools()[name]
                chk := widget.NewCheck(t.Description(), func(on bool) { d.setToolEnabled(name, on) })
                chk.SetChecked(d.cfg.ToolEnabled(name))
                d.toolsChecks[name] = chk
                checks.Add(chk)
        }
        allBtn := primaryButton("Enable all", "check", func() { d.setAllTools(true) })
        offBtn := ghostButton("Disable all", "close", func() { d.setAllTools(false) })
        presetRow := container.NewHBox(allBtn, offBtn)

        cards.Add(panel(container.NewVBox(
                sectionHeader("tools", "Tool selection"),
                checks,
                presetRow,
        ), 8, 8))

        for _, t := range d.orch.Tools() {
                name := t.Name()
                desc := t.Description()
                title := widget.NewLabel(name)
                body := widget.NewLabel(clipStrMemory(desc, 280))
                body.Wrapping = fyne.TextWrapWord
                cards.Add(panel(container.NewVBox(title, body), 8, 8))
        }
        sandboxInfo := widget.NewLabel(fmt.Sprintf(
                "OS: %s/%s\nWindows: Job-Object sandbox (memory cap, kill-on-close, isolated workdir under sandbox/).\nOther: os/exec fallback with temp workdir.",
                runtime.GOOS, runtime.GOARCH,
        ))
        sandboxInfo.Wrapping = fyne.TextWrapWord
        cards.Add(panel(container.NewVBox(
                sectionHeader("sandbox", "Sandbox"),
                sandboxInfo,
        ), 8, 8))
        return container.NewVScroll(container.NewPadded(cards))
}

// --- slider row ---

type sliderRow struct {
        slider *widget.Slider
        value  *widget.Label
        row    fyne.CanvasObject
        Value  float64
}

func newSliderRow(label string, val, min, max, step float64) *sliderRow {
        r := &sliderRow{
                slider: widget.NewSlider(min, max),
                value:  widget.NewLabel(fmt.Sprintf("%.2f", val)),
        }
        r.slider.Value = val
        r.slider.Step = step
        r.slider.OnChanged = func(v float64) {
                r.value.SetText(fmt.Sprintf("%.2f", v))
                r.Value = v
        }
        r.Value = val
        r.row = container.NewVBox(
                container.NewBorder(nil, nil, widget.NewLabel(label), nil, r.value),
                r.slider,
        )
        return r
}

func (s *sliderRow) Box() fyne.CanvasObject { return s.row }

// renderChat rebuilds the bubble stream from the active session. Only the
// most recent maxRenderedMessages messages become widgets so huge sessions
// stay fast to switch into.
const maxRenderedMessages = 300

func (d *desktopApp) renderChat() {
        if d.chatBox == nil {
                return
        }
        // v1.0.3: history re-renders skip the entrance animation.
        d.renderingHistory = true
        defer func() { d.renderingHistory = false }()
        d.chatBox.RemoveAll()
        msgs := d.active.Messages
        offset := 0
        if len(msgs) > maxRenderedMessages {
                offset = len(msgs) - maxRenderedMessages
                msgs = msgs[offset:]
        }
        for i, m := range msgs {
                idx := offset + i
                // v1.0.7 Continuum: chapter briefings are not conversation —
                // they render as the divider card at the top of the chapter.
                if m.Role == "system" && continuum.IsBriefing(m.Content) {
                        d.chatBox.Add(newChapterDivider(chapterNumber(d.active, idx), 0, 0, 0))
                        continue
                }
                info := bubbleInfo{
                        Role:        m.Role,
                        Content:     m.Content,
                        Reasoning:   m.Reasoning,
                        Attachments: m.Attachments,
                        Images:      m.Images,
                        Feedback:    m.Feedback,
                        At:          m.At,
                        OnZoom:      d.zoomImage,
                }
                if m.Role == "assistant" {
                        info.OnFeedback = func(fb int) { d.setFeedback(idx, fb) }
                }
                d.appendBubble(info)
        }
        if len(msgs) == 0 {
                d.chatEmpty.Show()
        } else {
                d.chatEmpty.Hide()
        }
        d.chatBox.Refresh()
        d.chatScroll.ScrollToBottom()
        d.updateContextMeter()
}

// chapterNumber resolves the chapter a briefing at message index idx belongs
// to (the session's own chapter, or 2+ for a fresh session whose chapter is
// still unset).
func chapterNumber(sess *sessions.Session, idx int) int {
        if sess != nil && sess.Chapter > 0 {
                return sess.Chapter
        }
        return idx + 2 // briefing at position 0 of a chain implies chapter 2+
}

// setFeedback records the user's 👍/👎 on the assistant message at index
// idx (v1.0.6): persisted on the message, mirrored into the recall engine
// (liked answers rank higher in future memory retrieval) and logged.
func (d *desktopApp) setFeedback(idx, fb int) {
        if d.active == nil {
                return
        }
        d.setFeedbackIn(d.active.ID, idx, fb)
}

// setFeedbackIn is the v1.0.7 session-pinned variant: the vote lands on the
// session that actually owns the message, even after a Continuum rollover
// swapped the active session underneath the chat stream.
func (d *desktopApp) setFeedbackIn(sessionID string, idx, fb int) {
        var target *sessions.Session
        if d.active != nil && d.active.ID == sessionID {
                target = d.active
        } else if full, err := d.store.Get(sessionID); err == nil {
                target = full
        }
        if target == nil || idx < 0 || idx >= len(target.Messages) {
                return
        }
        target.Messages[idx].Feedback = fb
        _ = d.store.Save(target)
        if d.recall != nil {
                // The recall capsule is keyed by session+the preceding user query.
                query := ""
                for j := idx - 1; j >= 0; j-- {
                        if target.Messages[j].Role == "user" {
                                query = target.Messages[j].Content
                                break
                        }
                }
                if query != "" {
                        _ = d.recall.SetFeedback(recall.CapsuleID(sessionID, query), fb)
                }
        }
        switch fb {
        case 1:
                d.setStatus("Thanks — SHEYTAN will prefer answers like this one")
        case -1:
                d.setStatus("Noted — SHEYTAN will avoid answers like this one")
        default:
                d.setStatus("Feedback cleared")
        }
        logging.Default().Info("gui", "feedback on message %d → %d (session %s)", idx, fb, sessionID)
}

// renderActive refreshes the chat view from the active session.
func (d *desktopApp) renderActive() {
        if d.active == nil {
                if d.chatBox != nil {
                        d.chatBox.RemoveAll()
                        d.chatEmpty.Show()
                        d.chatBox.Refresh()
                }
                d.clearActivities()
                return
        }
        d.renderChat()
        if d.dots != nil {
                d.dots.hide()
        }
        if d.sysPromptEntry != nil {
                d.sysPromptEntry.SetText(d.active.Context.SystemPrompt)
        }
        d.clearActivities()
}

// message is one display message in the chat view.
type message struct {
        Role    string
        Content string
}

// refreshBottom re-layouts the chat bottom dock after show/hide changes —
// Fyne does not re-arrange a container when only a child's visibility
// flips, so the parent must be refreshed or the section keeps its stale
// (pre-show) zero size.
func (d *desktopApp) refreshBottom() {
        if d.chatBottom != nil {
                d.chatBottom.Refresh()
        }
}

// clearActivities resets the activity stream (new turn / session switch).
func (d *desktopApp) clearActivities() {
        d.activities = nil
        d.liveResponseLbl = nil
        d.liveReasoningLbl = nil
        if d.activityBox != nil {
                d.activityBox.RemoveAll()
                d.activityBox.Refresh()
        }
        if d.activitySection != nil {
                d.activitySection.Hide()
        }
        d.refreshBottom()
}

// appendActivity adds an activity row and updates the live status line. It
// can be called from any goroutine — every widget mutation is routed through
// fyne.Do. In minimal mode only the one-line summary updates; the full
// stream renders when Pro mode is on.
//
// v1.0.9 (TURBINE): streaming snapshots (response/reasoning) no longer
// touch the widget tree directly — they are absorbed by the frame-paced
// pacer, which coalesces them into at most ONE UI batch per display frame
// (default 120 fps). Milestone activities (tool calls, errors, done) still
// apply immediately.
func (d *desktopApp) appendActivity(a agent.Activity) {
        if d.cfg.SmoothStream && (a.Type == "response" || a.Type == "reasoning") {
                if d.stream == nil {
                        d.stream = newStreamPacer(d.cfg.EffectiveTargetFPS(), func(resp, reason string, tps float64) {
                                runOnMain(func() { d.flushStreamFrame(resp, reason, tps) })
                        })
                }
                d.stream.Pump(a)
                // Status-line text also rides the pacer (one SetText per frame,
                // not one per snapshot) — the flame/dots indicators keep the
                // "alive" feel meanwhile.
                return
        }
        runOnMain(func() {
                d.activities = append(d.activities, a)
                caption := a.Caption
                // v1.0.2: reasoning deltas surface as a thinking indicator in
                // the status line, not as raw text (the full trace lands in
                // the message's collapsible section once the turn completes).
                if a.Type == "reasoning" {
                        caption = "Thinking… " + clipStrMemory(firstLine(a.Caption, 90), 90)
                }
                d.activityLbl.SetText(clipStrMemory(caption, 140))
                if d.cfg.ProMode {
                        d.activitySection.Show()
                        d.refreshBottom()
                        if a.Type == "response" {
                                // v1.0.1 perf: streaming text renders in ONE
                                // live row updated in place — v1.0.0 added a
                                // full widget row per token delta, flooding
                                // the VBox with hundreds of rows per reply.
                                if d.liveResponseLbl == nil {
                                        row, lbl := newLiveResponseRow(a.Caption)
                                        d.liveResponseLbl = lbl
                                        d.activityBox.Add(row)
                                } else {
                                        d.liveResponseLbl.SetText(clipStrMemory(a.Caption, 120))
                                }
                        } else if a.Type == "reasoning" {
                                // v1.0.2: reasoning streams into its own live
                                // row (dimmed), separate from the response.
                                if d.liveReasoningLbl == nil {
                                        row, lbl := newLiveReasoningRow(a.Caption)
                                        d.liveReasoningLbl = lbl
                                        d.activityBox.Add(row)
                                } else {
                                        d.liveReasoningLbl.SetText(clipStrMemory(a.Caption, 120))
                                }
                        } else {
                                d.liveResponseLbl = nil // next stream opens a fresh row
                                d.liveReasoningLbl = nil
                                d.activityBox.Add(newActivityRowWidget(a))
                        }
                        d.activityBox.Refresh()
                        d.activityScroll.ScrollToBottom()
                }
                if a.Type == "error" || a.Type == "done" {
                        d.setRunning(false, a.Caption)
                }
        })
}

// flushStreamFrame applies ONE coalesced streaming frame to the UI. Called
// on the UI thread by the pacer. Empty channels are skipped so a frame with
// only reasoning movement does not rewrite the response label (and vice
// versa).
func (d *desktopApp) flushStreamFrame(resp, reason string, tps float64) {
        // The status line: live tokens/sec while the text pours in — the
        // speed HUD users previously only saw after the turn now reads
        // live, exactly where the activity caption lives.
        if tps > 0 {
                d.activityLbl.SetText(fmt.Sprintf("Streaming… %.0f tok/s", tps))
        }
        if !d.cfg.ProMode {
                return
        }
        d.activitySection.Show()
        if reason != "" {
                if d.liveReasoningLbl == nil {
                        row, lbl := newLiveReasoningRow(reason)
                        d.liveReasoningLbl = lbl
                        d.activityBox.Add(row)
                } else {
                        d.liveReasoningLbl.SetText(clipStrMemory(reason, 120))
                }
        }
        if resp != "" {
                if d.liveResponseLbl == nil {
                        row, lbl := newLiveResponseRow(resp)
                        d.liveResponseLbl = lbl
                        d.activityBox.Add(row)
                } else {
                        d.liveResponseLbl.SetText(clipStrMemory(resp, 120))
                }
        }
        d.activityBox.Refresh()
        d.activityScroll.ScrollToBottom()
}

// firstLine returns the first line of s (clipped to n bytes).
func firstLine(s string, n int) string {
        if i := strings.IndexByte(s, '\n'); i >= 0 {
                s = s[:i]
        }
        if len(s) > n {
                s = s[:n]
        }
        return s
}

// setRunning toggles all live-run indicators with smooth transitions.
// Always executed on the main UI thread.
func (d *desktopApp) setRunning(running bool, caption string) {
        runOnMain(func() {
                d.setRunningMain(running, caption)
        })
}

func (d *desktopApp) setRunningMain(running bool, caption string) {
        d.turnRunning = running
        if running {
                // v1.0.9: a fresh turn starts a clean streaming frame session.
                if d.stream != nil {
                        d.stream.Stop()
                        d.stream = nil
                }
                if d.sendCirc != nil {
                        d.sendCirc.SetEnabled(false)
                }
                d.abortBtn.Show()
                // The status strip (flame + dots + caption + Stop) is always part of
                // the activity section; show the section itself so minimal users see
                // the one-line status even without Pro mode.
                d.activitySection.Show()
                d.flame.Show()
                if d.flameStop == nil {
                        d.flameStop = pulse(d.flame, 1100*time.Millisecond, 0.35)
                }
                d.dots.show()
                d.breathLine.Show()
                // Layout AFTER every visibility flip: Fyne does not re-run a
                // container's layout when a child is shown, so the dock must be
                // refreshed once all indicators are visible.
                d.refreshBottom()
        } else {
                // v1.0.9: the turn is over — park the frame pacer after a final
                // flush so no tail text is lost.
                if d.stream != nil {
                        d.stream.Stop()
                        d.stream = nil
                }
                if d.sendCirc != nil {
                        d.sendCirc.SetEnabled(true)
                }
                d.abortBtn.Hide()
                if d.flameStop != nil {
                        d.flameStop()
                        d.flameStop = nil
                }
                d.flame.Translucency = 0
                d.flame.Refresh()
                d.flame.Hide()
                d.dots.hide()
                d.breathLine.Hide()
                // In minimal mode the whole section collapses back after a run; in
                // Pro mode it stays for inspection.
                if !d.cfg.ProMode {
                        d.activitySection.Hide()
                } else if len(d.activities) == 0 {
                        d.activitySection.Hide()
                }
                d.refreshBottom()
                if caption != "" {
                        d.activityLbl.SetText(clipStrMemory(caption, 140))
                }
        }
}
