// Package ui is the native Fyne desktop UI for SHEYTAN-Local-Agent v1.0.0.
// No browser, no WebView — pure native widgets in the Ember Minimal theme.
//
// v1.0.0 layout ("chat.z.ai template", fire-flavored):
//
//      ┌──────────────┬────────────────────────────────────┬─────────────┐
//      │ 🔥 SHEYTAN   │ What shall we forge today?         │ Pro dock    │
//      │ [+ New chat] │   (suggestion cards / messages)    │ (pro only)  │
//      │ [search…]    │                                    │ Context     │
//      │ · sessions   │  ┌ status line · thinking… [Stop] ┐ │ Params      │
//      │ · ──────     │  └ composer          (send ⬤) ────┘ │ System/Tools│
//      │ Charts       │                                    │ Live logs   │
//      │ Memory (pro) ├────────────────────────────────────┤             │
//      │ Logs (pro)   │ footer: engine · updates · v1.0.0  │             │
//      │ ──────       │                                    │             │
//      │ Pro [switch] │                                    │             │
//      │ ⚙ Settings   │                                    │             │
//      └──────────────┴────────────────────────────────────┴─────────────┘
//
// Minimal by default: normal users see chat + sessions + charts only;
// background processing is summarized in ONE status line. Flipping Pro
// reveals memory/logs views, the right dock, the full activity stream and
// live engine logs.
package ui

import (
        "context"
        "fmt"
        "image/color"
        "os"
        "path/filepath"
        "runtime"
        "sort"
        "strings"
        "sync"
        "time"

        "fyne.io/fyne/v2"
        "fyne.io/fyne/v2/canvas"
        "fyne.io/fyne/v2/container"
        "fyne.io/fyne/v2/dialog"
        "fyne.io/fyne/v2/layout"
        "fyne.io/fyne/v2/storage"
        "fyne.io/fyne/v2/widget"

        "github.com/sheytan/local-agent/internal/agent"
        "github.com/sheytan/local-agent/internal/artifacts"
        "github.com/sheytan/local-agent/internal/brand"
        "github.com/sheytan/local-agent/internal/chunking"
        "github.com/sheytan/local-agent/internal/config"
        "github.com/sheytan/local-agent/internal/continuum"
        "github.com/sheytan/local-agent/internal/llm"
        "github.com/sheytan/local-agent/internal/logging"
        "github.com/sheytan/local-agent/internal/memory"
        "github.com/sheytan/local-agent/internal/multiagent"
        "github.com/sheytan/local-agent/internal/native"
        "github.com/sheytan/local-agent/internal/netcheck"
        "github.com/sheytan/local-agent/internal/proc"
        "github.com/sheytan/local-agent/internal/recall"
        agentrt "github.com/sheytan/local-agent/internal/runtime"
        "github.com/sheytan/local-agent/internal/sandbox"
        "github.com/sheytan/local-agent/internal/screen"
        "github.com/sheytan/local-agent/internal/sessions"
        "github.com/sheytan/local-agent/internal/sysinfo"
        "github.com/sheytan/local-agent/internal/updater"
        "github.com/sheytan/local-agent/internal/vision"
)

// runOnMain schedules fn on the main UI thread when a Fyne app is running.
// Every widget mutation triggered from a goroutine MUST go through here —
// calling SetText/Refresh/Show from the agent goroutine races the render
// loop and produces text and widgets painting over each other (v0.9.1).
//
// v1.0.8: fn runs under a panic guard — a faulty deferred mutation can no
// longer take the whole app down (crash-file + log instead of a vanished
// window). This is the central chokepoint every goroutine→UI hop passes
// through, so one wrapper guards them all.
func runOnMain(fn func()) {
        wrapped := func() {
                defer func() {
                        if r := recover(); r != nil {
                                recoverPanic("runOnMain", r)
                        }
                }()
                fn()
        }
        if a := fyne.CurrentApp(); a != nil {
                fyne.Do(wrapped)
                return
        }
        wrapped()
}

// desktopApp is the single instance — it owns everything.
type desktopApp struct {
        cfg    *config.Config
        fyne   fyne.App
        win    fyne.Window
        store  *sessions.Store
        client *llm.Client
        orch   *agent.Orchestrator
        multi  *multiagent.MultiAgent
        mem    *memory.Store
        llama  *llm.LlamaServer
        sb     *sandbox.CodeExecSandbox
        stack  *agentrt.Stack
        recall *recall.Engine // v1.0.2 persistent memory over past chats

        mu       sync.Mutex
        active   *sessions.Session
        sessions []*sessions.Session

        // UI elements
        sessionsList   *widget.List
        msgInput       *widget.Entry
        sendBtn        *widget.Button
        sendCirc       *sendButton
        abortBtn       *widget.Button
        activityLbl    *widget.Label
        logsList       *widget.List
        sysPromptEntry *widget.Entry
        maxIterEntry   *widget.Entry

        // Chat stream (bubbles in a VScroll+VBox — variable heights, no overlap)
        chatBox    *fyne.Container
        chatScroll *container.Scroll
        chatEmpty  *fyne.Container // v1.0.0: the empty-state hero (label + cards)

        // Activity stream (pro)
        activities      []agent.Activity
        activityBox     *fyne.Container
        activityScroll  *container.Scroll
        activitySection *fyne.Container
        chatBottom      *fyne.Container
        // v1.0.1 perf: the single live row that carries streaming response
        // text in Pro mode (updated in place instead of one row per delta).
        liveResponseLbl *widget.Label
        // v1.0.2: the live streaming THINKING row (Pro mode), separate from
        // the response row.
        liveReasoningLbl *widget.Label

        // v1.0.2 composer state
        attachBtn    *composerButton          // 📎 file attach
        camBtn       *composerButton          // v1.0.6 📷 capture screen
        toolsBtn     *composerButton          // tool selection
        thinkBtn     *composerButton          // thinking-mode toggle
        attachRow    *container.Scroll        // staged attachment chips
        toolsChecks  map[string]*widget.Check // pro-dock tool toggles
        pendingFiles []string                 // staged attachment paths

        // v1.0.6: Terminal view + Resources view + vision pill
        term          *terminalView // built-in Linux-like terminal
        resView       *resourcesView
        visionPill    *pill             // VISION badge when the engine sees images
        visionPillObj fyne.CanvasObject // the pill's canvas (show/hide target)

        // v1.0.7: Continuum context engine surfaces
        ctxMeter    *contextMeter   // live pressure gauge above the composer
        rolling     bool            // rollover-in-progress guard
        turnRunning bool            // a turn is in flight (blocks manual rollover)
        lastUsage   continuum.Usage // last observed usage (turn result or session scan)

        // v1.0.9 (TURBINE): frame-paced streaming — tokens coalesce into one
        // UI batch per display frame (default 120 fps).
        stream *streamPacer

        // Memory view
        memEntries []memory.Entry
        memBox     *fyne.Container
        memScroll  *container.Scroll
        memEmpty   *widget.Label

        // Live agent indicators
        flame      *canvas.Image
        flameStop  func()
        dots       *typingDots
        breathLine *canvas.Rectangle
        breathStop func()

        // v1.0.0 chrome
        navRows    map[string]*navRow
        modelChip  *chipButton
        netPill    *pill
        proSwitch  *widget.Check
        headerLbl  *canvas.Text
        footerLbl  *widget.Label
        engineLbl  *widget.Label
        updateLbl  *widget.Label
        proSection *fyne.Container // sidebar pro nav rows (memory/logs)
        proDock    fyne.CanvasObject
        mainSplit  *container.Split
        // activeModelDialog is the open model picker (hidden the moment a model
        // is picked — the v0.9 "click does nothing" bug).
        activeModelDialog *dialog.CustomDialog

        // Views & navigation
        centerStack *fyne.Container
        fader       *crossFader
        views       map[string]fyne.CanvasObject
        currentView string

        // Data view
        chartGrid  *widget.GridWrap
        chartFiles []chartFile
        chartEmpty *widget.Label

        // v1.0.4 Files (artifacts) view + tracker
        tracker       *artifacts.Tracker
        fileList      *widget.List
        fileArtifacts []artifacts.Artifact
        fileEmpty     *widget.Label

        // v1.0.4 Models view (model manager + storage + speed pack)
        modelsBox    *fyne.Container
        modelsScroll *container.Scroll
        modelsEmpty  *widget.Label

        // v1.0.4 speed HUD: last turn's tokens/sec line (footer, right side)
        perfLbl *widget.Label

        // Live state
        logLines    []string
        abortCancel context.CancelFunc
        // v1.0.3: renderingHistory suppresses the bubble entrance animation
        // while re-rendering an existing conversation.
        renderingHistory bool
}

// --- menu ---

func (d *desktopApp) buildMenu() *fyne.MainMenu {
        return fyne.NewMainMenu(
                fyne.NewMenu("File",
                        fyne.NewMenuItem("New chat", d.newSession),
                        fyne.NewMenuItem("Open app folder", func() { d.openInExplorer(d.cfg.DataDir) }),
                        fyne.NewMenuItem("Open models dir", func() { d.openInExplorer(d.cfg.ModelsDir) }),
                        fyne.NewMenuItem("Open charts dir", func() { d.openInExplorer(d.cfg.ChartsDir()) }),
                        fyne.NewMenuItemSeparator(),
                        fyne.NewMenuItem("Quit", func() { d.fyne.Quit() }),
                ),
                fyne.NewMenu("Agent",
                        fyne.NewMenuItem("Choose model…", d.pickModel),
                        fyne.NewMenuItem("LLM provider…", d.showProviderDialog),
                        fyne.NewMenuItem("Sampling preset…", d.pickPreset),
                        fyne.NewMenuItem("Multi-agent pipeline…", d.runPipeline),
                        fyne.NewMenuItemSeparator(),
                        fyne.NewMenuItem("Start llama.cpp engine", d.startLlama),
                        fyne.NewMenuItem("Stop llama.cpp engine", d.stopLlama),
                        fyne.NewMenuItemSeparator(),
                        fyne.NewMenuItem("Run stress tests", d.runStress),
                ),
                fyne.NewMenu("Tools",
                        fyne.NewMenuItem("Command palette (Ctrl+K)", d.showPalette),
                        fyne.NewMenuItem("Capture screen & ask", d.captureScreen),
                        fyne.NewMenuItem("Open Terminal", func() { d.showView("terminal") }),
                        fyne.NewMenuItem("Open Resources", func() { d.showView("resources") }),
                        fyne.NewMenuItemSeparator(),
                        fyne.NewMenuItem("Settings…", d.showSettings),
                        fyne.NewMenuItem("System info", d.showSysinfo),
                        fyne.NewMenuItem("Components / install", d.showComponents),
                        fyne.NewMenuItem("Check for engine update…", d.checkUpdatesNow),
                        fyne.NewMenuItem("Export diagnostics…", d.exportDiagnostics),
                ),
                fyne.NewMenu("Help",
                        fyne.NewMenuItem("About "+brand.Trademark, d.showAbout),
                        fyne.NewMenuItem("License", d.showLicense),
                ),
        )
}

