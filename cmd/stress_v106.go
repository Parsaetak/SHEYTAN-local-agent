package cmd

// v1.0.6 stress tests: the VISION release — multimodal wire format (mmproj
// pairing, image_url content parts), the screenshot tool + [[IMG:…]] bridge,
// the Linux simulator, feedback + recall steering, resources quotas, and the
// config surface.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sheytan/local-agent/internal/agent"
	"github.com/sheytan/local-agent/internal/aicontext"
	"github.com/sheytan/local-agent/internal/config"
	"github.com/sheytan/local-agent/internal/llm"
	"github.com/sheytan/local-agent/internal/recall"
	"github.com/sheytan/local-agent/internal/resources"
	"github.com/sheytan/local-agent/internal/tools"
	"github.com/sheytan/local-agent/internal/vision"
)

// --- config surface ---------------------------------------------------------

func stressV106Defaults() error {
	// v1.0.7: forward-compatible — v106 locks the >= 1.0.6 surface; the
	// current exact-version assertion lives in the latest release's tests.
	if !versionAtLeast(config.AppVersion, "1.0.6") {
		return fmt.Errorf("AppVersion = %q, want >= 1.0.6", config.AppVersion)
	}
	cfg := config.Default()
	if !cfg.VisionEnabled {
		return fmt.Errorf("VisionEnabled default must be true")
	}
	if cfg.VisionMMProj != "" {
		return fmt.Errorf("VisionMMProj default must be empty (auto)")
	}
	if cfg.MaxWorkspaceMB != 512 || cfg.MaxSessionsKept != 100 || cfg.MaxLogMB != 50 {
		return fmt.Errorf("quota defaults wrong: %d/%d/%d", cfg.MaxWorkspaceMB, cfg.MaxSessionsKept, cfg.MaxLogMB)
	}
	// helper clamps
	if cfg.MultiAgentDepth != 3 {
		return fmt.Errorf("MultiAgentDepth default = %d, want 3", cfg.MultiAgentDepth)
	}
	cfg.MultiAgentDepth = 99
	if cfg.EffectiveMultiAgentDepth() != 5 {
		return fmt.Errorf("depth clamp = %d, want 5", cfg.EffectiveMultiAgentDepth())
	}
	cfg.MaxSessionsKept = 3
	if cfg.EffectiveMaxSessionsKept() != 10 {
		return fmt.Errorf("sessions clamp = %d, want >= 10", cfg.EffectiveMaxSessionsKept())
	}
	return nil
}

// --- vision: detection + pairing ---------------------------------------------

func stressVisionMMProjDetection() error {
	if !vision.IsMMProj("mmproj-gemma-4-E2B-it-BF16.gguf") {
		return fmt.Errorf("canonical mmproj name must be detected")
	}
	if vision.IsMMProj("gemma-4-E2B-it-Q4_K_M.gguf") {
		return fmt.Errorf("a plain model must NOT be detected as mmproj")
	}
	if vision.IsMMProj("mmproj-gemma-4-E2B-it-BF16.bin") {
		return fmt.Errorf("non-gguf mmproj-ish name must not be detected")
	}
	return nil
}

// stressVisionProjectorPairing locks the EXACT user scenario from v1.0.6:
// gemma-4-E2B-it-Q4_K_M.gguf pairs with mmproj-gemma-4-E2B-it-BF16.gguf even
// though quantization (Q4_K_M) and precision (BF16) suffixes differ.
func stressVisionProjectorPairing() error {
	dir, err := os.MkdirTemp("", "sheytan-vision-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for _, f := range []string{
		"mmproj-gemma-4-E2B-it-BF16.gguf",
		"mmproj-gemma-3-4b.gguf",
		"gemma-4-E2B-it-Q4_K_M.gguf",
	} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			return err
		}
	}
	got := vision.FindProjector(dir, "gemma-4-E2B-it-Q4_K_M.gguf", "")
	if got == "" || !strings.Contains(got, "gemma-4-E2B") {
		return fmt.Errorf("E2B model must pair with the E2B projector, got %q", got)
	}
	// override wins
	if got := vision.FindProjector(dir, "gemma-4-E2B-it-Q4_K_M.gguf", "mmproj-gemma-3-4b.gguf"); got == "" || !strings.Contains(got, "gemma-3") {
		return fmt.Errorf("explicit override must win, got %q", got)
	}
	return nil
}

