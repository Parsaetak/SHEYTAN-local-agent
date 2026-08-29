//go:build headless

// Screenshot harness: renders the full SHEYTAN GUI offscreen (software
// painter, no display needed) and saves PNGs of every view so the design
// can be verified without running Windows.
//
// Run:  go test -tags headless ./internal/ui -run TestScreenshots -v
// Out:  /home/z/my-project/sheytan-go/shots/*.png
package ui

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/sheytan/local-agent/internal/agent"
	"github.com/sheytan/local-agent/internal/artifacts"
	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/continuum"
	"github.com/sheytan/local-agent/internal/llm"
	"github.com/sheytan/local-agent/internal/logging"
	agentrt "github.com/sheytan/local-agent/internal/runtime"
	"github.com/sheytan/local-agent/internal/sessions"
	"github.com/sheytan/local-agent/internal/tools"
)

func capture(t *testing.T, c software.WindowlessCanvas, name string) {
	img := c.Capture()
	outDir := "shots"
	_ = os.MkdirAll(outDir, 0o755)
	f, err := os.Create(filepath.Join(outDir, name+".png"))
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode %s: %v", name, err)
	}
	t.Logf("captured %s.png", name)
}

func nowAt(minutesAgo int) time.Time { return time.Now().Add(-time.Duration(minutesAgo) * time.Minute) }

func newScreenshotApp(t *testing.T) *desktopApp {
	a := test.NewApp()
	a.Settings().SetTheme(Theme())

	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.ModelsDir = filepath.Join(dir, "models")
	cfg.SessionsDir = filepath.Join(dir, "sessions")
	_ = cfg.EnsureDirs()

	mgr, err := logging.New(cfg.LogsDir())
if err == nil {
    logging.SetDefault(mgr)
    logging.SetVersion(config.AppVersion)

    // Windows keeps exclusive file handles open until explicitly closed.
    // Register cleanup before returning so TempDir cleanup happens only
    // after app.log/tools.jsonl/llm.jsonl are released.
    t.Cleanup(func() {
        _ = mgr.Close()
        logging.SetDefault(nil)
    })
}

	stack := agentrt.NewStack(cfg)

	d := &desktopApp{
		cfg:     cfg,
		fyne:    a,
		store:   sessions.New(cfg.SessionsDir),
		client:  stack.Client,
		orch:    stack.Orch,
		multi:   stack.Multi,
		mem:     stack.Mem,
		llama:   stack.Llama,
		sb:      stack.Sandbox,
		stack:   stack,
		recall:  stack.Recall,
		tracker: newArtifactTracker(cfg), // v1.0.4 Files view
	}
	d.navRows = map[string]*navRow{}
	d.views = map[string]fyne.CanvasObject{}
	d.fader = newCrossFader()
	return d
}