// --- root assembly ---

func (d *desktopApp) buildRoot() fyne.CanvasObject {
        // --- Sidebar ---
        sidebar := d.buildSidebar()

        // --- Center views ---
        d.views["chat"] = d.buildChatView()
        d.views["data"] = d.buildDataView()
        d.views["files"] = d.buildFilesView()
        d.views["models"] = d.buildModelsView()
        d.views["memory"] = d.buildMemoryView()
        d.views["logs"] = d.buildLogsView()
        // v1.0.6: Terminal (built-in Linux-like console) + Resources
        d.views["terminal"] = d.buildTerminalView()
        d.views["resources"] = d.buildResourcesView()
        d.centerStack = container.NewStack(d.views["chat"])
        d.currentView = "chat"
        if nr, ok := d.navRows["chat"]; ok {
                nr.SetActive(true)
        }
        centerArea := container.NewStack(d.centerStack, d.fader.veil)

        // --- Header + center + footer = main column ---
        header := d.buildHeader()
        footer := d.buildFooter()
        mainCol := container.NewBorder(header, footer, nil, nil, centerArea)

        // --- Pro dock (right, only in pro mode) ---
        // v1.0.5: an invisible floor object pins the dock's MINIMUM width —
        // container.Split clamps its divider so neither pane drops below its
        // MinSize, which means the Context/Params/System/Tools tabs can never
        // be squeezed into an unusable sliver again (the "tabs too small"
        // report: the dock's scroll-based tabs report a tiny MinSize on their
        // own, so the splitter could crush them to ~150px).
        dockFloor := canvas.NewRectangle(color.Transparent)
        dockFloor.SetMinSize(fyne.NewSize(380, 460))
        d.proDock = container.NewStack(dockFloor, container.NewPadded(d.buildRightTabs()))
        if !d.cfg.ProMode {
                d.proDock.Hide() // Split gives hidden children zero space
        }
        d.mainSplit = container.NewHSplit(mainCol, d.proDock)
        d.mainSplit.SetOffset(0.74)

        body := container.NewHSplit(sidebar, d.mainSplit)
        body.SetOffset(0.185)

        return container.NewBorder(nil, nil, nil, nil, body)
}

// buildSidebar renders the left column: brand, new chat, search, sessions,
// navigation rows, pro toggle and settings.
func (d *desktopApp) buildSidebar() fyne.CanvasObject {
        logo := canvas.NewImageFromResource(Logo)
        logo.FillMode = canvas.ImageFillContain
        logo.SetMinSize(fyne.NewSize(26, 26))
        brandTxt := canvas.NewText(brand.Trademark, ColText)
        brandTxt.TextSize = 17
        brandTxt.TextStyle.Bold = true
        verTxt := canvas.NewText("v"+config.AppVersion, ColTextMuted)
        verTxt.TextSize = 11
        brandRow := container.NewHBox(
                container.NewPadded(logo),
                container.NewPadded(brandTxt),
                container.NewPadded(verTxt),
        )

        newBtn := newFireButton("New chat", "plus", d.newSession)

        d.sessionsList = widget.NewList(
                func() int { return len(d.sessions) },
                func() fyne.CanvasObject { return container.NewPadded(newSessionRow()) },
                func(idx widget.ListItemID, item fyne.CanvasObject) {
                        if idx < 0 || idx >= len(d.sessions) {
                                return
                        }
                        row := item.(*fyne.Container).Objects[0].(*sessionRow)
                        s := d.sessions[idx]
                        isActive := d.active != nil && s.ID == d.active.ID
                        meta := fmt.Sprintf("%s · %d msg", humanize(s.UpdatedAt), s.MessageCount())
                        if s.Chapter > 1 {
                                meta = fmt.Sprintf("CH %d · %s", s.Chapter, meta)
                        }
                        row.Set(clipStrMemory(s.Title, 26), meta, isActive)
                },
        )
        d.sessionsList.OnSelected = func(id widget.ListItemID) {
                if id < 0 || id >= len(d.sessions) {
                        return
                }
                // v1.0.2: List() entries are meta-index stubs — load the full
                // session on selection (only the clicked one pays the cost).
                picked := d.sessions[id]
                if full, err := d.store.Get(picked.ID); err == nil {
                        d.active = full
                } else {
                        d.active = picked
                }
                d.renderActive()
                d.sessionsList.Refresh()
                // v1.0.0 dead-click fix: deselect so clicking the SAME session again
                // still fires (and no row stays stuck highlighted).
                d.sessionsList.UnselectAll()
        }

        search := widget.NewEntry()
        search.SetPlaceHolder("Search sessions…")
        search.OnChanged = func(s string) {
                d.sessions, _ = d.store.List()
                if s != "" {
                        var filtered []*sessions.Session
                        for _, sess := range d.sessions {
                                if strings.Contains(strings.ToLower(sess.Title), strings.ToLower(s)) {
                                        filtered = append(filtered, sess)
                                }
                        }
                        d.sessions = filtered
                }
                d.sessionsList.Refresh()
        }

        nav := func(name, label string, fn func()) *navRow {
                nr := newNavRow(name, label, fn)
                d.navRows[name] = nr
                return nr
        }

        // Pro-only navigation (memory / logs) — hidden in minimal mode.
        d.proSection = container.NewVBox(
                nav("memory", "Memory", func() { d.showView("memory") }).obj,
                nav("logs", "Logs", func() { d.showView("logs") }).obj,
        )
        if !d.cfg.ProMode {
                d.proSection.Hide()
        }

        proLbl := canvas.NewText("Pro mode", ColTextMuted)
        proLbl.TextSize = 12
        d.proSwitch = widget.NewCheck("", func(on bool) { d.setProMode(on) })
        d.proSwitch.SetChecked(d.cfg.ProMode)
        proRow := container.NewHBox(
                container.NewPadded(canvasNewIconMuted("panel")),
                container.NewPadded(proLbl),
                layout.NewSpacer(),
                container.NewPadded(d.proSwitch),
        )

        settingsRow := newNavRow("settings", "Settings", d.showSettings)

        sidebar := container.NewBorder(
                container.NewVBox(
                        container.NewPadded(brandRow),
                        container.NewPadded(newBtn),
                        container.NewPadded(search),
                ),
                container.NewVBox(
                        widget.NewSeparator(),
                        container.NewPadded(nav("models", "Models", func() { d.showView("models") }).obj),
                        container.NewPadded(nav("files", "Files", func() { d.showView("files") }).obj),
                        container.NewPadded(nav("data", "Charts", func() { d.showView("data") }).obj),
                        // v1.0.6: Terminal + Resources are first-class views.
                        container.NewPadded(nav("terminal", "Terminal", func() { d.showView("terminal") }).obj),
                        container.NewPadded(nav("resources", "Resources", func() { d.showView("resources") }).obj),
                        d.proSection,
                        widget.NewSeparator(),
                        proRow,
                        container.NewPadded(settingsRow.obj),
                ),
                nil, nil,
                container.NewPadded(d.sessionsList),
        )
        bg := canvas.NewRectangle(ColBgDeep)
        return container.NewStack(bg, sidebar)
}

// canvasNewIconMuted is a small helper for inline muted icons.
func canvasNewIconMuted(name string) fyne.CanvasObject {
        ic := canvas.NewImageFromResource(iconMuted(name))
        ic.SetMinSize(fyne.NewSize(15, 15))
        return ic
}

// buildHeader renders the slim top bar: view title, model chip (ALWAYS
// clickable — this is the model picker entry point), net pill, pro toggle.
// v1.0.3: the chip shows the SELECTED LOCAL MODEL (no phantom default) or a
// "Choose model" invitation when the models folder is empty.
func (d *desktopApp) buildHeader() fyne.CanvasObject {
        d.headerLbl = canvas.NewText("Chat", ColText)
        d.headerLbl.TextSize = 16
        d.headerLbl.TextStyle.Bold = true

        // v1.0.0: the model selector is a real interactive chip. In v0.9 the
        // model pill was pure canvas — clicking it did nothing.
        modelLabel := d.cfg.DisplayModel()
        if !d.cfg.IsRemote() && modelLabel == "" {
                if models := llm.ListLocalModels(d.cfg.ModelsDir); len(models) > 0 {
                        modelLabel = strings.TrimSuffix(models[0], ".gguf")
                } else {
                        modelLabel = "Choose model"
                }
        }
        d.modelChip = newChipButton("model", clipStrMemory(modelLabel, 24), d.pickModel)
        d.netPill = newPill("…", ColText, ColBgRaised)
        overflow := widget.NewButtonWithIcon("", icon("menu"), d.showSettings)
        overflow.Importance = widget.LowImportance

        left := container.NewHBox(
                container.NewPadded(d.headerLbl),
        )
        right := container.NewHBox(
                container.NewPadded(d.modelChip),
                container.NewPadded(d.netPill.canvas()),
                container.NewPadded(overflow),
        )
        bar := container.NewBorder(nil, nil, left, right, nil)
        line := canvas.NewRectangle(ColBorderSoft)
        line.SetMinSize(fyne.NewSize(2, 1))
        return container.NewVBox(container.NewPadded(bar), line)
}

