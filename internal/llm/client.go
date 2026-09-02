package llm

import (
        "bufio"
        "bytes"
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "io"
        "net/http"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "time"

        "github.com/Parsaetak/SHEYTAN-local-agent/internal/config"
        "github.com/Parsaetak/SHEYTAN-local-agent/internal/logging"
        "github.com/Parsaetak/SHEYTAN-local-agent/internal/netcheck"
        "github.com/Parsaetak/SHEYTAN-local-agent/internal/vision"
)

// Message is an OpenAI-style chat message.
type Message struct {
        Role       string     `json:"role"`
        Content    string     `json:"content,omitempty"`
        ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
        ToolCallID string     `json:"tool_call_id,omitempty"`
        Name       string     `json:"name,omitempty"`

        // Reasoning holds the model's separate thinking trace (v1.0.2
        // thinking mode): native reasoning_content from the endpoint, or
        // extracted <think>…</think> blocks. Persisted with the session but
        // never re-sent to the API (json:"-" on the wire would break some
        // strict servers, so it is stripped when building requests via
        // StripReasoning).
        Reasoning   string   `json:"reasoning,omitempty"`
        Attachments []string `json:"attachments,omitempty"` // display-only file names

        // Images (v1.0.6 vision): file paths of images attached to this
        // message. Persisted with the session so history keeps them; on the
        // WIRE they are converted into OpenAI content parts (image_url data
        // URLs) for the current turn and degraded to text notes for older
        // turns (see wireMessages — re-encoding every historical image every
        // iteration would stall the agent loop).
        Images []string `json:"images,omitempty"`

        // Feedback (v1.0.6): the user's 👍/👎 verdict on an assistant message.
        // 0 = none, +1 = like, -1 = dislike. Persisted with the session and
        // mirrored into the recall engine (liked answers rank higher in future
        // memory searches). Never sent to the API.
        Feedback int `json:"feedback,omitempty"`

        // At (v1.0.6): when the message was sent — rendered as the bubble's
        // timestamp. Zero = unknown (legacy sessions). Never sent to the API.
        At time.Time `json:"at,omitempty"`
}

// StripReasoning returns a copy of msgs with the reasoning and attachment
// display fields removed — the chat-completions wire format knows neither.
func StripReasoning(msgs []Message) []Message {
        out := make([]Message, len(msgs))
        copy(out, msgs)
        for i := range out {
                out[i].Reasoning = ""
                out[i].Attachments = nil
                out[i].Feedback = 0
                out[i].At = time.Time{}
        }
        return out
}

type ToolCall struct {
        // Index identifies the streaming slot this delta belongs to. OpenAI-
        // compatible servers stream tool-call arguments in fragments tagged
        // with index; assembling by index (not by arrival order) is required
        // when several tool calls are streamed interleaved.
        Index    int    `json:"index"`
        ID       string `json:"id"`
        Type     string `json:"type"`
        Function struct {
                Name      string `json:"name"`
                Arguments string `json:"arguments"`
        } `json:"function"`
}

// Client is the OpenAI-compatible chat client. It talks to either the
// bundled llama.cpp server (provider=local) or any remote OpenAI-compatible
// endpoint (provider=remote) using cfg.EffectiveBaseURL/APIKey.
type Client struct {
        cfg  *config.Config
        http *http.Client

        // v1.0.6: encoded-image cache. The agent loop rebuilds the request
        // every iteration; without the cache a 1 MB screenshot would be
        // base64-encoded dozens of times per turn. Keyed by path, invalidated
        // by mtime, hard-capped at 8 entries (one turn's worth).
        imgCacheMu sync.Mutex
        imgCache   map[string]imageCacheEntry
}

type imageCacheEntry struct {
        modTime time.Time
        url     string
}

// newTunedTransport builds the shared HTTP transport (v1.0.4 Speed Pack):
// keep-alive connections are pooled per host so the health checks, the
// streaming chat calls, and parallel tool requests reuse one warm
// connection instead of paying a fresh TCP handshake every time.
func newTunedTransport() *http.Transport {
        return &http.Transport{
                MaxIdleConns:        16,
                MaxIdleConnsPerHost: 8,
                IdleConnTimeout:     120 * time.Second,
                DisableCompression:  false,
        }
}

