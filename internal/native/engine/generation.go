package engine

// generation.go — native engine generation concern (v1.1.5Z Phase 1:
// TYPES ONLY — NO NATIVE INFERENCE EXISTS YET).
//
// The future native engine will serve generation through the same
// coarse-grained boundary (a whole request streams whole chunks; the
// boundary never carries per-token chatter). Phase 1 defines the request/
// stats data model reported through the metrics op. Backend.Generate and
// Backend.StreamGenerate return llm.ErrNotImplemented, which is the
// fallback signal — do not "fix" that by faking inference.

// GenerationRequest is the future native generation request (wire shape
// reserved; not accepted by the host yet).
type GenerationRequest struct {
	// RequestID identifies the request for cancel/status round-trips.
	RequestID string `json:"requestId,omitempty"`

	// Model the request targets.
	Model string `json:"model,omitempty"`

	// PromptTokens / MaxTokens bound the generation.
	PromptTokens int `json:"promptTokens,omitempty"`
	MaxTokens    int `json:"maxTokens,omitempty"`

	// Stream selects chunked delivery.
	Stream bool `json:"stream,omitempty"`
}

// GenerationChunk is one future streamed chunk.
type GenerationChunk struct {
	RequestID string `json:"requestId,omitempty"`
	Text      string `json:"text,omitempty"`
	// Done marks the final chunk.
	Done bool `json:"done,omitempty"`
}

// GenerationStats is the generation snapshot reported through metrics.
// All timing fields are zero until the engine actually serves generation.
type GenerationStats struct {
	// ActiveRequests: in-flight generation requests (measured; zero in
	// Phase 1).
	ActiveRequests int `json:"activeRequests,omitempty"`

	// TTFTSeconds: time-to-first-token of the most recent completed
	// request (measured; absent until generation exists).
	TTFTSeconds float64 `json:"ttftSeconds,omitempty"`

	// PromptTokensPerSecond: prompt processing speed of the most recent
	// completed request (measured; absent until generation exists).
	PromptTokensPerSecond float64 `json:"promptTokensPerSecond,omitempty"`

	// DecodeTokensPerSecond: decode speed of the most recent completed
	// request (measured; absent until generation exists).
	DecodeTokensPerSecond float64 `json:"decodeTokensPerSecond,omitempty"`
}