// buildFooter renders the slim status footer: engine state, update state,
// version. NOTE: no Truncation on these labels — a Label with
// TextTruncateClip reports a collapsed ~one-glyph MinSize in Fyne 2.8, so
// the Border layout stops reserving room and the text clips to "SHE…"
// (found by pixel audit). Text length is bounded in code instead.
func (d *desktopApp) buildFooter() fyne.CanvasObject {
        d.engineLbl = widget.NewLabel("engine: —")
        d.engineLbl.TextStyle = fyne.TextStyle{Italic: true}
        d.engineLbl.Importance = widget.LowImportance
        d.updateLbl = widget.NewLabel("")
        d.updateLbl.TextStyle = fyne.TextStyle{Italic: true}
        d.updateLbl.Importance = widget.LowImportance
        // v1.0.4: live speed HUD — last turn's tokens/sec + time-to-first-
        // token, the number local-LLM users watch first (LM Studio parity).
        d.perfLbl = widget.NewLabel("")
        d.perfLbl.TextStyle = fyne.TextStyle{Bold: true}
        d.perfLbl.Importance = widget.MediumImportance
        // v1.0.6: VISION pill — lit when the engine carries a multimodal
        // projector and can see images/screenshots.
        d.visionPill = newPill("VISION", color.NRGBA{R: 255, G: 138, B: 80, A: 255}, color.NRGBA{R: 44, G: 20, B: 14, A: 255})
        d.visionPillObj = d.visionPill.canvas()
        d.visionPillObj.Hide()
        ver := widget.NewLabel(brand.Trademark + " v" + config.AppVersion)
        ver.TextStyle = fyne.TextStyle{Bold: true}
        line := canvas.NewRectangle(ColBorderSoft)
        line.SetMinSize(fyne.NewSize(2, 1))
        foot := container.NewBorder(nil, nil,
                container.NewHBox(container.NewPadded(d.engineLbl), container.NewPadded(d.updateLbl), container.NewPadded(d.visionPillObj)),
                container.NewHBox(container.NewPadded(d.perfLbl), container.NewPadded(ver)),
                nil,
        )
        return container.NewVBox(line, container.NewPadded(foot))
}

// setProMode flips the UI between minimal and professional density.
func (d *desktopApp) setProMode(on bool) {
        d.cfg.ProMode = on
        _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        logging.Default().Info("gui", "pro mode: %v", on)
        if on {
                d.proSection.Show()
                d.proDock.Show()
                d.mainSplit.SetOffset(0.74)
        } else {
                d.proSection.Hide()
                // Split gives a hidden trailing child zero space (Hide, not offset).
                d.proDock.Hide()
                // Leaving a pro-only view while in minimal mode lands back on chat.
                if d.currentView == "memory" || d.currentView == "logs" {
                        d.showView("chat")
                }
        }
        d.proSection.Refresh()
        d.mainSplit.Refresh()
        if d.activitySection != nil {
                if on {
                        if len(d.activities) > 0 {
                                d.activitySection.Show()
                        }
                } else {
                        d.activitySection.Hide()
                }
                d.refreshBottom()
        }
}

// showView switches the center view with a cross-fade transition.
func (d *desktopApp) showView(name string) {
        if d.currentView == name {
                return
        }
        view, ok := d.views[name]
        if !ok {
                return
        }
        d.fader.cover()
        for n, nr := range d.navRows {
                nr.SetActive(n == name)
        }
        d.centerStack.Objects = []fyne.CanvasObject{view}
        d.centerStack.Refresh()
        d.currentView = name
        titles := map[string]string{
                "chat": "Chat", "data": "Charts", "files": "Files", "models": "Models",
                "memory": "Memory", "logs": "Logs",
                "terminal": "Terminal", "resources": "Resources", // v1.0.6
        }
        if t, ok := titles[name]; ok && d.headerLbl != nil {
                d.headerLbl.Text = t
                d.headerLbl.Refresh()
        }
        switch name {
        case "data":
                d.refreshCharts()
        case "files":
                d.refreshFiles()
        case "models":
                d.refreshModels()
        case "logs":
                d.refreshLogs()
        case "memory":
                d.memEntries, _ = d.mem.All()
                d.renderMemory()
        case "resources":
                if d.resView != nil {
                        d.resView.refresh()
                }
        }
        d.fader.reveal(220 * time.Millisecond)
}

// fitWindowToScreen clamps the window to the visible screen (v1.0.5).
//
// The design default is 1340x840 logical pixels, which is perfect on a
// 1080p-class screen at 100% scaling — but on a 13-14" laptop running
// 125-150% display scaling the same logical size becomes physically larger
// than the panel, opening a clipped window. This runs once, right after the
// event loop starts (when the real canvas scale is known and the DPI-aware
// manifest has done its job), and shrinks the window to 94% of the logical
// screen when needed.
func (d *desktopApp) fitWindowToScreen() {
        if d.win == nil {
                return
        }
        devW, devH, ok := screenDevicePixels()
        if !ok || devW <= 0 || devH <= 0 {
                return
        }
        scale := d.win.Canvas().Scale()
        if scale <= 0 {
                scale = 1
        }
        logW, logH := devW/scale, devH/scale
        cur := d.win.Canvas().Size()
        maxW, maxH := logW*0.94, logH*0.94
        w, h := cur.Width, cur.Height
        changed := false
        if w > maxW {
                w, changed = maxW, true
        }
        if h > maxH {
                h, changed = maxH, true
        }
        if !changed {
                return
        }
        if w < 720 {
                w = 720 // never collapse below a usable width
        }
        if h < 480 {
                h = 480
        }
        d.win.Resize(fyne.NewSize(w, h))
        d.win.CenterOnScreen()
        logging.Default().Info("gui", "window fitted to screen: %.0fx%.0f logical (scale %.2f)", w, h, scale)
}

// --- connectivity ---

// setNetStatus updates the connectivity pill (ONLINE / OFFLINE).
func (d *desktopApp) setNetStatus(online bool) {
        if d.netPill == nil {
                return
        }
        if online {
                d.netPill.SetState("ONLINE", ColTextMuted, ColBgRaised)
        } else {
                d.netPill.SetState("OFFLINE", color.NRGBA{R: 0xFF, G: 0xE4, B: 0xD6, A: 0xFF},
                        color.NRGBA{R: 0x8C, G: 0x1D, B: 0x0E, A: 0xFF})
        }
}

// startNetWatcher probes connectivity now and then keeps watching. v1.0.3:
// ADAPTIVE cadence — while offline it re-probes every 10 seconds so a
// reconnect flips the pill to ONLINE within moments (the old fixed 45s
// cycle is why "offline never became online" felt broken); while online it
// relaxes to 45s. The probe itself is multi-strategy (see netcheck).
func (d *desktopApp) startNetWatcher() {
        go func() {
                prev := ""
                update := func() bool {
                        online := netcheck.Force()
                        runOnMain(func() { d.setNetStatus(online) })
                        st := "offline"
                        if online {
                                st = "online"
                        }
                        if st != prev {
                                logging.Default().Info("net", "connectivity: %s", st)
                                runOnMain(func() {
                                        if online {
                                                d.setStatus("Back online — web tools available again")
                                        } else {
                                                d.setStatus("Offline — local tools keep working")
                                        }
                                })
                                prev = st
                        }
                        return online
                }
                online := update()
                for {
                        // Fast retry while offline; relaxed while online.
                        wait := 45 * time.Second
                        if !online {
                                wait = 10 * time.Second
                        }
                        t := time.NewTicker(wait)
                        select {
                        case <-t.C:
                        }
                        t.Stop()
                        online = update()
                }
        }()
}

// startEngineWatcher polls the llama.cpp state and keeps the model chip dot
// + footer current (ready / loading / off / error).
func (d *desktopApp) startEngineWatcher() {
        if d.llama == nil {
                return
        }
        go func() {
                t := time.NewTicker(1500 * time.Millisecond)
                defer t.Stop()
                for range t.C {
                        st := d.llama.State()
                        runOnMain(func() { d.setEngineState(st) })
                }
        }()
        runOnMain(func() { d.setEngineState(d.llama.State()) })
}

var engineDotColors = map[string]color.Color{
        "running":     ColSuccess,
        "starting":    ColGold,
        "downloading": ColGold,
        "error":       ColDanger,
}

// setEngineState updates the chip dot and footer engine label. v1.0.3: the
// dot PULSES while the engine is starting/downloading so the boot is
// visibly alive.
func (d *desktopApp) setEngineState(state string) {
        if d.cfg.IsRemote() {
                if d.modelChip != nil {
                        d.modelChip.SetDot(ColSuccess)
                }
                if d.engineLbl != nil {
                        d.engineLbl.SetText("endpoint: " + clipStrMemory(d.cfg.RemoteBaseURL, 40))
                }
                return
        }
        if d.modelChip != nil {
                d.modelChip.SetDot(engineDotColors[state])
        }
        // v1.0.6: the VISION pill lights up when the running engine carries
        // a multimodal projector.
        if d.visionPillObj != nil {
                if d.llama != nil && d.llama.VisionActive() {
                        d.visionPillObj.Show()
                } else {
                        d.visionPillObj.Hide()
                }
        }
        if d.engineLbl != nil {
                switch state {
                case "running":
                        tag := updater.InstalledEngineTag(d.cfg)
                        if tag == "" {
                                tag = updater.DefaultEngineTag
                        }
                        label := "engine: llama.cpp " + tag + " · ready"
                        // v1.0.5: say it plainly when the engine had to fall
                        // back to a compatibility profile for this model.
                        if d.cfg.EngineCompat > 0 {
                                label += fmt.Sprintf(" · compat mode %d", d.cfg.EngineCompat)
                        }
                        d.engineLbl.SetText(label)
                case "starting":
                        d.engineLbl.SetText("engine: loading " + clipStrMemory(d.cfg.DisplayModel(), 24) + "…")
                case "downloading":
                        d.engineLbl.SetText("engine: downloading…")
                case "error":
                        d.engineLbl.SetText("engine: error (see Logs in Pro mode)")
                default:
                        d.engineLbl.SetText("engine: off (starts on first message)")
                }
        }
}