func NewClient(cfg *config.Config) *Client {
        return &Client{
                cfg:  cfg,
                http: &http.Client{Timeout: 10 * time.Minute, Transport: newTunedTransport()},
        }
}

// ChatRequest is the body sent to /v1/chat/completions.
type ChatRequest struct {
        Model       string     `json:"model"`
        Messages    []Message  `json:"-"` // marshaled via MarshalJSON (wire form)
        Temperature float64    `json:"temperature,omitempty"`
        TopP        float64    `json:"top_p,omitempty"`
        TopK        int        `json:"top_k,omitempty"`
        MaxTokens   int        `json:"max_tokens,omitempty"`
        Stop        []string   `json:"stop,omitempty"`
        Seed        int        `json:"seed,omitempty"`
        Stream      bool       `json:"stream,omitempty"`
        Tools       []ToolSpec `json:"tools,omitempty"`
        NumCtx      int        `json:"n_ctx,omitempty"`
        // CachePrompt asks llama.cpp to reuse the KV cache across turns
        // (v1.0.4). Together with --cache-reuse on the server this collapses
        // the repeated agent prefix (AI context + tool schemas) to a near-zero
        // prefill after the first turn. Harmless no-op on other servers.
        CachePrompt bool `json:"cache_prompt"`

        // wire (v1.0.6) is the precomputed multimodal wire form produced by
        // BuildChatRequest: messages with images become OpenAI content parts.
        // nil → MarshalJSON derives a plain string-content wire copy (the
        // pre-v1.0.6 behavior, used by direct constructions like the
        // multi-agent planner prompts).
        wire []wireMessage `json:"-"`
}

// wireMessage is the on-the-wire message: Content is either a string or a
// content-parts array (multimodal).
type wireMessage struct {
        Role       string     `json:"role"`
        Content    any        `json:"content,omitempty"` // string | []contentPart
        ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
        ToolCallID string     `json:"tool_call_id,omitempty"`
        Name       string     `json:"name,omitempty"`
}

type contentPart struct {
        Type     string     `json:"type"` // "text" | "image_url"
        Text     string     `json:"text,omitempty"`
        ImageURL *imageURL  `json:"image_url,omitempty"`
}

type imageURL struct {
        URL string `json:"url"`
}

// MarshalJSON projects the request into the OpenAI wire format: the
// precomputed multimodal wire form when present, otherwise the plain
// string-content form. Display-only fields (reasoning, attachments,
// feedback, image paths) never reach the wire.
func (r *ChatRequest) MarshalJSON() ([]byte, error) {
        msgs := r.wire
        if msgs == nil && r.Messages != nil {
                msgs = plainWireMessages(r.Messages)
        }
        return json.Marshal(struct {
                Model       string      `json:"model"`
                Messages    interface{} `json:"messages"`
                Temperature float64     `json:"temperature,omitempty"`
                TopP        float64     `json:"top_p,omitempty"`
                TopK        int         `json:"top_k,omitempty"`
                MaxTokens   int         `json:"max_tokens,omitempty"`
                Stop        []string    `json:"stop,omitempty"`
                Seed        int         `json:"seed,omitempty"`
                Stream      bool        `json:"stream,omitempty"`
                Tools       []ToolSpec  `json:"tools,omitempty"`
                NumCtx      int         `json:"n_ctx,omitempty"`
                CachePrompt bool        `json:"cache_prompt"`
        }{
                Model:       r.Model,
                Messages:    msgs,
                Temperature: r.Temperature,
                TopP:        r.TopP,
                TopK:        r.TopK,
                MaxTokens:   r.MaxTokens,
                Stop:        r.Stop,
                Seed:        r.Seed,
                Stream:      r.Stream,
                Tools:       r.Tools,
                NumCtx:      r.NumCtx,
                CachePrompt: r.CachePrompt,
        })
}