func TestScreenshots(t *testing.T) {
	d := newScreenshotApp(t)

	// Seed a realistic session.
	s := d.store.Create()
	s.Title = "Analyze sales data & chart revenue"
	s.Messages = []llm.Message{
		{Role: "user", Content: "Create sales.csv with sample data, analyze it, and chart revenue by region."},
		{Role: "assistant", Content: "**Done.** Here is the revenue breakdown by region:\n\n" +
			"| Region | Revenue |\n|---|---|\n| Tehran | 25,800 |\n| Isfahan | 15,720 |\n| Tabriz | 8,910 |\n\n" +
			"The bar chart is saved in `charts/rev-by-region.svg` — open the **Data** view to preview it.\n\n" +
			"Correlation between price and revenue is weak (r = 0.09); units drive revenue (r = 0.80)."},
	}
	_ = d.store.Save(s)
	s2 := d.store.Create()
	s2.Title = "Browser automation demo"
	_ = d.store.Save(s2)

	d.sessions, _ = d.store.List()
	// Pick the seeded session WITH messages. v1.0.2: List() returns
	// meta-index stubs — load the full history for the one we open.
	for _, s := range d.sessions {
		if s.MessageCount() > 0 {
			if full, err := d.store.Get(s.ID); err == nil {
				d.active = full
			}
			break
		}
	}

	// Seed activity stream (v0.9.1: rows are appended through
	// appendActivity so the widget path matches production).
	seedActivities := []agent.Activity{
		{Type: "tool_start", Caption: "Calling tool: files({\"action\":\"write\",\"path\":\"sales.csv\"...})", Timestamp: nowAt(0)},
		{Type: "tool_end", Caption: "Tool files done (84ms): wrote 482 bytes to sales.csv", Timestamp: nowAt(1)},
		{Type: "tool_start", Caption: "Calling tool: dataAnalysis({\"action\":\"profile\"...})", Timestamp: nowAt(2)},
		{Type: "tool_end", Caption: "Tool dataAnalysis done (12ms): 10 rows × 6 columns", Timestamp: nowAt(3)},
		{Type: "tool_start", Caption: "Calling tool: dataAnalysis({\"action\":\"chart\",\"chart\":\"bar\"...})", Timestamp: nowAt(4)},
		{Type: "tool_end", Caption: "Tool dataAnalysis done (5ms): Chart rendered → charts/rev-by-region.svg", Timestamp: nowAt(5)},
	}

	// Render a real chart into the charts dir via the data tool.
	dt := tools.NewDataTool(d.cfg)
	csv := `region,product,units,price,revenue
Tehran,Laptop,12,900,10800
Tehran,Phone,30,500,15000
Isfahan,Tablet,15,300,4500
Isfahan,Phone,22,510,11220
Shiraz,Laptop,4,920,3680
Tabriz,Phone,18,495,8910
Karaj,Laptop,10,880,8800
`
	_ = os.WriteFile(filepath.Join(d.cfg.DataDir, "sales.csv"), []byte(csv), 0o644)
	_, _ = dt.Run(context.Background(), json.RawMessage(`{"action":"chart","path":"sales.csv","chart":"bar","labelCol":"region","valueCol":"revenue","name":"rev-by-region"}`))
	_, _ = dt.Run(context.Background(), json.RawMessage(`{"action":"chart","path":"sales.csv","chart":"pie","labelCol":"product","valueCol":"revenue","name":"rev-by-product"}`))

	// Seed memory.
	_ = d.mem.Append([]string{"preference"}, "User prefers Persian region names in reports", "system")
	_ = d.mem.Append([]string{"data"}, "sales.csv is the primary dataset for revenue analysis", "system")

	// Build the UI.
	content := d.buildRoot()
	d.renderActive()
	d.setNetStatus(true)
	d.setEngineState("stopped")
	for _, a := range seedActivities {
		d.appendActivity(a)
	}
	d.refreshCharts()
	d.refreshLogs()

	c := software.NewCanvas()
	c.SetContent(content)
	c.Resize(fyne.NewSize(1340, 840))
	d.win = test.NewWindow(content)
	d.win.Resize(fyne.NewSize(1340, 840))
	// 1. Chat view (default).
	capture(t, c, "01-chat")

	// 2. Data view.
	d.showView("data")
	capture(t, c, "02-data")

	// 3. Memory view.
	d.showView("memory")
	capture(t, c, "03-memory")

	// 4. Logs view.
	d.showView("logs")
	capture(t, c, "04-logs")

	// 5. Running state (flame + dots + abort).
	d.showView("chat")
	d.setRunning(true, "Calling tool: dataAnalysis({\"action\":\"stats\"}...)")
	capture(t, c, "05-running")
	d.setRunning(false, "")

	// 6. Icons sheet — render every icon at 64px.
	iconCanvas := software.NewCanvas()
	iconCanvas.SetContent(iconSheet())
	iconCanvas.Resize(fyne.NewSize(1340, 700))
	img := iconCanvas.Capture()
	out, err := os.Create("shots/06-icons.png")
	if err == nil {
		_ = png.Encode(out, img)
		out.Close()
		t.Log("captured 06-icons.png")
	}

	// 7. Splash layer.
	splashCanvas := software.NewCanvas()
	splashCanvas.SetContent(containerStack(splashLayer(nil)))
	splashCanvas.Resize(fyne.NewSize(1340, 840))
	simg := splashCanvas.Capture()
	sout, err := os.Create("shots/07-splash.png")
	if err == nil {
		_ = png.Encode(sout, simg)
		sout.Close()
		t.Log("captured 07-splash.png")
	}

	// 8. v0.9.1 regression: minimum supported window size — nothing
	// may overlap when the window is at its floor. (Resize the SAME
	// canvas: sharing the content object across canvases would resize
	// the root and corrupt subsequent captures.)
	c.Resize(fyne.NewSize(980, 620))
	capture(t, c, "08-small")
	c.Resize(fyne.NewSize(1340, 840))

	// 9. v0.9.1 regression: a very long message + very long activity
	// caption + huge pasted input must stay inside its own bounds.
	long := "This is a deliberately enormous message body designed to stress the bubble layout. "
	for i := 0; i < 30; i++ {
		long += fmt.Sprintf("Paragraph %d continues with more words so the RichText has to wrap over many lines, which the old fixed-height list rows could not hold. ", i+1)
	}
	d.appendMessage("user", long)
	d.appendMessage("assistant", "**Understood.** The long message above is fully contained in its own bubble — the next messages never overlap it.")
	d.appendActivity(agent.Activity{Type: "tool_start", Caption: "Calling tool: dataAnalysis({\"action\":\"query\",\"select\":[\"region\",\"product\",\"units\",\"price\",\"revenue\"],\"where\":\"revenue>5000\",\"orderBy\":\"revenue desc\"}) — extremely long activity caption to prove truncation works", Timestamp: nowAt(6)})
	d.msgInput.SetText(strings.Repeat("A wall of pasted text that used to inflate the input panel until it covered the message list.\n", 30))
	capture(t, c, "09-longmsg")

	// 10. v1.0.0: Pro mode — right dock, memory/logs nav, activity detail.
	d.msgInput.SetText("")
	d.cfg.ProMode = true
	d.setProMode(true)
	d.setRunning(true, "Calling tool: dataAnalysis({\"action\":\"stats\"})")
	capture(t, c, "10-pro")
	d.setRunning(false, "")
	d.cfg.ProMode = false
	d.setProMode(false)

	// 11. v1.0.0: empty-state hero (fresh session, no messages) with a
	// seeded model so the get-started card is absent.
	_ = os.WriteFile(filepath.Join(d.cfg.ModelsDir, "qwen2.5-3b-instruct-q4_k_m.gguf"), []byte("gguf"), 0o644)
	s3 := d.store.Create()
	s3.Title = "Fresh"
	d.active = s3
	d.renderActive()
	capture(t, c, "11-hero")
	// and the first-run variant (no models at all)
	_ = os.Remove(filepath.Join(d.cfg.ModelsDir, "qwen2.5-3b-instruct-q4_k_m.gguf"))
	d.views["chat"] = d.buildChatView()
	d.centerStack.Objects = []fyne.CanvasObject{d.views["chat"]}
	d.centerStack.Refresh()
	d.active = s3
	d.renderActive()
	capture(t, c, "12-hero-firstrun")

	// 13. v1.0.0: the model picker rows (the fixed interaction) — current
	// model carries the check mark. Rendered as a centered panel over the
	// chat view (the dialog chrome itself is stock Fyne).
	_ = os.WriteFile(filepath.Join(d.cfg.ModelsDir, "gemma-3-4b-it-q4_k_m.gguf"), []byte("gguf"), 0o644)
	_ = os.WriteFile(filepath.Join(d.cfg.ModelsDir, "qwen2.5-3b-instruct-q4_k_m.gguf"), []byte("gguf"), 0o644)
	d.cfg.Model = "gemma-3-4b-it-q4_k_m.gguf"
	picker := panel(container.NewVBox(
		sectionHeader("model", "Choose model"),
		d.buildModelList(llm.ListLocalModels(d.cfg.ModelsDir)),
	), 12, 10)
	overlay := container.NewStack(content, container.NewCenter(picker))
	c.SetContent(overlay)
	c.Resize(fyne.NewSize(1340, 840))
	capture(t, c, "13-modelpicker")
	_ = overlay

	// 14. v1.0.2: composer with staged attachments + thinking mode on +
	// an assistant message carrying a reasoning trace and attachment
	// chips.
	c.SetContent(content)
	c.Resize(fyne.NewSize(1340, 840))
	d.cfg.ThinkingMode = true
	if d.thinkBtn != nil {
		d.thinkBtn.SetActive(true)
	}
	notesPath := filepath.Join(d.cfg.DataDir, "notes.txt")
	_ = os.WriteFile(notesPath, []byte("quarterly numbers attached"), 0o644)
	d.setPendingFiles([]string{notesPath})
	d.appendMessageFull("user", "Analyze the attached notes and summarize the quarter.", "", []string{"notes.txt"})
	d.appendMessageFull("assistant",
		"The quarter closed strong: revenue up **23%** YoY, EMEA leading growth.",
		"The user attached notes.txt with quarterly figures. I should compare revenue to last year, highlight the leading region, and keep it to three sentences.",
		nil)
	capture(t, c, "14-attachments-thinking")
	d.setPendingFiles(nil)
	d.cfg.ThinkingMode = false
	if d.thinkBtn != nil {
		d.thinkBtn.SetActive(false)
	}

	// 15. v1.0.4: Files (artifacts) view — seeded workspace outputs with
	// in-chat "created files" chips under the last reply.
	_ = os.WriteFile(filepath.Join(d.cfg.WorkspaceDir(), "sales-summary.md"), []byte("# Sales summary\n\nRevenue up 23% YoY."), 0o644)
	_ = os.WriteFile(filepath.Join(d.cfg.WorkspaceDir(), "cleaned-sales.csv"), []byte("region,revenue\nTehran,25800\n"), 0o644)
	d.refreshFiles()
	d.showView("files")
	capture(t, c, "15-files")
	d.showView("chat")
	d.appendArtifactChips([]artifacts.Artifact{
		{Path: filepath.Join(d.cfg.WorkspaceDir(), "sales-summary.md"), Name: "sales-summary.md", Size: 4096, ModTime: time.Now(), Kind: artifacts.KindDoc},
		{Path: filepath.Join(d.cfg.ChartsDir(), "rev-by-region.svg"), Name: "rev-by-region.svg", Size: 8192, ModTime: time.Now(), Kind: artifacts.KindChart},
	})
	capture(t, c, "16-artifact-chips")

	// 16. v1.0.4: Models view — real GGUF cards + storage line. Build a
	// minimal valid GGUF so the card parser has real metadata to show.
	writeTestGGUF(t, filepath.Join(d.cfg.ModelsDir, "qwen2.5-7b-instruct-q4_k_m.gguf"))
	d.refreshModels()
	d.showView("models")
	capture(t, c, "17-models")

	// 17. v1.0.6: Terminal view — the built-in Linux-like console with
	// neofetch output, a couple of commands and history chips.
	d.showView("terminal")
	if d.term != nil {
		d.term.run("ls -l")
		d.term.run("cat notes.md | grep alpha")
	}
	capture(t, c, "18-terminal")
	d.showView("chat")

	// 18. v1.0.6: Resources view — live usage bars, disk breakdown,
	// engine allocation + budgets.
	d.showView("resources")
	capture(t, c, "19-resources")
	d.showView("chat")

	// 19. v1.0.6: Vision chat — a user message carrying a real image
	// thumbnail + an assistant reply with the feedback buttons and
	// timestamps.
	shotPath := filepath.Join(d.cfg.ScreenshotsDir(), "screen-demo.png")
	writeTestPNG(t, shotPath, 480, 300)
	d.active.Messages = append(d.active.Messages, llm.Message{Role: "user", Content: "What's wrong with my screen?", At: time.Now().Add(-2 * time.Minute)})
	d.active.Messages = append(d.active.Messages, llm.Message{Role: "assistant", Content: "I can see a **license dialog** blocking the install. Click *Activate* to continue.", Feedback: 1, At: time.Now().Add(-90 * time.Second)})
	d.renderChat()
	// The image rides the user message via a direct bubble append.
	d.appendBubble(bubbleInfo{
		Role:    "user",
		Content: "What's wrong with my screen?",
		Images:  []string{shotPath},
		At:      time.Now().Add(-2 * time.Minute),
		OnZoom:  d.zoomImage,
	})
	d.appendBubble(bubbleInfo{
		Role:       "assistant",
		Content:    "I can see a **license dialog** blocking the install. Click *Activate* to continue.",
		At:         time.Now().Add(-90 * time.Second),
		Feedback:   1,
		OnFeedback: func(int) {},
	})
	capture(t, c, "20-vision-feedback")

	// 20. v1.0.6: the command palette (Ctrl+K) — rendered as a centered
	// overlay panel, same technique as the model picker shot.
	palSearch := widget.NewEntry()
	palSearch.SetPlaceHolder("Search commands…")
	palList := container.NewVBox()
	acts := d.paletteActions()
	if len(acts) > 9 {
		acts = acts[:9]
	}
	for _, a := range acts {
		palList.Add(newPaletteRow(a))
	}
	palBody := container.NewVBox(
		container.NewPadded(palSearch),
		widget.NewSeparator(),
		container.NewVScroll(palList),
	)
	palPanel := panel(palBody, 12, 10)
	// The production palette is a modal dialog with a dimmed backdrop —
	// reproduce that scrim so the shot represents the real look.
	dim := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 190})
	palOverlay := container.NewStack(content, dim, container.NewCenter(palPanel))
	c.SetContent(palOverlay)
	c.Resize(fyne.NewSize(1340, 840))
	capture(t, c, "21-palette")
	c.SetContent(content)
	c.Resize(fyne.NewSize(1340, 840))

	// 21. v1.0.7: Continuum — a chapter-2 session rendered end-to-end:
	// the briefing becomes the chapter divider card, the meter shows
	// live pressure with the CH 2 chip, the conversation continues.
	fw := continuum.Distill(continuum.NewFramework(), []llm.Message{
		{Role: "user", Content: "Remember my company is called Voltra and we use llama.cpp locally."},
		{Role: "assistant", Content: "Noted. I've created workspace/report.md with the market research skeleton."},
		{Role: "user", Content: "I prefer concise answers. Can you analyze charging speeds next?"},
	})
	_ = continuum.SaveFramework(d.cfg.SessionsDir, d.active.ID, fw)
	d.active.Chapter = 2
	d.active.ThreadID = "demo-thread"
	d.active.Messages = []llm.Message{
		{Role: "system", Content: continuum.Render(fw, 700)},
		{Role: "user", Content: "Continue the analysis.", At: time.Now().Add(-3 * time.Minute)},
		{Role: "assistant", Content: "Picking up where chapter 1 left off — the charging-speed comparison now covers 12 models across four price bands, and the Voltra positioning is highlighted in the summary table.", At: time.Now().Add(-2 * time.Minute)},
	}
	d.renderChat()
	d.updateContextMeter()
	// Force a visible pressure state (Set with from=0 sizes instantly).
	d.ctxMeter.Set(81, 2, 6480, 8000)
	capture(t, c, "22-continuum-chapter")

	// 22. v1.0.7: Memory view with the thread-state framework card.
	d.showView("memory")
	d.renderMemory()
	capture(t, c, "23-thread-state")
	d.showView("chat")

	fmt.Println("screenshots complete — see shots/")
}