// startUpdateLoop launches the scheduled engine updater in the background
// (daily / weekly / monthly cadence from config).
func (d *desktopApp) startUpdateLoop() {
        ctx := context.Background()
        notify := func(msg string) {
                logging.Default().Info("updater", "%s", msg)
                runOnMain(func() {
                        if d.updateLbl != nil {
                                d.updateLbl.SetText(clipStrMemory(msg, 60))
                        }
                })
        }
        save := func() {
                _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        }
        go updater.RunScheduled(ctx, d.cfg, d.llama, notify, save)
}

// checkUpdatesNow runs one update check on demand (Tools menu / settings).
// v1.0.3: FORCED — the check ignores the schedule gate so clicking the
// button always actually checks (before, a recent automatic check made the
// button a no-op that said "up to date").
func (d *desktopApp) checkUpdatesNow() {
        go func() {
                runOnMain(func() { d.setStatus("Checking for engine update…") })
                msg, updated, err := updater.CheckAndApplyForced(context.Background(), d.cfg, d.llama)
                _ = config.Save(d.cfg.ConfigPath(), d.cfg)
                if err != nil {
                        logging.Default().Warn("updater", "%s", msg)
                }
                runOnMain(func() {
                        if d.updateLbl != nil {
                                d.updateLbl.SetText(clipStrMemory(msg, 60))
                        }
                        dialog.ShowInformation("Engine updates", msg, d.win)
                })
                _ = updated
        }()
}

// --- actions ---

func (d *desktopApp) newSession() {
        s := d.store.Create()
        d.sessions = append([]*sessions.Session{s}, d.sessions...)
        d.active = s
        d.renderActive()
        d.sessionsList.Refresh()
        if d.ctxMeter != nil {
                d.ctxMeter.Set(0, 1, 0, d.cfg.HistoryWindowTokens())
        }
        if d.currentView != "chat" {
                d.showView("chat")
        }
}

func (d *desktopApp) reloadSessions() {
        list, _ := d.store.List()
        d.sessions = list
        if d.sessionsList != nil {
                d.sessionsList.Refresh()
        }
}

func (d *desktopApp) sendMessage() {
        if d.active == nil {
                return
        }
        text := strings.TrimSpace(d.msgInput.Text)
        if text == "" && len(d.pendingFiles) == 0 {
                return
        }

        // v1.0.3: the engine runs the moment the user starts chatting — if the
        // local engine is not up yet, boot it (with a visible status) BEFORE the
        // first LLM call instead of failing with a connection error.
        if !d.cfg.IsRemote() && d.llama != nil && !d.llama.IsRunning() {
                models := llm.ListLocalModels(d.cfg.ModelsDir)
                if len(models) == 0 {
                        d.setStatus("No model selected — click the model chip above")
                        d.pickModel()
                        return
                }
                d.setRunning(true, "Starting engine (first chat)…")
                go func() {
                        if err := d.llama.EnsureRunning(); err != nil {
                                runOnMain(func() {
                                        d.setRunning(false, "")
                                        d.setStatus("Engine failed: " + clipStrMemory(err.Error(), 60))
                                        // v1.0.5: full engine stderr in a sized,
                                        // scrollable dialog — no more bare
                                        // "exit status 1" mysteries.
                                        msg := widget.NewLabel(err.Error())
                                        msg.Wrapping = fyne.TextWrapWord
                                        d.bigDialog("Could not start the llama.cpp engine",
                                                scrollDialogContent(container.NewPadded(msg), fyne.NewSize(600, 380)), 660, 460)
                                })
                                return
                        }
                        runOnMain(func() { d.sendMessage() }) // retry now that the engine is up
                }()
                return
        }

        // v1.0.2: compose the outgoing message with staged attachments —
        // text files (.txt/.md guaranteed + every other text/code format)
        // inlined within the byte budget, binaries noted as metadata.
        // v1.0.6: IMAGE attachments split out of the text pipeline — they
        // ride the message as Images (the multimodal client turns them into
        // image_url parts for the vision encoder) instead of becoming
        // metadata notes.
        attached := append([]string{}, d.pendingFiles...)
        composed, images := chunking.ComposeWithImages(text, attached, d.cfg.AttachmentsBudgetBytes())
        var attachNames []string
        for _, p := range attached {
                if !vision.IsImageFile(p) {
                        attachNames = append(attachNames, filepath.Base(p))
                }
        }

        d.msgInput.SetText("")
        d.setPendingFiles(nil)
        d.clearActivities()
        now := time.Now()
        d.appendBubble(bubbleInfo{
                Role:        "user",
                Content:     textOrAttachmentNote(text, attachNames),
                Attachments: attachNames,
                Images:      images,
                At:          now,
                OnZoom:      d.zoomImage,
        })
        d.setRunning(true, "Thinking…")

        // Append to session (full composed content for the LLM + display
        // metadata for the UI).
        d.active.Messages = append(d.active.Messages, llm.Message{
                Role:        "user",
                Content:     composed,
                Attachments: attachNames,
                Images:      images,
                At:          now,
        })
        if len(d.active.Messages) == 1 {
                title := text
                if title == "" {
                        title = strings.Join(attachNames, ", ")
                }
                if len(title) > 60 {
                        title = title[:60] + "…"
                }
                d.active.Title = title
                _ = d.store.UpdateTitle(d.active.ID, title)
        }
        _ = d.store.Save(d.active)

        // Build LLM messages
        var msgs []llm.Message
        if d.active.Context.SystemPrompt != "" {
                msgs = append(msgs, llm.Message{Role: "system", Content: d.active.Context.SystemPrompt})
        }
        msgs = append(msgs, d.active.Messages...)
        d.orch.SetSessionID(d.active.ID)
        logging.Default().Info("gui", "turn start (session=%s, msgs=%d, attachments=%d)", d.active.ID, len(msgs), len(attached))

        ctx, cancel := context.WithCancel(context.Background())
        d.mu.Lock()
        d.abortCancel = cancel
        d.mu.Unlock()

        // v1.0.4: fingerprint the artifact folders so every file this turn
        // creates is detected the moment the turn ends.
        if d.tracker != nil {
                d.tracker.BeginTurn()
        }

        go func() {
                start := time.Now()
                defer func() {
                        d.mu.Lock()
                        d.abortCancel = nil
                        d.mu.Unlock()
                        d.setRunning(false, "")
                }()
                res, err := d.orch.RunDetailed(ctx, msgs, func(a agent.Activity) {
                        d.appendActivity(a)
                })
                runOnMain(func() {
                        if err != nil {
                                d.appendActivity(agent.Activity{Type: "error", Caption: "Error: " + err.Error(), Timestamp: time.Now()})
                                d.setRunning(false, "Error — open Pro mode → Logs for details")
                                return
                        }
                        if res.Text != "" {
                                d.active.Messages = append(d.active.Messages, llm.Message{
                                        Role:      "assistant",
                                        Content:   res.Text,
                                        Reasoning: res.Reasoning,
                                        At:        time.Now(),
                                })
                                _ = d.store.Save(d.active)
                                replyIdx := len(d.active.Messages) - 1
                                // v1.0.7: pin feedback to THIS session — a
                                // chapter rollover swaps d.active under the
                                // bubbles, and an unpinned index would land on
                                // the wrong message in the new chapter.
                                fbSess := d.active.ID
                                d.appendBubble(bubbleInfo{
                                        Role:       "assistant",
                                        Content:    res.Text,
                                        Reasoning:  res.Reasoning,
                                        At:         time.Now(),
                                        OnFeedback: func(fb int) { d.setFeedbackIn(fbSess, replyIdx, fb) },
                                })
                        }
                        // v1.0.4: speed HUD — tokens/sec + TTFT of the final
                        // generation, exactly what a local-LLM power user wants
                        // to see after every reply.
                        if d.cfg.ShowPerfHUD && res.Perf != "" && d.perfLbl != nil {
                                d.perfLbl.SetText(res.Perf)
                        }
                        d.setEngineState(d.llama.State())

                        // v1.0.7 Continuum: record the turn's peak prompt
                        // pressure, then check for a chapter rollover — the
                        // new session is created in the background BEFORE the
                        // user types the next word.
                        if res.ContextUsage.BudgetTokens > 0 {
                                d.lastUsage = res.ContextUsage
                        }
                        d.maybeRollover()

                        // v1.0.4: artifacts — everything the turn created,
                        // surfaced as chips right under the reply.
                        if d.tracker != nil {
                                if arts := d.tracker.EndTurn(); len(arts) > 0 {
                                        d.appendArtifactChips(arts)
                                        if len(arts) == 1 {
                                                d.setStatus("Created " + arts[0].Name + " — see Files")
                                        } else {
                                                d.setStatus(fmt.Sprintf("Created %d files — see Files", len(arts)))
                                        }
                                        d.refreshFiles()
                                }
                        }
                })
                // v1.0.2: index the completed exchange into persistent
                // recall so future turns can find it without re-feeding the
                // conversation into the context window.
                if d.recall != nil && res.Text != "" {
                        _ = d.recall.IndexTurn(d.active.ID, d.active.Title, textOrAttachmentNote(text, attachNames), res.Text, res.ToolsUsed)
                }
                _ = start
        }()
}

// textOrAttachmentNote renders what the user bubble shows when the typed
// text is empty but files are attached.
func textOrAttachmentNote(text string, attachments []string) string {
        if text != "" || len(attachments) == 0 {
                return text
        }
        return "📎 " + strings.Join(attachments, ", ")
}

// --- v1.0.6: screen capture → vision attachment ---