// plainWireMessages is the pre-v1.0.6 projection: string content, display
// fields stripped, images degraded to text notes (callers that build a
// ChatRequest directly never carry images — this is a safety net).
func plainWireMessages(msgs []Message) []wireMessage {
        out := make([]wireMessage, 0, len(msgs))
        for _, m := range msgs {
                wm := wireMessage{
                        Role:       m.Role,
                        ToolCalls:  m.ToolCalls,
                        ToolCallID: m.ToolCallID,
                        Name:       m.Name,
                }
                content := m.Content
                if len(m.Images) > 0 {
                        var names []string
                        for _, p := range m.Images {
                                names = append(names, filepath.Base(p))
                        }
                        note := "[image attached: " + strings.Join(names, ", ") + "]"
                        if content == "" {
                                content = note
                        } else {
                                content += "\n" + note
                        }
                }
                if content != "" {
                        wm.Content = content
                }
                out = append(out, wm)
        }
        return out
}

type ToolSpec struct {
        Type     string `json:"type"`
        Function struct {
                Name        string `json:"name"`
                Description string `json:"description"`
                Parameters  any    `json:"parameters"`
        } `json:"function"`
}

// ChatResponse is a non-streaming response.
type ChatResponse struct {
        Choices []struct {
                Message      Message `json:"message"`
                FinishReason string  `json:"finish_reason"`
        } `json:"choices"`
        Usage struct {
                PromptTokens     int `json:"prompt_tokens"`
                CompletionTokens int `json:"completion_tokens"`
                TotalTokens      int `json:"total_tokens"`
        } `json:"usage"`
}

// baseURL/apiKey resolve the active provider endpoints.
func (c *Client) baseURL() string { return c.cfg.EffectiveBaseURL() }
func (c *Client) apiKey() string  { return c.cfg.EffectiveAPIKey() }

// promptStats returns message count + total chars for logging.
func promptStats(msgs []Message) (int, int) {
        n := 0
        for _, m := range msgs {
                n += len(m.Content)
        }
        return len(msgs), n
}

// offlineError is the fail-fast error returned when a remote provider is
// unreachable because the machine has no internet. The GUI surfaces its
// message directly so the user knows local mode keeps working.
func (c *Client) offlineError() error {
        return errors.New("remote LLM endpoint unreachable: no internet connection detected. " +
                "Switch to the local provider (Agent → LLM provider → local llama.cpp) to keep working offline — " +
                "all local tools continue to function")
}

// isLocalBaseURL reports whether an endpoint lives on this machine
// (127.0.0.1, localhost, ::1). Local servers — llama.cpp, Ollama, LM Studio
// — keep working while offline, so the offline fast-fail must not fire.
func isLocalBaseURL(base string) bool {
        b := strings.ToLower(base)
        return strings.Contains(b, "localhost") || strings.Contains(b, "127.0.0.1") ||
                strings.Contains(b, "[::1]") || strings.Contains(b, "0.0.0.0")
}

// offlineBlocked reports whether the active provider is a REMOTE endpoint
// that cannot be reached while offline.
func (c *Client) offlineBlocked() bool {
        return c.cfg.IsRemote() && !isLocalBaseURL(c.baseURL()) && netcheck.IsOffline()
}

// Chat sends a non-streaming chat request with retry on transient errors.
func (c *Client) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
        // Offline fail-fast: skip the 4-attempt/19s backoff ladder when the
        // machine simply has no connectivity (local endpoints are exempt).
        if c.offlineBlocked() {
                err := c.offlineError()
                c.logCall(req, time.Now(), 0, 0, "", err)
                return nil, err
        }
        req.Stream = false
        body, err := json.Marshal(req)
        if err != nil {
                return nil, err
        }
        var lastErr error
        for attempt := 0; attempt < 4; attempt++ {
                if attempt > 0 {
                        logging.Default().Warn("llm", "retry %d after %v", attempt, lastErr)
                        if err := sleepCtx(ctx, retryDelay(attempt)); err != nil {
                                return nil, err
                        }
                }
                httpReq, err := http.NewRequestWithContext(ctx, "POST",
                        c.baseURL()+"/chat/completions", bytes.NewReader(body))
                if err != nil {
                        return nil, err
                }
                httpReq.Header.Set("Content-Type", "application/json")
                httpReq.Header.Set("Authorization", "Bearer "+c.apiKey())

                start := time.Now()
                resp, err := c.http.Do(httpReq)
                if err != nil {
                        lastErr = fmt.Errorf("LLM call: %w", err)
                        c.logCall(req, start, 0, 0, "", lastErr)
                        if isTransient(err) {
                                continue
                        }
                        return nil, lastErr
                }
                if resp.StatusCode == 429 || resp.StatusCode >= 500 {
                        buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
                        resp.Body.Close()
                        lastErr = fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, truncateStr(string(buf), 512))
                        continue // retryable
                }
                if resp.StatusCode != 200 {
                        buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
                        resp.Body.Close()
                        err := fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, truncateStr(string(buf), 512))
                        c.logCall(req, start, 0, 0, "", err)
                        return nil, err
                }
                var out ChatResponse
                decodeErr := json.NewDecoder(resp.Body).Decode(&out)
                resp.Body.Close()
                if decodeErr != nil {
                        c.logCall(req, start, 0, 0, "", decodeErr)
                        return nil, decodeErr
                }
                finish := ""
                contentLen := 0
                toolCalls := 0
                if len(out.Choices) > 0 {
                        finish = out.Choices[0].FinishReason
                        contentLen = len(out.Choices[0].Message.Content)
                        toolCalls = len(out.Choices[0].Message.ToolCalls)
                }
                c.logCall(req, start, contentLen, toolCalls, finish, nil)
                return &out, nil
        }
        return nil, lastErr
}