// writeTestPNG writes a small deterministic PNG (the vision-attachment shot).
func writeTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := imgNew(w, h)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// imgNew paints a deterministic mock "screen": ember gradient + a light
// dialog rectangle + text-ish bars, so the thumbnail reads as a screenshot.
func imgNew(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b uint8
			switch {
			case x > w/4 && x < 3*w/4 && y > h/4 && y < 3*h/4:
				r, g, b = 46, 24, 18 // dialog
				if (x+y)%9 == 0 {
					r, g, b = 255, 138, 80 // border accents
				}
			default:
				r = uint8(18 + 30*x/w)
				g = uint8(10 + 14*y/h)
				b = uint8(8 + 20*x/w)
			}
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

// writeTestGGUF writes a minimal-but-valid GGUF v3 header for the model-card
// parser (shared shape with the cmd stress suite).
func writeTestGGUF(t *testing.T, path string) {
	t.Helper()
	buf := []byte("GGUF")
	buf = binary.LittleEndian.AppendUint32(buf, 3)
	buf = binary.LittleEndian.AppendUint64(buf, 1) // tensor count
	kvAt := len(buf)
	buf = binary.LittleEndian.AppendUint64(buf, 0) // kv count placeholder
	kv := 0
	addKV := func(key string, vtype uint32, val []byte) {
		buf = binary.LittleEndian.AppendUint64(buf, uint64(len(key)))
		buf = append(buf, key...)
		buf = binary.LittleEndian.AppendUint32(buf, vtype)
		buf = append(buf, val...)
		kv++
	}
	strVal := func(s string) []byte {
		b := binary.LittleEndian.AppendUint64(nil, uint64(len(s)))
		return append(b, s...)
	}
	addKV("general.architecture", 8, strVal("qwen2"))
	addKV("general.name", 8, strVal("Qwen2.5 7B Instruct"))
	addKV("general.parameter_count", 10, binary.LittleEndian.AppendUint64(nil, 7_615_000_000))
	addKV("general.file_type", 4, binary.LittleEndian.AppendUint32(nil, 15))
	addKV("qwen2.context_length", 4, binary.LittleEndian.AppendUint32(nil, 32768))
	binary.LittleEndian.PutUint64(buf[kvAt:kvAt+8], uint64(kv))
	buf = append(buf, make([]byte, 1<<20)...) // 1 MB of padding for a realistic size
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write test gguf: %v", err)
	}
}