// captureScreen grabs the primary display, saves it into the screenshots
// folder and stages it as a message attachment — the user's "look at my
// screen" button. When the engine cannot see images yet the capture still
// works, but the status line teaches the mmproj path instead of failing.
func (d *desktopApp) captureScreen() {
        if !screen.Supported() {
                d.setStatus("Screen capture is available in the Windows build")
                return
        }
        png, err := screen.CapturePNG(0)
        if err != nil {
                d.setStatus("Capture failed: " + clipStrMemory(err.Error(), 60))
                return
        }
        shots := d.cfg.ScreenshotsDir()
        if err := os.MkdirAll(shots, 0o755); err != nil {
                d.setStatus("Capture failed: " + err.Error())
                return
        }
        path := filepath.Join(shots, "screen-"+time.Now().Format("20060102-150405")+".png")
        if err := os.WriteFile(path, png, 0o644); err != nil {
                d.setStatus("Capture failed: " + err.Error())
                return
        }
        if d.tracker != nil {
                d.tracker.Report(path)
        }
        d.addPendingFile(path)
        if d.llama != nil && !d.llama.VisionActive() && !d.cfg.IsRemote() {
                d.setStatus("Screen captured. Add an mmproj-*.gguf projector to models/ for the engine to SEE it")
        } else {
                d.setStatus("Screen captured — describe what you want to know about it")
        }
}

func (d *desktopApp) abortRun() {
        d.mu.Lock()
        cancel := d.abortCancel
        d.mu.Unlock()
        if cancel != nil {
                cancel()
        }
        d.orch.Abort()
}

// --- v1.0.2: file attachments, tool selection, thinking mode ---

// attachFilterList is the picker's type filter: everything the models and
// the chunking layer can work with, grouped into friendly buckets.
var attachFilterList = []native.FileFilter{
        {
                Label:   "Documents — text, markdown, code, data",
                Pattern: "*.txt;*.md;*.markdown;*.mdx;*.csv;*.tsv;*.json;*.jsonl;*.yaml;*.yml;*.toml;*.ini;*.cfg;*.conf;*.env;*.xml;*.html;*.htm;*.css;*.scss;*.js;*.mjs;*.cjs;*.ts;*.tsx;*.jsx;*.vue;*.svelte;*.go;*.py;*.pyi;*.rs;*.java;*.kt;*.kts;*.scala;*.swift;*.dart;*.c;*.h;*.cpp;*.cc;*.hpp;*.cs;*.rb;*.php;*.pl;*.lua;*.r;*.m;*.sql;*.sh;*.bash;*.zsh;*.fish;*.ps1;*.psm1;*.bat;*.cmd;*.log;*.diff;*.patch;*.srt;*.vtt;*.pdf;*.docx;*.xlsx;*.pptx",
        },
        {
                Label:   "Images",
                Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.bmp;*.svg",
        },
        {
                Label:   "Archives & binaries",
                Pattern: "*.zip;*.tar;*.gz;*.7z;*.exe;*.dll;*.bin;*.dat;*.gguf",
        },
        {
                Label:   "Media",
                Pattern: "*.ogg;*.mp3;*.mp4;*.wav",
        },
        {
                Label:   "All files",
                Pattern: "*.*",
        },
}

// attachFiles opens the file picker for chat attachments.
//
// v1.0.8 — THE ATTACHMENT-CRASH FIX. v1.0.7 opened Fyne's built-in dialog,
// which walks the filesystem from Go to populate its browser; on real
// Windows machines that walker panics on special folders (network drives,
// OneDrive junctions, empty card readers) and an uncaught panic closes the
// app — the reported "app closes when I attach" bug. The picker is now the
// OS's own dialog via a raw comdlg32 syscall (internal/native): no walker,
// native chrome, true multi-select, opens instantly. The Fyne dialog stays
// only as a non-Windows fallback.
func (d *desktopApp) attachFiles() {
        defer func() {
                if r := recover(); r != nil {
                        recoverPanic("attachFiles", r)
                        d.setStatus("Attachment dialog hiccup — recovered, nothing was lost")
                }
        }()

        // Native picker BLOCKS in a modal loop: run it off the UI thread so
        // our window keeps painting beneath the dialog.
        go func() {
                defer func() {
                        if r := recover(); r != nil {
                                recoverPanic("attachFiles:native", r)
                        }
                }()
                res := native.PickFiles("Attach files", attachFilterList, "")
                switch {
                case res.Err != nil:
                        // Native unavailable/failed — fall back to the toolkit dialog.
                        logging.Default().Warn("ui", "native picker unavailable (%v) — falling back", res.Err)
                        runOnMain(func() { d.attachFilesFyne() })
                        return
                case res.Canceled || len(res.Paths) == 0:
                        return
                }
                runOnMain(func() {
                        added := 0
                        for _, p := range res.Paths {
                                before := len(d.pendingFiles)
                                d.addPendingFile(p)
                                if len(d.pendingFiles) > before {
                                        added++
                                }
                        }
                        if added > 0 {
                                if added == 1 {
                                        d.setStatus("Attached " + filepath.Base(res.Paths[0]))
                                } else {
                                        d.setStatus(fmt.Sprintf("Attached %d files", added))
                                }
                        }
                })
        }()
}

// attachFilesFyne is the legacy fallback picker (non-Windows platforms and
// the unlikely case the native dialog fails to open). Kept minimal and
// wrapped in panic guards — if THIS ever panics the app survives and logs.
func (d *desktopApp) attachFilesFyne() {
        fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
                if err != nil {
                        d.setStatus("Attach failed: " + clipStrMemory(err.Error(), 60))
                        return
                }
                if reader == nil {
                        return // cancelled
                }
                uri := reader.URI()
                _ = reader.Close()
                if uri == nil {
                        return
                }
                path := uri.Path()
                if path == "" {
                        return
                }
                runOnMain(func() {
                        defer func() {
                                if r := recover(); r != nil {
                                        recoverPanic("attachFilesFyne:cb", r)
                                }
                        }()
                        d.addPendingFile(path)
                })
        }, d.win)
        fd.SetFilter(storage.NewExtensionFileFilter([]string{
                ".txt", ".md", ".markdown", ".mdx", ".csv", ".tsv", ".json", ".jsonl",
                ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".env", ".xml",
                ".html", ".htm", ".css", ".scss", ".js", ".mjs", ".cjs", ".ts",
                ".tsx", ".jsx", ".vue", ".svelte", ".go", ".py", ".pyi", ".rs",
                ".java", ".kt", ".kts", ".scala", ".swift", ".dart", ".c", ".h",
                ".cpp", ".cc", ".hpp", ".cs", ".rb", ".php", ".pl", ".lua", ".r",
                ".m", ".sql", ".sh", ".bash", ".zsh", ".fish", ".ps1", ".psm1",
                ".bat", ".cmd", ".log", ".diff", ".patch", ".srt", ".vtt",
                ".pdf", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg",
                ".zip", ".tar", ".gz", ".7z", ".exe", ".dll", ".bin", ".dat",
                ".docx", ".xlsx", ".pptx", ".ogg", ".mp3", ".mp4", ".wav",
        }))
        fd.Resize(fyne.NewSize(720, 480))
        fd.Show()
}

// addPendingFile stages one attachment (dedup + cap at 8 files).
func (d *desktopApp) addPendingFile(path string) {
        for _, p := range d.pendingFiles {
                if strings.EqualFold(p, path) {
                        d.setStatus("Already attached: " + filepath.Base(path))
                        return
                }
        }
        if len(d.pendingFiles) >= 8 {
                d.setStatus("Attachment limit reached (8 files per message)")
                return
        }
        d.pendingFiles = append(d.pendingFiles, path)
        d.renderPendingFiles()
}

// setPendingFiles replaces the staged list.
func (d *desktopApp) setPendingFiles(files []string) {
        d.pendingFiles = files
        d.renderPendingFiles()
}

// renderPendingFiles redraws the attachment tiles row above the composer.
// v1.0.3: icon-first TILES — one big type-specific glyph per file instead of
// the old tiny-icon text pills.
func (d *desktopApp) renderPendingFiles() {
        if d.attachRow == nil {
                return
        }
        inner := d.attachRow.Content.(*fyne.Container)
        inner.RemoveAll()
        for _, p := range d.pendingFiles {
                path := p
                name := filepath.Base(path)
                hint := "text"
                if !chunking.IsKnownTextExt(path) {
                        hint = "binary"
                }
                tile := newAttachTile(name, hint, true, func() {
                        var kept []string
                        for _, q := range d.pendingFiles {
                                if !strings.EqualFold(q, path) {
                                        kept = append(kept, q)
                                }
                        }
                        d.setPendingFiles(kept)
                })
                inner.Add(container.NewPadded(tile))
        }
        if len(d.pendingFiles) > 0 {
                d.attachRow.Show()
        } else {
                d.attachRow.Hide()
        }
        inner.Refresh()
        d.attachRow.Refresh()
        d.refreshBottom()
}

// toggleThinking flips thinking mode on/off for the next turns.
func (d *desktopApp) toggleThinking() {
        d.cfg.ThinkingMode = !d.cfg.ThinkingMode
        _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        if d.thinkBtn != nil {
                d.thinkBtn.SetActive(d.cfg.ThinkingMode)
        }
        state := "off"
        if d.cfg.ThinkingMode {
                state = "on"
        }
        d.setStatus("Thinking mode " + state)
        logging.Default().Info("gui", "thinking mode: %v", d.cfg.ThinkingMode)
}