// isTransient reports whether a transport error is worth retrying.
func isTransient(err error) bool {
        if err == nil {
                return false
        }
        msg := strings.ToLower(err.Error())
        return strings.Contains(msg, "connection reset") ||
                strings.Contains(msg, "connection refused") ||
                strings.Contains(msg, "eof") ||
                strings.Contains(msg, "timeout") ||
                strings.Contains(msg, "broken pipe")
}

// retryDelay returns the backoff before retry attempt n (1-based):
// 2s, 5s, 12s — long enough to ride out API rate-limit windows.
func retryDelay(attempt int) time.Duration {
        switch attempt {
        case 1:
                return 2 * time.Second
        case 2:
                return 5 * time.Second
        default:
                return 12 * time.Second
        }
}

// sleepCtx sleeps for d or until ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
        select {
        case <-time.After(d):
                return nil
        case <-ctx.Done():
                return ctx.Err()
        }
}

// StreamChat sends a streaming chat request. The callback is invoked for
// every chunk. A chunk may contain a delta of either content text or a
// tool_call. When `finishReason` is non-empty the stream is done.
//
// Robustness notes:
//   - tool-call argument fragments are assembled BY INDEX, so multiple
//     interleaved tool calls stream correctly
//   - if the server ignores `stream:true` and returns plain JSON, the
//     response is decoded and replayed as a single event
//   - both `data: ` and `data:` prefixes are accepted
type StreamEvent struct {
        Content      string
        Reasoning    string // native thinking trace delta (reasoning_content / reasoning)
        ToolCalls    []ToolCall
        FinishReason string
        Usage        *struct {
                PromptTokens     int `json:"prompt_tokens"`
                CompletionTokens int `json:"completion_tokens"`
                TotalTokens      int `json:"total_tokens"`
        }
}

// PerfStats is the live speed telemetry of one streaming call (v1.0.4
// Speed Pack): time-to-first-token and tokens/sec, LM Studio-style — what
// local-LLM users expect to see.
type PerfStats struct {
        TTFTMs       int     // request start → first content delta
        Tokens       int     // completion tokens (usage when present, else deltas)
        TokensPerSec float64 // generation throughput
        PromptTokens int     // from usage when the server reports it
        WallMs       int     // total call duration
}

// String renders the compact HUD line: "41.2 tok/s · first token 0.8s".
func (p PerfStats) String() string {
        if p.TokensPerSec <= 0 || p.WallMs <= 0 {
                return ""
        }
        s := fmt.Sprintf("%.1f tok/s", p.TokensPerSec)
        if p.TTFTMs > 0 {
                s += fmt.Sprintf(" · first token %.1fs", float64(p.TTFTMs)/1000)
        }
        return s
}

// toolCallAssembler assembles streaming tool-call deltas by index.
type toolCallAssembler struct {
        order []int       // indices in first-seen order
        byIdx map[int]int // index -> position in calls
        calls map[int]*ToolCall
}

