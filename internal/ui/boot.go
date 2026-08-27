//go:build !headless

package ui

import (
        "fmt"
        "image/color"
        "os"
        "runtime"
        "runtime/debug"

        "fyne.io/fyne/v2"
        "fyne.io/fyne/v2/app"
        "fyne.io/fyne/v2/canvas"
        "fyne.io/fyne/v2/container"
        "fyne.io/fyne/v2/driver/desktop"

        "github.com/sheytan/local-agent/internal/brand"
        "github.com/sheytan/local-agent/internal/config"
        "github.com/sheytan/local-agent/internal/llm"
        "github.com/sheytan/local-agent/internal/logging"
        agentrt "github.com/sheytan/local-agent/internal/runtime"
        "github.com/sheytan/local-agent/internal/sessions"
        "github.com/sheytan/local-agent/internal/tools"
)

// RunDesktop boots the native desktop GUI.
func RunDesktop() {
        // v1.0.8 low-level runtime tuning — the "faster on Windows" pass.
        // A desktop chat UI allocates heavily while streaming (RichText
        // re-segments every token delta) and the default GOGC=100 triggers
        // collections twice as often as this app needs. Raising the target
        // trades a slightly larger heap for noticeably fewer GC pauses —
        // fewer dropped frames while tokens stream. The soft memory limit
        // keeps the heap bounded so the trade can never balloon.
        debug.SetGCPercent(160)
        debug.SetMemoryLimit(768 << 20) // soft cap: 768 MB

        a := app.NewWithID("com.sheytan.local-agent")
        a.Settings().SetTheme(Theme())
        a.SetIcon(Logo)

        // Portable mode: everything lives in the app folder next to the exe.
        cfg, err := config.Load(config.DefaultPath())
        if err != nil {
                cfg = config.Default()
        }
        if err := cfg.EnsureDirs(); err != nil {
                fmt.Println("mkdir:", err)
                return
        }

        // Boot the log catcher first so the GUI itself is logged.
        mgr, logErr := logging.New(cfg.LogsDir())
        if logErr == nil {
                logging.SetDefault(mgr)
                logging.SetVersion(config.AppVersion)
        }
        logging.Default().Info("gui", "%s v%s booting (provider=%s, root=%s)",
                brand.FullName, config.AppVersion, cfg.ProviderKind(), cfg.DataDir)

        // Shared agent stack (identical tool set to the CLI `ask` command).
        stack := agentrt.NewStack(cfg)

        // v1.0.3: model sanity at boot — the config's model (or the old
        // phantom "gemma-4" default) may not exist in the models folder.
        // Snap to the first REAL .gguf so the chip never shows a model the
        // engine could not load; when the folder is empty the chip invites
        // the user to choose (and the hero shows the get-started card).
        if !cfg.IsRemote() {
                if models := llm.ListLocalModels(cfg.ModelsDir); len(models) > 0 {
                        if _, err := llm.ResolveModelPath(cfg.ModelsDir, cfg.Model); err != nil {
                                cfg.Model = models[0]
                                _ = config.Save(cfg.ConfigPath(), cfg)
                                logging.Default().Info("gui", "model auto-selected at boot: %s", cfg.Model)
                        }
                } else if cfg.Model != "" {
                        cfg.Model = ""
                        _ = config.Save(cfg.ConfigPath(), cfg)
                }
        }

        d := &desktopApp{
                cfg:    cfg,
                fyne:   a,
                store:  sessions.New(cfg.SessionsDir),
                client: stack.Client,
                orch:   stack.Orch,
                multi:  stack.Multi,
                mem:    stack.Mem,
                llama:  stack.Llama,
                sb:     stack.Sandbox,
                stack:  stack,
                recall: stack.Recall, // v1.0.2 persistent memory over past chats
        }

        // v1.0.4: artifact tracker — every file the agent creates lands in
        // the Files view and as chips under the chat reply.
        d.tracker = newArtifactTracker(cfg)
        // Tool hook: files the tools write anywhere under the app folder are
        // reported explicitly (the turn snapshot-diff covers the watched dirs).
        tools.OnFileCreated = func(path string) { d.tracker.Report(path) }
        d.navRows = map[string]*navRow{}
        d.views = map[string]fyne.CanvasObject{}
        d.fader = newCrossFader()
        d.win = a.NewWindow(brand.Trademark + " Local-Agent v" + config.AppVersion)
        d.win.SetMainMenu(d.buildMenu())
        // v0.9.1: the root content carries a 1024x660 minimum (via an
        // invisible floor object) so the window never re-flows the layout
        // below its design floor — Fyne has no Window.SetMinSize API.
        // (v1.0.5: raised from 980x620 — the pro dock now carries a 380px
        // minimum, and the chat column needs its measure at that width.)
        floor := canvas.NewRectangle(color.Transparent)
        floor.SetMinSize(fyne.NewSize(1024, 660))
        root := container.NewStack(d.buildRoot(), floor, splashLayer(nil))
        d.win.SetContent(root)
        d.win.Resize(fyne.NewSize(1340, 840))
        // v1.0.6: Ctrl+K — the command palette (search-everything surface).
        d.win.Canvas().AddShortcut(
                &desktop.CustomShortcut{KeyName: fyne.KeyK, Modifier: desktop.ControlModifier},
                func(fyne.Shortcut) { d.showPalette() })
        d.win.SetOnClosed(func() {
                logging.Default().Info("gui", "window closed — shutting down")
                stack.Close()
        })

        // v1.0.5: fit the window to the actual screen once the event loop is
        // live and the real canvas scale is known. On a 150%-scaled laptop
        // display (now that the exe is DPI-aware) the 1340x840 logical
        // default would be 2010x1260 device pixels — larger than a 1080p
        // panel. The fitter clamps the window to 94% of the logical screen
        // so it always opens fully visible.
        a.Lifecycle().SetOnStarted(func() {
                runOnMain(func() { d.fitWindowToScreen() })
        })

        // Initial data load
        d.reloadSessions()
        if len(d.sessions) == 0 {
                s := d.store.Create()
                d.sessions = append(d.sessions, s)
        }
        // v1.0.2: sessions list entries are meta-index stubs — load the full
        // history for the session we open at boot.
        if full, err := d.store.Get(d.sessions[0].ID); err == nil {
                d.active = full
        } else {
                d.active = d.sessions[0]
        }
        d.renderActive()
        d.refreshCharts()
        d.refreshFiles()

        // Connectivity watcher: ONLINE/OFFLINE pill + agent environment note.
        d.startNetWatcher()
        // Engine watcher: model-chip dot + footer engine state.
        d.startEngineWatcher()
        // Scheduled engine updates (daily / weekly / monthly) in the background.
        d.startUpdateLoop()

        d.win.CenterOnScreen()

        // Crash catcher around the GUI event loop.
        defer func() {
                if r := recover(); r != nil {
                        buf := make([]byte, 16384)
                        n := runtime.Stack(buf, false)
                        path := logging.Default().Crash(r, buf[:n])
                        fmt.Fprintf(os.Stderr, "panic: %v (crash report: %s)\n", r, path)
                }
        }()
        d.win.ShowAndRun()
}