// showToolsPopup opens the tool-selection popover anchored at the composer
// tools control: per-tool toggles plus quick presets.
func (d *desktopApp) showToolsPopup() {
        names := make([]string, 0, len(d.orch.Tools()))
        for name := range d.orch.Tools() {
                names = append(names, name)
        }
        sort.Strings(names)

        box := container.NewVBox()
        title := canvas.NewText("Tools the agent may use", ColTextMuted)
        title.TextSize = 12
        title.TextStyle.Bold = true
        box.Add(container.NewPadded(title))

        checks := container.NewVBox()
        var pop *widget.PopUp
        for _, name := range names {
                toolName := name
                chk := widget.NewCheck(toolName, func(on bool) {
                        d.setToolEnabled(toolName, on)
                })
                chk.SetChecked(d.cfg.ToolEnabled(toolName))
                checks.Add(container.NewPadded(chk))
        }
        box.Add(checks)

        allBtn := primaryButton("All", "check", func() {
                d.setAllTools(true)
                if pop != nil {
                        pop.Hide()
                }
        })
        localBtn := ghostButton("Local only", "sandbox", func() {
                d.setToolPreset([]string{"files", "shell", "codeExec", "dataAnalysis", "git", "memory"})
                if pop != nil {
                        pop.Hide()
                }
        })
        noneBtn := ghostButton("None", "close", func() {
                d.setAllTools(false)
                if pop != nil {
                        pop.Hide()
                }
        })
        box.Add(container.NewPadded(container.NewHBox(allBtn, localBtn, noneBtn)))

        pop = widget.NewPopUp(box, d.win.Canvas())
        pop.ShowAtPosition(fyne.NewPos(16, d.win.Content().Size().Height-260))
}

// setToolEnabled flips one tool and persists.
func (d *desktopApp) setToolEnabled(name string, on bool) {
        var list []string
        for _, n := range d.cfg.EnabledTools {
                if !strings.EqualFold(n, name) {
                        list = append(list, n)
                }
        }
        if on {
                list = append(list, name)
        }
        d.cfg.EnabledTools = normalizeToolList(d.orch, list)
        _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        if chk, ok := d.toolsChecks[name]; ok {
                chk.SetChecked(on)
        }
        d.updateToolsChip()
        logging.Default().Info("gui", "tool %s enabled=%v", name, on)
}

// setAllTools enables/disables every tool.
func (d *desktopApp) setAllTools(on bool) {
        if on {
                d.cfg.EnabledTools = nil
        } else {
                d.cfg.EnabledTools = []string{}
        }
        _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        for name, chk := range d.toolsChecks {
                chk.SetChecked(on)
                _ = name
        }
        d.updateToolsChip()
}

// setToolPreset restricts the agent to the given tool set.
func (d *desktopApp) setToolPreset(tools []string) {
        d.cfg.EnabledTools = normalizeToolList(d.orch, tools)
        _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        for name, chk := range d.toolsChecks {
                chk.SetChecked(d.cfg.ToolEnabled(name))
        }
        d.updateToolsChip()
}

// updateToolsChip refreshes the tools control tooltip/state.
func (d *desktopApp) updateToolsChip() {
        if d.toolsBtn == nil {
                return
        }
        total := len(d.orch.Tools())
        enabled := 0
        for name := range d.orch.Tools() {
                if d.cfg.ToolEnabled(name) {
                        enabled++
                }
        }
        if enabled == total {
                d.toolsBtn.SetIcon("tools")
        } else {
                d.toolsBtn.SetIcon("tools")
        }
        // The count itself is communicated via the status line on change;
        // the chip stays icon-only (minimal chrome).
        if enabled < total {
                logging.Default().Info("gui", "tools enabled: %d/%d", enabled, total)
        }
}

// normalizeToolList keeps only registered tool names (case-insensitive).
func normalizeToolList(orch *agent.Orchestrator, list []string) []string {
        if len(list) == 0 {
                return []string{}
        }
        seen := map[string]bool{}
        var out []string
        for _, name := range list {
                if _, ok := orch.Tools()[name]; ok && !seen[name] {
                        seen[name] = true
                        out = append(out, name)
                }
        }
        return out
}

// runPipeline launches the multi-agent planner→executor→critic→summarizer
// pipeline on a task entered in a dialog.
func (d *desktopApp) runPipeline() {
        dialog.ShowEntryDialog("Multi-agent pipeline",
                "Task for the planner → executor → critic → summarizer pipeline:",
                func(task string) {
                        task = strings.TrimSpace(task)
                        if task == "" {
                                return
                        }
                        if d.currentView != "chat" {
                                d.showView("chat")
                        }
                        d.clearActivities()
                        d.appendMessage("user", "[pipeline] "+task)
                        d.setRunning(true, "Pipeline: planning…")

                        ctx, cancel := context.WithCancel(context.Background())
                        d.mu.Lock()
                        d.abortCancel = cancel
                        d.mu.Unlock()

                        go func() {
                                defer func() {
                                        d.mu.Lock()
                                        d.abortCancel = nil
                                        d.mu.Unlock()
                                        d.setRunning(false, "")
                                }()
                                // v1.0.3: pipelines auto-start the engine too.
                                if !d.cfg.IsRemote() && d.llama != nil && !d.llama.IsRunning() {
                                        runOnMain(func() { d.setRunning(true, "Starting engine…") })
                                        if err := d.llama.EnsureRunning(); err != nil {
                                                runOnMain(func() {
                                                        d.appendActivity(agent.Activity{Type: "error", Caption: "Engine: " + err.Error(), Timestamp: time.Now()})
                                                        dialog.ShowError(err, d.win)
                                                })
                                                return
                                        }
                                        runOnMain(func() { d.setRunning(true, "Pipeline: planning…") })
                                }
                                result, err := d.multi.Run(ctx, task, func(a agent.Activity) {
                                        d.appendActivity(a)
                                })
                                out := result
                                if err != nil {
                                        out = "Pipeline error: " + err.Error()
                                }
                                runOnMain(func() {
                                        if d.active != nil {
                                                d.active.Messages = append(d.active.Messages, llm.Message{Role: "assistant", Content: out})
                                                _ = d.store.Save(d.active)
                                        }
                                        d.appendMessage("assistant", out)
                                })
                        }()
                }, d.win)
}

// --- model picker (v1.0.0 FIX) ---

// pickModel opens the model picker. v0.9 bug: tapping a model did nothing
// visible — the dialog stayed open, the config was written but the running
// engine kept serving the OLD model (llama.cpp loads the model at boot) and
// the bare filename was passed to --model where the subprocess could not
// resolve it. Now: the dialog closes, the engine reloads, the chip updates,
// and the status line confirms the switch.
func (d *desktopApp) pickModel() {
        if d.cfg.IsRemote() {
                d.pickRemoteModel()
                return
        }
        models := llm.ListLocalModels(d.cfg.ModelsDir)
        if len(models) == 0 {
                d.showModelGuidance()
                return
        }
        list := d.buildModelList(models)
        d.activeModelDialog = d.bigDialog("Choose model", scrollDialogContent(list, fyne.NewSize(600, 420)), 660, 520)
}

// buildModelList renders the picker rows for the given model files.
// v1.0.4: each row shows the REAL model card — params, quantization,
// context length, size — parsed from the GGUF header (LM Studio-style).
func (d *desktopApp) buildModelList(models []string) *fyne.Container {
        list := container.NewVBox()
        for _, m := range models {
                name := m
                meta := humanSize(d.fileSize(filepath.Join(d.cfg.ModelsDir, name)))
                if card, err := llm.ReadModelCard(filepath.Join(d.cfg.ModelsDir, name)); err == nil && card != nil {
                        meta = card.Meta()
                }
                row := newModelRow(name, meta,
                        strings.EqualFold(name, d.cfg.Model),
                        func() {
                                if dlg := d.activeModelDialog; dlg != nil {
                                        dlg.Hide() // close on pick — feedback first, always
                                }
                                d.applyModel(name)
                        })
                list.Add(row)
        }
        return list
}

func (d *desktopApp) pickRemoteModel() {
        go func() {
                models, err := llm.ListRemoteModels(d.cfg)
                if err != nil {
                        runOnMain(func() {
                                dialog.ShowInformation("Remote models",
                                        "Could not list remote models:\n"+err.Error()+
                                                "\n\nSet the model name manually via Agent → LLM provider…", d.win)
                        })
                        return
                }
                runOnMain(func() {
                        list := container.NewVBox()
                        var dlg *dialog.CustomDialog
                        for _, m := range models {
                                name := m
                                row := newModelRow(name, "remote", strings.EqualFold(name, d.cfg.RemoteModel), func() {
                                        if dlg != nil {
                                                dlg.Hide()
                                        }
                                        d.cfg.RemoteModel = name
                                        _ = config.Save(d.cfg.ConfigPath(), d.cfg)
                                        d.modelChip.SetText(clipStrMemory(name, 24))
                                        d.setStatus("Model set to " + name + " (remote)")
                                })
                                list.Add(row)
                        }
                        dlg = d.bigDialog("Pick remote model", scrollDialogContent(list, fyne.NewSize(600, 420)), 660, 520)
                })
        }()
}

// applyModel switches the local engine to `name`: persists the choice,
// updates the chip, and (v1.0.3) PRE-WARMS the engine immediately — the
// server boots with the model right away so the first message answers
// instantly. This is the "engine runs the moment a model is selected"
// behavior.
func (d *desktopApp) applyModel(name string) {
        d.cfg.Model = name
        _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        if d.modelChip != nil {
                d.modelChip.SetText(clipStrMemory(d.cfg.DisplayModel(), 24))
        }
        logging.Default().Info("gui", "model switched to %s", name)
        if d.llama == nil {
                return
        }
        // v1.0.3: prewarm — start (or reload) the engine with the new GGUF now.
        d.setStatus("Loading " + clipStrMemory(d.cfg.DisplayModel(), 28) + "…")
        go func() {
                err := d.llama.LoadOrStartWithModel(name)
                runOnMain(func() {
                        if err != nil {
                                d.setStatus("Engine failed — see Logs (Pro mode)")
                                // v1.0.5: launch failures now carry the engine's
                                // own stderr (the REAL reason — unknown
                                // architecture, template parse error, OOM), so
                                // surface them in a properly sized, scrollable
                                // dialog instead of a tiny error box.
                                msg := widget.NewLabel(err.Error())
                                msg.Wrapping = fyne.TextWrapWord
                                d.bigDialog("Could not load "+clipStrMemory(name, 32),
                                        scrollDialogContent(container.NewPadded(msg), fyne.NewSize(600, 380)), 660, 460)
                                return
                        }
                        d.setStatus("Model ready: " + clipStrMemory(d.cfg.DisplayModel(), 28))
                })
        }()
}