func newAssembler() *toolCallAssembler {
        return &toolCallAssembler{byIdx: map[int]int{}, calls: map[int]*ToolCall{}}
}

func (a *toolCallAssembler) add(tc ToolCall) {
        if _, ok := a.byIdx[tc.Index]; !ok {
                a.byIdx[tc.Index] = len(a.order)
                a.order = append(a.order, tc.Index)
        }
        pos := a.byIdx[tc.Index]
        if _, ok := a.calls[pos]; !ok {
                clone := tc
                a.calls[pos] = &clone
                return
        }
        existing := a.calls[pos]
        if tc.ID != "" {
                existing.ID = tc.ID
        }
        if tc.Type != "" {
                existing.Type = tc.Type
        }
        if tc.Function.Name != "" {
                existing.Function.Name = tc.Function.Name
        }
        if tc.Function.Arguments != "" {
                existing.Function.Arguments += tc.Function.Arguments
        }
}

func (a *toolCallAssembler) all() []ToolCall {
        out := make([]ToolCall, 0, len(a.order))
        for _, idx := range a.order {
                if c, ok := a.calls[a.byIdx[idx]]; ok {
                        out = append(out, *c)
                }
        }
        return out
}

func (c *Client) StreamChat(ctx context.Context, req *ChatRequest, onEvent func(StreamEvent) error) error {
        _, err := c.StreamChatDetailed(ctx, req, onEvent)
        return err
}

// StreamChatDetailed streams like StreamChat and additionally returns the
// performance telemetry of the successful attempt (v1.0.4).
func (c *Client) StreamChatDetailed(ctx context.Context, req *ChatRequest, onEvent func(StreamEvent) error) (PerfStats, error) {
        // Offline fail-fast (remote provider): skip retries entirely.
        if c.offlineBlocked() {
                err := c.offlineError()
                c.logCall(req, time.Now(), 0, 0, "", err)
                return PerfStats{}, err
        }
        req.Stream = true
        // v1.0.4: ask the engine to reuse the cached prompt prefix across
        // turns (agent loop prefill collapse; no-op elsewhere).
        req.CachePrompt = true
        body, err := json.Marshal(req)
        if err != nil {
                return PerfStats{}, err
        }
        var lastErr error
        var perf PerfStats
        for attempt := 0; attempt < 4; attempt++ {
                if attempt > 0 {
                        logging.Default().Warn("llm", "stream retry %d after %v", attempt, lastErr)
                        if err := sleepCtx(ctx, retryDelay(attempt)); err != nil {
                                return perf, err
                        }
                }
                var emitted bool
                // per-attempt telemetry
                tr := newTokenTimer()
                lastErr = c.streamOnce(ctx, req, body, func(ev StreamEvent) error {
                        emitted = true
                        tr.observe(ev)
                        return onEvent(ev)
                })
                perf = tr.stats()
                // Retry only when nothing was emitted yet (retrying mid-stream
                // would duplicate content the caller already saw).
                if lastErr == nil || emitted {
                        return perf, lastErr
                }
                if !isTransient(lastErr) && !isRetryableStatus(lastErr) {
                        return perf, lastErr
                }
        }
        return perf, lastErr
}

// tokenTimer measures TTFT + throughput of one streaming attempt.
type tokenTimer struct {
        start    time.Time
        firstAt  time.Time
        deltas   int // content/reasoning deltas seen (token approximation)
        usageTok int // authoritative completion tokens when reported
        prompt   int
        done     time.Time
}

func newTokenTimer() *tokenTimer { return &tokenTimer{start: time.Now()} }

func (t *tokenTimer) observe(ev StreamEvent) {
        if ev.Content != "" || ev.Reasoning != "" {
                t.deltas++
                if t.firstAt.IsZero() {
                        t.firstAt = time.Now()
                }
        }
        if ev.Usage != nil {
                if ev.Usage.CompletionTokens > 0 {
                        t.usageTok = ev.Usage.CompletionTokens
                }
                if ev.Usage.PromptTokens > 0 {
                        t.prompt = ev.Usage.PromptTokens
                }
        }
}