func stressModelListingExcludesMMProj() error {
	dir, err := os.MkdirTemp("", "sheytan-models106-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for _, f := range []string{
		"gemma-4-E2B-it-Q4_K_M.gguf",
		"mmproj-gemma-4-E2B-it-BF16.gguf",
		"qwen3-8b.gguf",
		"readme.txt",
	} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			return err
		}
	}
	got := llm.ListLocalModels(dir)
	if len(got) != 2 {
		return fmt.Errorf("ListLocalModels = %v, want the 2 chat models only (mmproj must be excluded)", got)
	}
	for _, m := range got {
		if strings.Contains(strings.ToLower(m), "mmproj") {
			return fmt.Errorf("mmproj leaked into the model list: %v", got)
		}
	}
	return nil
}

// --- engine args: --mmproj rides every ladder level --------------------------

func stressEngineMMProjArgs() error {
	cfg := config.Default()
	cfg.DataDir = os.TempDir()
	cfg.LlamaBinPath = filepath.Join(cfg.DataDir, "bin", "llama-server.exe")
	cfg.GPUAutoOffload = false
	srv := llm.NewLlamaServer(cfg)
	model := filepath.Join(os.TempDir(), "model.gguf")
	proj := filepath.Join(os.TempDir(), "mmproj-model.gguf")

	// Without a projector: no --mmproj at any level.
	for level := 0; level <= 3; level++ {
		if args := strings.Join(srv.BuildArgsForTest(model, level), " "); strings.Contains(args, "--mmproj") {
			return fmt.Errorf("level %d must not carry --mmproj without a projector: %s", level, args)
		}
	}
	// With a projector: --mmproj <path> present at EVERY level (the
	// vision-retry pass strips it only when it is the crash cause).
	srv.SetProjectorForTest(proj)
	for level := 0; level <= 3; level++ {
		args := strings.Join(srv.BuildArgsForTest(model, level), " ")
		if !strings.Contains(args, "--mmproj "+proj) {
			return fmt.Errorf("level %d missing --mmproj %s (have: %s)", level, proj, args)
		}
	}
	// Accessors.
	if srv.ProjectorPathForTest() != proj {
		return fmt.Errorf("ProjectorPath = %q, want %q", srv.ProjectorPathForTest(), proj)
	}
	srv.SetProjectorForTest("")
	if srv.ProjectorPathForTest() != "" {
		return fmt.Errorf("ProjectorPath must clear")
	}
	return nil
}

// --- multimodal wire format ----------------------------------------------------

// mustPNGFile writes a small test PNG and returns its path.
func mustPNGFile(dir, name string) (string, error) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.NRGBA{R: 255, G: 90, B: 38, A: 255})
		}
	}
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return "", err
	}
	return p, nil
}

// captureRequestServer runs a scripted server; the LAST request body is
// captured. Replies with non-streaming JSON (the client's Chat path).
func captureRequestServer(reply func(calls int32) string) (*httptest.Server, *atomic.Value, *atomic.Int32) {
	var lastBody atomic.Value
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastBody.Store(string(body))
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, reply(n))
	}))
	return srv, &lastBody, &calls
}