// showModelGuidance explains what to do when no .gguf exists yet.
func (d *desktopApp) showModelGuidance() {
        body := widget.NewLabel(
                "No local models yet.\n\n" +
                        "Drop a .gguf model file (Qwen, Gemma, Llama…) into the models folder,\n" +
                        "or connect a remote OpenAI-compatible endpoint instead.")
        body.Wrapping = fyne.TextWrapWord
        openBtn := primaryButton("Open models folder", "folder", func() { d.openInExplorer(d.cfg.ModelsDir) })
        provBtn := ghostButton("Connect remote provider", "provider", func() { d.showProviderDialog() })
        d.bigDialog("Choose model", container.NewVBox(body, container.NewHBox(openBtn, provBtn)), 620, 400)
}

// setStatus writes a transient status message to the footer.
func (d *desktopApp) setStatus(msg string) {
        if d.updateLbl != nil {
                d.updateLbl.SetText(clipStrMemory(msg, 70))
                d.updateLbl.Refresh()
        }
}

func (d *desktopApp) fileSize(path string) int64 {
        if fi, err := os.Stat(path); err == nil {
                return fi.Size()
        }
        return 0
}

func humanSize(n int64) string {
        switch {
        case n >= 1<<30:
                return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
        case n >= 1<<20:
                return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
        case n >= 1<<10:
                return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
        default:
                return fmt.Sprintf("%d B", n)
        }
}

// --- other dialogs ---

// bigDialog opens a CustomDialog at a GUARANTEED usable size (v1.0.5).
//
// The bug this kills: a dialog whose content is a Scroll container sizes
// itself to the scroll's MinSize — which is a ~30px sliver, because a
// scroller can in principle scroll anything. Settings, the model picker,
// system info and friends all collapsed into that sliver: "so small and
// not functional". Every custom dialog now goes through here, gets an
// explicit generous Resize, and is clamped to 92% of the window so it can
// never outgrow the screen the app is running on.
func (d *desktopApp) bigDialog(title string, content fyne.CanvasObject, w, h float32) *dialog.CustomDialog {
        if d.win != nil {
                if cw := d.win.Canvas().Size(); cw.Width > 100 && cw.Height > 100 {
                        if maxW := cw.Width * 0.92; w > maxW {
                                w = maxW
                        }
                        if maxH := cw.Height * 0.92; h > maxH {
                                h = maxH
                        }
                }
        }
        dlg := dialog.NewCustom(title, "Close", content, d.win)
        dlg.Resize(fyne.NewSize(w, h))
        dlg.Show()
        return dlg
}

// scrollDialogContent wraps dialog content in a vertical scroller that
// carries a sane MINIMUM size — the belt to bigDialog's suspenders. Even
// if some future Fyne release clamps the Resize away, the scroll itself
// still reports a usable MinSize and the dialog cannot collapse again.
func scrollDialogContent(obj fyne.CanvasObject, min fyne.Size) fyne.CanvasObject {
        sc := container.NewVScroll(obj)
        sc.SetMinSize(min)
        return sc
}

func (d *desktopApp) pickPreset() {
        v := container.NewVBox()
        var dlg *dialog.CustomDialog
        for _, p := range llm.Presets() {
                preset := p
                btn := widget.NewButton(preset.Label+" ("+preset.Name+")", func() {
                        llm.ApplyPreset(&d.cfg.LLM, preset.Name)
                        _ = config.Save(d.cfg.ConfigPath(), d.cfg)
                        if dlg != nil {
                                dlg.Hide() // close on apply
                        }
                        dialog.ShowInformation("Preset applied", preset.Label+" — "+preset.Description, d.win)
                })
                v.Add(btn)
                v.Add(widget.NewLabel(preset.Description))
        }
        dlg = dialog.NewCustom("Sampling preset", "Close", scrollDialogContent(v, fyne.NewSize(560, 460)), d.win)
        dlg.Resize(fyne.NewSize(640, 540))
        dlg.Show()
}

func (d *desktopApp) showSysinfo() {
        d.bigDialog("System info", scrollDialogContent(d.buildSystemTab(), fyne.NewSize(560, 480)), 640, 560)
}

func (d *desktopApp) showComponents() {
        dialog.ShowInformation("Components", fmt.Sprintf(
                "App folder:  %s\nModels dir:  %s\nSessions:    %s\nMemory:      %d entries\nSandbox:     %v\nCharts:      %d",
                d.cfg.DataDir, d.cfg.ModelsDir, d.cfg.SessionsDir, d.mem.Count(), d.sb != nil, len(d.chartFiles)), d.win)
}

// showSettings is the v1.0.0 settings hub: experience (pro mode), engine
// updates schedule (daily / weekly / monthly / off), provider, and about
// links — one calm dialog instead of scattered menu hops.
func (d *desktopApp) showSettings() {
        proCheck := widget.NewCheck("Pro mode — dock, live logs, memory & activity detail", func(on bool) {
                d.proSwitch.SetChecked(on)
        })
        proCheck.SetChecked(d.cfg.ProMode)

        schedule := widget.NewRadioGroup([]string{"daily", "weekly", "monthly", "off"}, func(s string) {
                d.cfg.UpdateSchedule = updater.NormalizeSchedule(s)
                _ = config.Save(d.cfg.ConfigPath(), d.cfg)
                logging.Default().Info("updater", "schedule set to %s", d.cfg.UpdateSchedule)
        })
        schedule.Horizontal = true
        schedule.Selected = updater.NormalizeSchedule(d.cfg.UpdateSchedule)

        last, _ := time.Parse(time.RFC3339, d.cfg.LastUpdateCheck)
        lastLbl := widget.NewLabel("Last check: " + func() string {
                if last.IsZero() {
                        return "never"
                }
                return humanize(last)
        }())
        lastLbl.Importance = widget.LowImportance
        checkNow := ghostButton("Check for engine update now", "update", d.checkUpdatesNow)

        // v1.0.7: Continuum — the "almost unlimited context" engine.
        continuumCheck := widget.NewCheck("Continuum context — chapters roll over with memory carried forward", func(on bool) {
                d.cfg.ContinuumEnabled = on
                _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        })
        continuumCheck.SetChecked(d.cfg.ContinuumEnabled)

        thresholdLbl := widget.NewLabel(fmt.Sprintf("Rollover threshold: %d%% of the history budget", d.cfg.EffectiveContinuumThreshold()))
        thresholdLbl.Importance = widget.LowImportance
        threshold := widget.NewSlider(50, 95)
        threshold.Step = 5
        threshold.SetValue(float64(d.cfg.EffectiveContinuumThreshold()))
        threshold.OnChanged = func(v float64) {
                d.cfg.ContinuumThresholdPct = int(v)
                thresholdLbl.SetText(fmt.Sprintf("Rollover threshold: %d%% of the history budget", int(v)))
                _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        }

        carryLbl := widget.NewLabel(fmt.Sprintf("Messages carried into a new chapter: %d", d.cfg.EffectiveContinuumCarry()))
        carryLbl.Importance = widget.LowImportance
        carry := widget.NewSlider(0, 16)
        carry.Step = 1
        carry.SetValue(float64(d.cfg.EffectiveContinuumCarry()))
        carry.OnChanged = func(v float64) {
                d.cfg.ContinuumCarryMessages = int(v)
                carryLbl.SetText(fmt.Sprintf("Messages carried into a new chapter: %d", int(v)))
                _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        }

        ctxBtn := ghostButton("Context & memory…", "layers", d.showContextDialog)

        // v1.0.9 (TURBINE): streaming smoothness — the frame-paced pump.
        smoothCheck := widget.NewCheck("Smooth streaming — render at the display's frame rate", func(on bool) {
                d.cfg.SmoothStream = on
                _ = config.Save(d.cfg.ConfigPath(), d.cfg)
        })
        smoothCheck.SetChecked(d.cfg.SmoothStream)
        fps := widget.NewRadioGroup([]string{"60", "90", "120", "144", "240"}, func(s string) {
                if n, err := fmt.Sscanf(s, "%d", &d.cfg.TargetFPS); err == nil && n == 1 {
                        _ = config.Save(d.cfg.ConfigPath(), d.cfg)
                }
        })
        fps.Horizontal = true
        fps.Selected = fmt.Sprintf("%d", d.cfg.EffectiveTargetFPS())
        fpsNote := widget.NewLabel("Target frame rate for stream coalescing (120 fps = one update every 8.3ms)")
        fpsNote.Importance = widget.LowImportance

        providerBtn := ghostButton("LLM provider…", "provider", d.showProviderDialog)
        engineBtn := ghostButton("System info…", "system", d.showSysinfo)
        aboutBtn := ghostButton("About "+brand.Trademark, "info", d.showAbout)
        licenseBtn := ghostButton("License", "license", d.showLicense)

        form := widget.NewForm(
                widget.NewFormItem("Experience", proCheck),
                widget.NewFormItem("Streaming", container.NewVBox(smoothCheck, fps, fpsNote)),
                widget.NewFormItem("Engine updates", container.NewVBox(schedule, lastLbl, checkNow)),
                widget.NewFormItem("Continuum", container.NewVBox(
                        continuumCheck,
                        container.NewVBox(thresholdLbl, threshold),
                        container.NewVBox(carryLbl, carry),
                        ctxBtn,
                )),
                widget.NewFormItem("Backend", container.NewHBox(providerBtn)),
                widget.NewFormItem("About", container.NewHBox(aboutBtn, licenseBtn, engineBtn)),
        )
        d.bigDialog("Settings", scrollDialogContent(container.NewPadded(form), fyne.NewSize(620, 520)), 700, 620)
}