func (t *tokenTimer) stats() PerfStats {
        t.done = time.Now()
        p := PerfStats{WallMs: int(t.done.Sub(t.start).Milliseconds()), PromptTokens: t.prompt}
        if !t.firstAt.IsZero() {
                p.TTFTMs = int(t.firstAt.Sub(t.start).Milliseconds())
        }
        tokens := t.usageTok
        if tokens == 0 {
                tokens = t.deltas
        }
        p.Tokens = tokens
        genMs := t.done.Sub(t.firstAt).Milliseconds()
        if tokens > 0 && genMs > 0 {
                p.TokensPerSec = float64(tokens) / (float64(genMs) / 1000)
        }
        return p
}

// isRetryableStatus inspects an error produced by streamOnce for HTTP-level
// retry codes embedded in the message.
func isRetryableStatus(err error) bool {
        if err == nil {
                return false
        }
        msg := err.Error()
        return strings.Contains(msg, "HTTP 429") || strings.Contains(msg, "HTTP 5")
}

// SSE wire tokens (v1.0.9 byte-level pump).
var (
        sseDataPrefix = []byte("data:")
        sseDoneToken  = []byte("[DONE]")
)

func (c *Client) streamOnce(ctx context.Context, req *ChatRequest, body []byte, onEvent func(StreamEvent) error) error {
        httpReq, err := http.NewRequestWithContext(ctx, "POST",
                c.baseURL()+"/chat/completions", bytes.NewReader(body))
        if err != nil {
                return err
        }
        httpReq.Header.Set("Content-Type", "application/json")
        httpReq.Header.Set("Authorization", "Bearer "+c.apiKey())
        httpReq.Header.Set("Accept", "text/event-stream")

        start := time.Now()
        resp, err := c.http.Do(httpReq)
        if err != nil {
                err = fmt.Errorf("LLM stream: %w", err)
                c.logCall(req, start, 0, 0, "", err)
                return err
        }
        defer resp.Body.Close()
        if resp.StatusCode != 200 {
                buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
                err := fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, truncateStr(string(buf), 512))
                c.logCall(req, start, 0, 0, "", err)
                return err
        }

        // Non-SSE fallback: server returned plain JSON despite stream=true.
        if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "text/event-stream") {
                var out ChatResponse
                if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
                        c.logCall(req, start, 0, 0, "", err)
                        return err
                }
                ev := StreamEvent{}
                if len(out.Choices) > 0 {
                        ev.Content = out.Choices[0].Message.Content
                        ev.Reasoning = out.Choices[0].Message.Reasoning
                        ev.ToolCalls = out.Choices[0].Message.ToolCalls
                        ev.FinishReason = out.Choices[0].FinishReason
                        if ev.FinishReason == "" {
                                ev.FinishReason = "stop"
                        }
                }
                c.logCall(req, start, len(ev.Content), len(ev.ToolCalls), ev.FinishReason, nil)
                return onEvent(ev)
        }

        // v1.0.9 (TURBINE): the SSE pump scans BYTES — scanner.Bytes() with
        // prefix/trim handled at the byte level, so a data line only becomes a
        // string when it is actually a JSON chunk worth decoding. The old loop
        // allocated two strings per SSE line (scanner.Text() + TrimPrefix) even
        // for comment/keep-alive lines, which on a fast stream meant thousands
        // of short-lived allocations per reply (GC pressure = dropped frames).
        scanner := bufio.NewScanner(resp.Body)
        scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
        asm := newAssembler()
        var contentChars int
        var finishReason string
        var streamErr error

        emit := func(ev StreamEvent) error {
                contentChars += len(ev.Content)
                if ev.FinishReason != "" {
                        finishReason = ev.FinishReason
                }
                return onEvent(ev)
        }

        defer func() {
                c.logCall(req, start, contentChars, len(asm.all()), finishReason, streamErr)
        }()

        for scanner.Scan() {
                line := scanner.Bytes()
                if !bytes.HasPrefix(line, sseDataPrefix) {
                        continue
                }
                data := bytes.TrimSpace(line[len(sseDataPrefix):])
                if len(data) == 0 {
                        continue
                }
                if bytes.Equal(data, sseDoneToken) {
                        break
                }
                var chunk struct {
                        Choices []struct {
                                Delta struct {
                                        Content    string     `json:"content"`
                                        Reasoning  string     `json:"reasoning_content"` // DeepSeek/GLM style
                                        Reasoning2 string     `json:"reasoning"`         // some vendors' alt field
                                        ToolCalls  []ToolCall `json:"tool_calls"`
                                        Role       string     `json:"role"`
                                } `json:"delta"`
                                Message struct {
                                        Content   string     `json:"content"`
                                        Reasoning string     `json:"reasoning_content"`
                                        ToolCalls []ToolCall `json:"tool_calls"`
                                } `json:"message"` // some servers send full messages mid-stream
                                FinishReason string `json:"finish_reason"`
                        } `json:"choices"`
                        Usage *struct {
                                PromptTokens     int `json:"prompt_tokens"`
                                CompletionTokens int `json:"completion_tokens"`
                                TotalTokens      int `json:"total_tokens"`
                        } `json:"usage"`
                }
                if err := json.Unmarshal(data, &chunk); err != nil {
                        continue
                }
                ev := StreamEvent{Usage: chunk.Usage}
                if len(chunk.Choices) > 0 {
                        choice := chunk.Choices[0]
                        ev.Content = choice.Delta.Content
                        ev.Reasoning = choice.Delta.Reasoning
                        if ev.Reasoning == "" {
                                ev.Reasoning = choice.Delta.Reasoning2
                        }
                        // Full-message variant (some servers emit it for the
                        // first chunk or when replaying non-SSE JSON as SSE).
                        if ev.Content == "" && choice.Message.Content != "" {
                                ev.Content = choice.Message.Content
                        }
                        if ev.Reasoning == "" && choice.Message.Reasoning != "" {
                                ev.Reasoning = choice.Message.Reasoning
                        }
                        if len(choice.Message.ToolCalls) > 0 && len(choice.Delta.ToolCalls) == 0 {
                                ev.ToolCalls = choice.Message.ToolCalls
                        }
                        ev.FinishReason = choice.FinishReason
                        if len(choice.Delta.ToolCalls) > 0 {
                                for _, tc := range choice.Delta.ToolCalls {
                                        asm.add(tc)
                                }
                                ev.ToolCalls = asm.all()
                        }
                }
                if err := emit(ev); err != nil {
                        streamErr = err
                        return err
                }
        }
        if err := scanner.Err(); err != nil {
                streamErr = err
                return err
        }
        return nil
}