// stressWireMultimodalParts: a LOCAL chat request whose last user message
// carries an image must serialize as OpenAI content parts with a data URL.
func stressWireMultimodalParts() error {
	dir, err := os.MkdirTemp("", "sheytan-wire-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	imgPath, err := mustPNGFile(dir, "shot.png")
	if err != nil {
		return err
	}
	srv, lastBody, _ := captureRequestServer(func(int32) string {
		return "a screenshot"
	})
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "vision"
	client := llm.NewClient(cfg)

	req := client.BuildChatRequest("vision", []llm.Message{
		{Role: "user", Content: "what is this?", Images: []string{imgPath}},
	}, nil)
	if _, err := client.Chat(context.Background(), req); err != nil {
		return fmt.Errorf("chat: %v", err)
	}
	body, _ := lastBody.Load().(string)

	var probe struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &probe); err != nil {
		return fmt.Errorf("decode body: %v\n%s", err, body)
	}
	if len(probe.Messages) == 0 {
		return fmt.Errorf("no messages on the wire")
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(probe.Messages[len(probe.Messages)-1].Content, &parts); err != nil {
		return fmt.Errorf("last user content must be an array of parts, got: %s", probe.Messages[len(probe.Messages)-1].Content)
	}
	hasText, hasImage := false, false
	for _, p := range parts {
		if p.Type == "text" && strings.Contains(p.Text, "what is this?") {
			hasText = true
		}
		if p.Type == "image_url" && p.ImageURL != nil && strings.HasPrefix(p.ImageURL.URL, "data:image/png;base64,") {
			hasImage = true
		}
	}
	if !hasText || !hasImage {
		return fmt.Errorf("wire parts missing text/image: %+v", parts)
	}
	// Display fields never leak to the wire.
	if strings.Contains(body, `"images"`) || strings.Contains(body, `"feedback"`) || strings.Contains(body, `"reasoning"`) {
		return fmt.Errorf("display-only fields leaked onto the wire: %.200s", body)
	}
	return nil
}

// stressWireOldImagesDegrade: images on messages BEFORE the last user message
// become text notes (never re-encoded every iteration).
func stressWireOldImagesDegrade() error {
	dir, err := os.MkdirTemp("", "sheytan-wire2-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	imgPath, err := mustPNGFile(dir, "old.png")
	if err != nil {
		return err
	}
	srv, lastBody, _ := captureRequestServer(func(int32) string {
		return "ok"
	})
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "vision"
	client := llm.NewClient(cfg)

	req := client.BuildChatRequest("vision", []llm.Message{
		{Role: "user", Content: "look at this", Images: []string{imgPath}},
		{Role: "assistant", Content: "an orange square"},
		{Role: "user", Content: "and now?"},
	}, nil)
	if _, err := client.Chat(context.Background(), req); err != nil {
		return fmt.Errorf("chat: %v", err)
	}
	body, _ := lastBody.Load().(string)
	if strings.Contains(body, "data:image") {
		return fmt.Errorf("old-turn image must degrade to a text note, not ride as a data URL: %.300s", body)
	}
	if !strings.Contains(body, "[image attached earlier: old.png]") {
		return fmt.Errorf("degrading note missing: %.300s", body)
	}
	return nil
}

// stressWireRemoteToolImagesOff: tool-role images stay string content for
// REMOTE providers (OpenAI wire rules); local keeps parts.
func stressWireRemoteToolImagesOff() error {
	dir, err := os.MkdirTemp("", "sheytan-wire3-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	imgPath, err := mustPNGFile(dir, "tool.png")
	if err != nil {
		return err
	}
	srv, lastBody, _ := captureRequestServer(func(int32) string {
		return "done"
	})
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "vision"
	client := llm.NewClient(cfg)

	req := client.BuildChatRequest("vision", []llm.Message{
		{Role: "user", Content: "check the screen"},
		{Role: "tool", Content: "captured", Name: "screenshot", Images: []string{imgPath}},
	}, nil)
	if _, err := client.Chat(context.Background(), req); err != nil {
		return fmt.Errorf("chat: %v", err)
	}
	body, _ := lastBody.Load().(string)
	var probe struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &probe); err != nil {
		return fmt.Errorf("decode: %v", err)
	}
	var toolContent string
	for _, m := range probe.Messages {
		if m.Role == "tool" {
			toolContent = string(m.Content)
		}
	}
	if !strings.HasPrefix(toolContent, `"captured`) {
		return fmt.Errorf("remote tool content must stay a string (note appended), got: %s", toolContent)
	}
	return nil
}

// --- [[IMG:…]] marker bridge -----------------------------------------------------

func stressImageMarkerExtraction() error {
	clean, paths := agent.ExtractImageMarkers("Captured → C:\\shots\\s.png\n[[IMG:C:\\shots\\s.png]]")
	if len(paths) != 1 || paths[0] != "C:\\shots\\s.png" {
		return fmt.Errorf("paths = %v", paths)
	}
	if strings.Contains(clean, "[[IMG:") {
		return fmt.Errorf("marker leaked into clean text: %q", clean)
	}
	if !strings.Contains(clean, "Captured") {
		return fmt.Errorf("real content lost: %q", clean)
	}
	// multiple markers
	_, many := agent.ExtractImageMarkers("[[IMG:/a.png]]\nmid\n[[IMG:/b.png]]")
	if len(many) != 2 || many[0] != "/a.png" || many[1] != "/b.png" {
		return fmt.Errorf("multi-marker paths = %v", many)
	}
	// no markers → unchanged, nil
	plain, none := agent.ExtractImageMarkers("plain text")
	if plain != "plain text" || none != nil {
		return fmt.Errorf("plain passthrough broken: %q %v", plain, none)
	}
	// unclosed marker → treated as text
	_, zero := agent.ExtractImageMarkers("weird [[IMG: no close")
	if zero != nil {
		return fmt.Errorf("unclosed marker must not extract, got %v", zero)
	}
	return nil
}

// stressOrchestratorToolImagesBridge: a tool that emits [[IMG:…]] causes the
// NEXT request's tool message to carry the image as an image_url part (local
// provider) with the marker stripped from the text.
func stressOrchestratorToolImagesBridge() error {
	dir, err := os.MkdirTemp("", "sheytan-bridge-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	imgPath, err := mustPNGFile(dir, "screen.png")
	if err != nil {
		return err
	}

	var lastBody atomic.Value
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastBody.Store(string(body))
		n := calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"shot","arguments":"{}"}}]}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
		} else {
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"I see it"}}]}`))
			fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.Provider = config.ProviderRemote // remote STILL puts user images on the wire; tool images degrade — so assert user-image path here
	cfg.RemoteBaseURL = srv.URL + "/v1"
	cfg.RemoteModel = "vision"

	// The fake tool returns an image marker.
	orch := agent.New(cfg, llm.NewClient(cfg))
	orch.Register(imgTool{path: imgPath})

	_, err = orch.Run(context.Background(), []llm.Message{{Role: "user", Content: "look"}}, func(agent.Activity) {})
	if err != nil {
		return fmt.Errorf("run: %v", err)
	}
	body, _ := lastBody.Load().(string)
	// The second request must contain the tool message; the marker text must
	// be gone; the image note must be present.
	if strings.Contains(body, "[[IMG:") {
		return fmt.Errorf("marker leaked to the wire: %.300s", body)
	}
	if !strings.Contains(body, "captured the display") {
		return fmt.Errorf("tool result text missing: %.300s", body)
	}
	// For the REMOTE provider the tool image degrades to a note.
	if !strings.Contains(body, "[image attached earlier") {
		// local provider variant would carry data:image — both acceptable, but
		// with remote config the note is required.
		return fmt.Errorf("remote tool image must degrade to a note: %.300s", body)
	}
	return nil
}

// imgTool is a fake tool whose result carries an [[IMG:…]] marker.
type imgTool struct{ path string }

func (imgTool) Name() string        { return "shot" }
func (imgTool) Description() string { return "test image tool" }
func (imgTool) Parameters() any {
	return struct{}{}
}
func (t imgTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	return "captured the display\n[[IMG:" + t.path + "]]", nil
}

// --- screenshot tool gating -------------------------------------------------------

func stressScreenshotVisionGate() error {
	// No vision → the gate returns a teaching error before any capture.
	tools.VisionCheck = func() error { return fmt.Errorf("no multimodal projector paired") }
	defer func() { tools.VisionCheck = nil }()

	shot := tools.Screenshot{}
	_, err := shot.Run(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "no multimodal projector") {
		return fmt.Errorf("gated screenshot must fail with the teaching error, got: %v", err)
	}
	// Gate passes but capture is Windows-only → the platform error surfaces.
	tools.VisionCheck = func() error { return nil }
	_, err = shot.Run(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "Windows") {
		return fmt.Errorf("non-Windows capture must explain the platform limit, got: %v", err)
	}
	return nil
}

// --- feedback → recall steering -----------------------------------------------------

func stressRecallFeedbackBoost() error {
	dir, err := os.MkdirTemp("", "sheytan-fb-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	eng := recall.New(dir)
	if err := eng.IndexTurn("s1", "t", "shared topic alpha", "liked answer", nil); err != nil {
		return err
	}
	if err := eng.IndexTurn("s2", "t", "shared topic alpha", "disliked answer", nil); err != nil {
		return err
	}
	// identical BM25 scores → before feedback, order is stable (insertion).
	id1 := recall.CapsuleID("s1", "shared topic alpha")
	id2 := recall.CapsuleID("s2", "shared topic alpha")
	if err := eng.SetFeedback(id1, 1); err != nil {
		return err
	}
	if err := eng.SetFeedback(id2, -1); err != nil {
		return err
	}
	// duplicate SetFeedback must not append a second line
	if err := eng.SetFeedback(id1, 1); err != nil {
		return err
	}
	got := eng.Search("shared topic alpha", 2)
	if len(got) < 2 {
		return fmt.Errorf("search returned %d", len(got))
	}
	if got[0].Answer != "liked answer" {
		return fmt.Errorf("liked capsule must rank first, got %q", got[0].Answer)
	}
	likes, dislikes := eng.FeedbackStats()
	if likes != 1 || dislikes != 1 {
		return fmt.Errorf("stats = %d/%d, want 1/1", likes, dislikes)
	}
	if eng.FeedbackFor(id2) != -1 {
		return fmt.Errorf("FeedbackFor(disliked) = %d", eng.FeedbackFor(id2))
	}
	// A fresh engine reads the sidecar back.
	eng2 := recall.New(dir)
	if likes, dislikes := eng2.FeedbackStats(); likes != 1 || dislikes != 1 {
		return fmt.Errorf("sidecar reload = %d/%d, want 1/1", likes, dislikes)
	}
	return nil
}

// --- message field roundtrip --------------------------------------------------------

func stressMessageV106Fields() error {
	at := time.Now().UTC().Truncate(time.Second)
	msgs := []llm.Message{{
		Role:     "assistant",
		Content:  "here you go",
		Feedback: 1,
		Images:   []string{"/tmp/a.png"},
		At:       at,
	}}
	data, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	var back []llm.Message
	if err := json.Unmarshal(data, &back); err != nil {
		return err
	}
	if back[0].Feedback != 1 || len(back[0].Images) != 1 || !back[0].At.Equal(at) {
		return fmt.Errorf("roundtrip lost fields: %+v", back[0])
	}
	// StripReasoning clears the DISPLAY-only fields. Images stay: they are
	// consumed by the multimodal wire conversion (image_url parts), and the
	// wireMessage shape never carries the raw field to the API.
	stripped := llm.StripReasoning(msgs)
	if stripped[0].Feedback != 0 || !stripped[0].At.IsZero() {
		return fmt.Errorf("StripReasoning must clear feedback+at")
	}
	if len(stripped[0].Images) != 1 {
		return fmt.Errorf("StripReasoning must KEEP images for the wire conversion")
	}
	return nil
}

// --- image cache -----------------------------------------------------------------------

func stressClientImageCache() error {
	dir, err := os.MkdirTemp("", "sheytan-cache-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	imgPath, err := mustPNGFile(dir, "c.png")
	if err != nil {
		return err
	}
	srv, _, _ := captureRequestServer(func(int32) string {
		return "ok"
	})
	defer srv.Close()
	cfg := config.Default()
	cfg.Provider = config.ProviderRemote
	cfg.RemoteBaseURL = srv.URL + "/v1"
	client := llm.NewClient(cfg)

	msg := llm.Message{Role: "user", Content: "see", Images: []string{imgPath}}
	for i := 0; i < 3; i++ {
		req := client.BuildChatRequest("m", []llm.Message{msg}, nil)
		if _, err := client.Chat(context.Background(), req); err != nil {
			return fmt.Errorf("chat %d: %v", i, err)
		}
	}
	if n := client.ImageCacheLenForTest(); n != 1 {
		return fmt.Errorf("3 identical sends must produce exactly 1 cache entry, got %d", n)
	}
	return nil
}

// --- linux tool ------------------------------------------------------------------------------

func stressLinuxTool() error {
	dir, err := os.MkdirTemp("", "sheytan-linux-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		return err
	}
	sim := tools.NewLinuxSim(dir)
	if sim.Name() != "linux" {
		return fmt.Errorf("tool name = %q", sim.Name())
	}
	out, err := sim.Run(context.Background(), json.RawMessage(`{"command":"cat notes.md | wc -l"}`))
	if err != nil {
		return err
	}
	if !strings.Contains(out, "2") {
		return fmt.Errorf("pipe through the tool = %q", out)
	}
	// jail: escape refused
	if out, _ := sim.Run(context.Background(), json.RawMessage(`{"command":"cat ../../etc/passwd"}`)); !strings.Contains(out, "denied") && !strings.Contains(out, "no such file") {
		return fmt.Errorf("jail escape attempt leaked: %q", out)
	}
	// empty command → error
	if _, err := sim.Run(context.Background(), json.RawMessage(`{"command":"  "}`)); err == nil {
		return fmt.Errorf("empty command must error")
	}
	return nil
}

// --- resources ----------------------------------------------------------------------------------

func stressResourcesScanQuota() error {
	dir, err := os.MkdirTemp("", "sheytan-res-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.MkdirAll(filepath.Join(dir, "models"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "models", "m.gguf"), make([]byte, 4096), 0o644); err != nil {
		return err
	}
	usage := resources.Scan(dir)
	if len(usage) == 0 || usage[0].Name != "Models" || usage[0].Bytes != 4096 {
		return fmt.Errorf("scan = %+v", usage)
	}
	// The self process is probeable.
	if _, err := resources.ProcRAM(os.Getpid()); err != nil {
		return fmt.Errorf("ProcRAM(self): %v", err)
	}
	return nil
}

// --- aicontext v6 -----------------------------------------------------------------------------------

func stressAicontextV6() error {
	// v1.0.7: forward-compatible — the instruction file only moves forward.
	if aicontext.ContextVersion < 6 {
		return fmt.Errorf("ContextVersion = %d, want >= 6", aicontext.ContextVersion)
	}
	// The vision briefing line appears exactly when a projector pairs.
	dir, err := os.MkdirTemp("", "sheytan-ctx-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	modelsDir := filepath.Join(dir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		return err
	}
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.ModelsDir = modelsDir
	cfg.Model = "gemma-4-E2B-it-Q4_K_M.gguf"
	if err := os.WriteFile(filepath.Join(modelsDir, cfg.Model), []byte("x"), 0o644); err != nil {
		return err
	}
	if strings.Contains(aicontext.Briefing(cfg), "Vision: ENABLED") {
		return fmt.Errorf("vision line must be absent without a projector")
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "mmproj-gemma-4-E2B-it-BF16.gguf"), []byte("x"), 0o644); err != nil {
		return err
	}
	brief := aicontext.Briefing(cfg)
	if !strings.Contains(brief, "Vision: ENABLED") || !strings.Contains(brief, "mmproj-gemma-4-E2B-it-BF16.gguf") {
		return fmt.Errorf("vision line missing with a paired projector: %.300s", brief)
	}
	if !strings.Contains(brief, "screenshot") || !strings.Contains(brief, "linux") {
		return fmt.Errorf("new tools missing from the LIVE ENVIRONMENT tool list")
	}
	// EnsureFile regenerates an outdated marker (v5 → current).
	if _, err := aicontext.EnsureFile(dir); err != nil {
		return err
	}
	old := "<!-- sheytan-context-version: 5 -->\n# old\n"
	if err := os.WriteFile(filepath.Join(dir, aicontext.FileName), []byte(old), 0o644); err != nil {
		return err
	}
	if _, err := aicontext.EnsureFile(dir); err != nil {
		return err
	}
	data, _ := os.ReadFile(filepath.Join(dir, aicontext.FileName))
	// v1.0.7: the upgrade target is whatever the current embedded
	// version is (>= 6), not a pinned number.
	if !strings.Contains(string(data), fmt.Sprintf("sheytan-context-version: %d", aicontext.ContextVersion)) {
		return fmt.Errorf("EnsureFile did not upgrade a v5 file")
	}
	return nil
}

// --- wire data-URL sanity (encoding path) ----------------------------------------------------------

func stressVisionEncodeImage() error {
	dir, err := os.MkdirTemp("", "sheytan-enc-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	imgPath, err := mustPNGFile(dir, "e.png")
	if err != nil {
		return err
	}
	url, err := vision.EncodeImage(imgPath)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		return fmt.Errorf("bad prefix: %.40s", url)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/png;base64,"))
	if err != nil {
		return fmt.Errorf("base64 payload invalid: %v", err)
	}
	if !strings.HasPrefix(string(raw), "\x89PNG") {
		return fmt.Errorf("payload is not PNG bytes")
	}
	return nil
}