func (d *desktopApp) showAbout() {
        logo := canvas.NewImageFromResource(Logo)
        logo.FillMode = canvas.ImageFillContain
        logo.SetMinSize(fyne.NewSize(64, 64))
        title := widget.NewLabelWithStyle(brand.Trademark+" Local-Agent v"+config.AppVersion,
                fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
        subtitle := widget.NewLabelWithStyle("Ember Minimal · Native Windows desktop agent",
                fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

        body := widget.NewLabel(fmt.Sprintf(
                "%s\n%s\n%s/%s\n\n"+
                        "Agent: planner → executor → critic → summarizer\n"+
                        "Tools: shell · files · codeExec (Job-Object sandbox) · webSearch · browser automation · git · dataAnalysis · memory\n"+
                        "Engine: llama.cpp (auto-updated on schedule) or any OpenAI-compatible endpoint\n"+
                        "Log catcher: app.log + tools.jsonl + llm.jsonl + crash reports\n\n"+
                        "%s\n%s\n%s",
                brand.FullName, brand.Copyright(), runtime.GOOS, runtime.GOARCH,
                brand.LicenseFooter, brand.TrademarkNotice, brand.LicensorURL))
        body.Wrapping = fyne.TextWrapWord

        // v1.0.8: the authorship signature — the app is signed under Parsa Tak.
        sig := canvas.NewText(brand.SignatureLine(), ColEmber)
        sig.TextSize = 12
        sig.TextStyle.Bold = true
        sig.Alignment = fyne.TextAlignCenter

        d.bigDialog("About", container.NewVBox(
                container.NewCenter(logo),
                title, subtitle,
                widget.NewSeparator(),
                container.NewVScroll(body),
                widget.NewSeparator(),
                container.NewPadded(sig),
        ), 600, 660)
}

func (d *desktopApp) showLicense() {
        text := widget.NewRichTextFromMarkdown("```\n" + brand.LicenseText + "\n```")
        text.Wrapping = fyne.TextWrapWord
        d.bigDialog(brand.LicenseName, scrollDialogContent(text, fyne.NewSize(600, 460)), 680, 560)
}

// showProviderDialog configures the LLM backend.
func (d *desktopApp) showProviderDialog() {
        provider := widget.NewRadioGroup([]string{"local llama.cpp", "remote endpoint"}, nil)
        if d.cfg.IsRemote() {
                provider.Selected = "remote endpoint"
        } else {
                provider.Selected = "local llama.cpp"
        }
        provider.Horizontal = true

        baseURL := widget.NewEntry()
        baseURL.SetPlaceHolder("https://api.example.com/v1")
        baseURL.Text = d.cfg.RemoteBaseURL
        apiKey := widget.NewPasswordEntry()
        apiKey.SetPlaceHolder("sk-… (stored locally, redacted in diagnostics)")
        apiKey.Text = d.cfg.RemoteAPIKey
        model := widget.NewEntry()
        model.SetPlaceHolder("glm-4.6 / gpt-4o-mini / …")
        model.Text = d.cfg.RemoteModel

        form := widget.NewForm(
                widget.NewFormItem("Provider", provider),
                widget.NewFormItem("Base URL", baseURL),
                widget.NewFormItem("API key", apiKey),
                widget.NewFormItem("Model", model),
        )
        var dlg *dialog.CustomDialog
        saveBtn := primaryButton("Save provider", "provider", func() {
                if provider.Selected == "remote endpoint" {
                        d.cfg.Provider = config.ProviderRemote
                        d.cfg.RemoteBaseURL = strings.TrimSpace(baseURL.Text)
                        d.cfg.RemoteAPIKey = strings.TrimSpace(apiKey.Text)
                        d.cfg.RemoteModel = strings.TrimSpace(model.Text)
                        if d.cfg.RemoteBaseURL == "" || d.cfg.RemoteModel == "" {
                                dialog.ShowInformation("Missing fields", "Remote provider needs a Base URL and a Model.", d.win)
                                return
                        }
                } else {
                        d.cfg.Provider = config.ProviderLocal
                }
                _ = config.Save(d.cfg.ConfigPath(), d.cfg)
                logging.Default().Info("gui", "provider switched to %s (model=%s)", d.cfg.ProviderKind(), d.cfg.EffectiveModel())
                if dlg != nil {
                        dlg.Hide() // v1.0.0: close the dialog — saving must feel like it worked
                }
                if d.modelChip != nil {
                        d.modelChip.SetText(clipStrMemory(d.cfg.EffectiveModel(), 24))
                }
                d.setEngineState("refresh")
                d.setStatus("Provider: " + d.cfg.ProviderKind() + " · " + d.cfg.EffectiveModel())
        })
        testBtn := ghostButton("Test connection", "provider", func() {
                if provider.Selected == "remote endpoint" {
                        d.cfg.Provider = config.ProviderRemote
                        d.cfg.RemoteBaseURL = strings.TrimSpace(baseURL.Text)
                        d.cfg.RemoteAPIKey = strings.TrimSpace(apiKey.Text)
                        d.cfg.RemoteModel = strings.TrimSpace(model.Text)
                }
                go func() {
                        models, err := llm.ListRemoteModels(d.cfg)
                        if err != nil {
                                runOnMain(func() {
                                        dialog.ShowError(fmt.Errorf("connection failed: %v", err), d.win)
                                })
                                return
                        }
                        runOnMain(func() {
                                dialog.ShowInformation("Connection OK",
                                        fmt.Sprintf("Endpoint reachable.\nModels: %s", strings.Join(models, ", ")), d.win)
                        })
                }()
        })

        dlg = d.bigDialog("LLM provider", container.NewVBox(form, container.NewHBox(saveBtn, testBtn)), 640, 480)
}

// exportDiagnostics writes the diagnostics zip.
func (d *desktopApp) exportDiagnostics() {
        go func() {
                m := logging.Default()
                out := d.cfg.DataDir + "/diagnostics/sheytan-diagnostics.zip"
                path, err := m.Diagnostics(out, d.cfg.ConfigPath(), nil)
                if err != nil {
                        runOnMain(func() { dialog.ShowError(err, d.win) })
                        return
                }
                logging.Default().Info("gui", "diagnostics exported: %s", path)
                runOnMain(func() {
                        dialog.ShowInformation("Diagnostics exported",
                                "Bundle written to:\n"+path+"\n\nContains app logs, tool/LLM stats, crash reports, sysinfo, and a redacted config.", d.win)
                })
        }()
}

func (d *desktopApp) refreshLogs() {
        d.logLines = logging.Default().Recent(300)
        if len(d.logLines) == 0 {
                d.logLines = []string{"(no log lines yet — run a turn)"}
        }
        if d.logsList != nil {
                d.logsList.Refresh()
        }
}

func formatToolStats(st logging.Stats) string {
        var b strings.Builder
        for tool, n := range st.CallsPerTool {
                errs := st.ErrorsPerTool[tool]
                avg := st.AvgDurationMs[tool]
                fmt.Fprintf(&b, "  %-14s %4d calls  %3d errors  avg %.0f ms\n", tool, n, errs, avg)
        }
        return b.String()
}

func (d *desktopApp) startLlama() {
        d.setStatus("Starting llama.cpp engine…")
        go func() {
                if err := d.llama.Start(); err != nil {
                        runOnMain(func() {
                                d.setStatus("Engine failed to start")
                                dialog.ShowError(err, d.win)
                        })
                        return
                }
                runOnMain(func() {
                        d.setStatus("Engine ready on " + d.cfg.LlamaHost + ":" + fmt.Sprint(d.cfg.LlamaPort))
                })
        }()
}

func (d *desktopApp) stopLlama() {
        _ = d.llama.Stop()
        d.setStatus("Engine stopped")
}

func (d *desktopApp) runStress() {
        d.bigDialog("Stress test", scrollDialogContent(widget.NewLabel("The full hostile-scenario suite runs via the CLI:\n\n  sheytan-local-agent stress\n\n58+ scenarios across the agent, tools, sessions, memory, logging, data analysis, offline behavior, and config."), fyne.NewSize(560, 260)), 660, 420)
}

// openInExplorer reveals a file or folder in Windows Explorer (the parent
// folder opens with the item selected when `path` is a file).
// v1.0.4: routed through internal/proc so no console window ever flashes.
func (d *desktopApp) openInExplorer(path string) {
        go func() {
                if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
                        // file → open the parent with the file pre-selected
                        switch runtime.GOOS {
                        case "windows":
                                _ = proc.Command("explorer", "/select,"+path).Start()
                        default:
                                _ = proc.Command("xdg-open", filepath.Dir(path)).Start()
                        }
                        return
                }
                switch runtime.GOOS {
                case "windows":
                        _ = proc.Command("explorer", path).Start()
                case "darwin":
                        _ = proc.Command("open", path).Start()
                default:
                        _ = proc.Command("xdg-open", path).Start()
                }
        }()
}

// openWithDefaultApp opens a file with the OS default application
// (rundll32 on Windows — GUI-only, no console window).
func (d *desktopApp) openWithDefaultApp(path string) {
        go func() {
                switch runtime.GOOS {
                case "windows":
                        _ = proc.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
                case "darwin":
                        _ = proc.Command("open", path).Start()
                default:
                        _ = proc.Command("xdg-open", path).Start()
                }
        }()
}

// copyPathToClipboard puts `text` on the system clipboard and confirms it
// in the status line.
func (d *desktopApp) copyPathToClipboard(text string) {
        if d.win != nil {
                if cb := d.win.Clipboard(); cb != nil {
                        cb.SetContent(text)
                }
        }
        d.setStatus("Copied: " + clipStrMemory(text, 70))
}

// --- helpers ---

func humanize(t time.Time) string {
        if t.IsZero() {
                return "—"
        }
        d := time.Since(t)
        if d < time.Minute {
                return "just now"
        }
        if d < time.Hour {
                return fmt.Sprintf("%dm ago", int(d.Minutes()))
        }
        if d < 24*time.Hour {
                return fmt.Sprintf("%dh ago", int(d.Hours()))
        }
        return t.Format("Jan 02 15:04")
}

var _ = sysinfo.Probe // sysinfo used in views.go