// logCall records one LLM call to the log catcher (llm.jsonl).
func (c *Client) logCall(req *ChatRequest, start time.Time, completionChars, toolCalls int, finish string, err error) {
        msgs, chars := promptStats(req.Messages)
        rec := logging.LLMCallRecord{
                TS:              start,
                Provider:        c.cfg.ProviderKind(),
                Model:           req.Model,
                PromptMsgs:      msgs,
                PromptChars:     chars,
                CompletionChars: completionChars,
                ToolCalls:       toolCalls,
                FinishReason:    finish,
                DurationMs:      time.Since(start).Milliseconds(),
        }
        if err != nil {
                rec.Error = err.Error()
        }
        logging.Default().LLMCall(rec)
}

func truncateStr(s string, n int) string {
        if len(s) <= n {
                return s
        }
        return s[:n] + "…"
}

// imageDataURL returns the data-URL form of an image file, cached by path
// and mtime (the agent loop rebuilds the request every iteration — the cache
// turns dozens of base64 encodings into one). Hard-capped at 8 entries.
func (c *Client) imageDataURL(path string) (string, error) {
        fi, err := os.Stat(path)
        if err != nil {
                return "", err
        }
        c.imgCacheMu.Lock()
        if e, ok := c.imgCache[path]; ok && e.modTime.Equal(fi.ModTime()) {
                c.imgCacheMu.Unlock()
                return e.url, nil
        }
        c.imgCacheMu.Unlock()

        url, err := vision.EncodeImage(path)
        if err != nil {
                return "", err
        }
        c.imgCacheMu.Lock()
        if c.imgCache == nil {
                c.imgCache = map[string]imageCacheEntry{}
        }
        if len(c.imgCache) >= 8 {
                c.imgCache = map[string]imageCacheEntry{} // bounded: one turn's worth
        }
        c.imgCache[path] = imageCacheEntry{modTime: fi.ModTime(), url: url}
        c.imgCacheMu.Unlock()
        return url, nil
}

// wireMessages projects messages into the multimodal wire form (v1.0.6).
//
// Images are LIVE (real image_url parts) only for the current turn's tail —
// the last user message and everything after it (tool results of this turn,
// e.g. screenshots the agent just took). Older images degrade to a text note:
// re-encoding every historical image on every iteration would stall the loop
// for minutes and most context windows cannot hold them anyway.
//
// Tool-role images are LOCAL-only: llama.cpp accepts image parts on tool
// messages, but the OpenAI wire spec (and most hosted endpoints) require
// string content there — remote providers get the degrading note instead.
func (c *Client) wireMessages(msgs []Message) []wireMessage {
        lastUser := -1
        for i := len(msgs) - 1; i >= 0; i-- {
                if msgs[i].Role == "user" {
                        lastUser = i
                        break
                }
        }
        remote := c.cfg.IsRemote()
        out := make([]wireMessage, 0, len(msgs))
        for i, m := range msgs {
                wm := wireMessage{
                        Role:       m.Role,
                        ToolCalls:  m.ToolCalls,
                        ToolCallID: m.ToolCallID,
                        Name:       m.Name,
                }
                if len(m.Images) == 0 {
                        if m.Content != "" {
                                wm.Content = m.Content
                        }
                        out = append(out, wm)
                        continue
                }
                live := lastUser >= 0 && i >= lastUser
                if m.Role == "tool" && remote {
                        live = false
                }
                if !live {
                        var names []string
                        for _, p := range m.Images {
                                names = append(names, filepath.Base(p))
                        }
                        note := "[image attached earlier: " + strings.Join(names, ", ") + "]"
                        content := m.Content
                        if content == "" {
                                content = note
                        } else {
                                content += "\n" + note
                        }
                        wm.Content = content
                        out = append(out, wm)
                        continue
                }
                // Live multimodal content: text part + up to MaxImagesPerMessage
                // image parts.
                var parts []contentPart
                if m.Content != "" {
                        parts = append(parts, contentPart{Type: "text", Text: m.Content})
                }
                appended := 0
                for _, p := range m.Images {
                        if appended >= vision.MaxImagesPerMessage {
                                break
                        }
                        url, err := c.imageDataURL(p)
                        if err != nil {
                                parts = append(parts, contentPart{Type: "text",
                                        Text: fmt.Sprintf("[image %s could not be loaded: %v]", filepath.Base(p), err)})
                                continue
                        }
                        parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: url}})
                        appended++
                }
                wm.Content = parts
                out = append(out, wm)
        }
        return out
}

// ImageCacheLenForTest exposes the encoded-image cache size (v1.0.6 stress
// seam: proves the cache is bounded and actually hit).
func (c *Client) ImageCacheLenForTest() int {
        c.imgCacheMu.Lock()
        defer c.imgCacheMu.Unlock()
        return len(c.imgCache)
}

// BuildChatRequest converts raw messages + sampling options to a ChatRequest.
// Provider-aware: llama-only knobs (top_k, n_ctx) are sent to local
// llama.cpp but omitted for remote endpoints, which may reject them.
// Reasoning/attachment display fields are stripped from the wire copy;
// v1.0.6: images are projected into OpenAI content parts.
func (c *Client) BuildChatRequest(model string, messages []Message, tools []ToolSpec) *ChatRequest {
        clean := StripReasoning(messages)
        req := &ChatRequest{
                Model:       model,
                Messages:    clean,
                wire:        c.wireMessages(clean),
                Tools:       tools,
                Temperature: c.cfg.LLM.Temperature,
                TopP:        c.cfg.LLM.TopP,
                MaxTokens:   c.cfg.LLM.MaxTokens,
                Seed:        c.cfg.LLM.Seed,
        }
        if !c.cfg.IsRemote() {
                req.TopK = c.cfg.LLM.TopK
                req.NumCtx = c.cfg.LLM.NumCtx
        }
        if c.cfg.LLM.Stop != "" {
                req.Stop = strings.Split(c.cfg.LLM.Stop, ",")
        }
        return req
}
