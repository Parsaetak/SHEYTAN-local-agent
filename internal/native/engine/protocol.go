package engine

// protocol.go — the SHEYTAN Native API wire protocol (v1.1.5Z Phase 1).
//
// Frame layout (both directions, binary-safe):
//
//      [4 bytes little-endian payload length][length bytes of UTF-8 JSON]
//
// Request:
//
//      {"id": 1, "op": "health"}
//
// Response:
//
//      {"id": 1, "ok": true, "result": { ... }}
//      {"id": 1, "ok": false, "error": "human-readable reason"}
//
// Operations (coarse-grained by design — no tiny high-frequency calls
// cross this boundary):
//
//      ping     handshake: protocol + ABI version negotiation
//      health   active engine health probe
//      hwinfo   hardware capability profile (detected values)
//      metrics  engine metrics snapshot (measured values)
//      cancel   request cooperative cancellation of one in-flight generation
//      shutdown graceful engine shutdown
//
// Unknown ops and malformed frames produce a bounded error response (or
// are rejected as a protocol violation on the Go side); they NEVER crash
// either side. Frame size is capped so a hostile or buggy peer cannot
// exhaust memory.

import (
        "encoding/binary"
        "encoding/json"
        "errors"
        "fmt"
        "io"
)

// ProtocolVersion is the wire protocol version implemented here. The C++
// host reports its own value in the ping result; a mismatch is a hard
// handshake failure (fail closed).
const ProtocolVersion = 1

// MaxFrameBytes bounds one protocol frame (1 MiB). Anything larger is a
// protocol violation, not a buffer to allocate.
const MaxFrameBytes = 1 << 20

// Op names crossing the boundary.
const (
        OpPing     = "ping"
        OpHealth   = "health"
        OpHardware = "hwinfo"
        OpMetrics  = "metrics"
        OpCancel   = "cancel"
        OpShutdown = "shutdown"
)

// ValidOps is the closed set of accepted operations (validation + tests).
var ValidOps = map[string]bool{
        OpPing:     true,
        OpHealth:   true,
        OpHardware: true,
        OpMetrics:  true,
        OpCancel:   true,
        OpShutdown: true,
}

// ErrFrameTooLarge reports a frame exceeding MaxFrameBytes.
var ErrFrameTooLarge = errors.New("native engine protocol: frame exceeds 1 MiB cap")

// ErrFrameClosed reports the peer closed the stream.
var ErrFrameClosed = errors.New("native engine protocol: stream closed")

// ErrFrameTruncated reports a short read/write mid-frame.
var ErrFrameTruncated = errors.New("native engine protocol: truncated frame")

// Request is one outbound operation.
type Request struct {
        ID      int64           `json:"id"`
        Op      string          `json:"op"`
        Payload json.RawMessage `json:"payload,omitempty"`
}

// Response is one inbound result.
type Response struct {
        ID     int64           `json:"id"`
        OK     bool            `json:"ok"`
        Result json.RawMessage `json:"result,omitempty"`
        Error  string          `json:"error,omitempty"`
}

// PingResult is the handshake answer.
type PingResult struct {
        ProtocolVersion int    `json:"protocolVersion"`
        ABIVersion       uint32 `json:"abiVersion"`
        Engine           string `json:"engine"`
}

// CancelPayload identifies the request to cancel.
type CancelPayload struct {
        RequestID string `json:"requestId"`
}

// CancelResult reports what the engine did with a cancel request.
type CancelResult struct {
        Cancelled bool   `json:"cancelled"`
        Reason    string `json:"reason,omitempty"`
}

// WriteFrame writes one length-prefixed JSON payload.
func WriteFrame(w io.Writer, payload []byte) error {
        if len(payload) > MaxFrameBytes {
                return ErrFrameTooLarge
        }

        var header [4]byte
        binary.LittleEndian.PutUint32(header[:], uint32(len(payload)))

        if _, err := w.Write(header[:]); err != nil {
                return fmt.Errorf("write frame header: %w", err)
        }

        if _, err := w.Write(payload); err != nil {
                return fmt.Errorf("write frame body: %w", err)
        }

        return nil
}

// ReadFrame reads one length-prefixed payload, enforcing the size cap and
// treating EOF as stream closure.
func ReadFrame(r io.Reader) ([]byte, error) {
        var header [4]byte

        n, err := io.ReadFull(r, header[:])
        if err != nil {
                if n == 0 && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe)) {
                        return nil, ErrFrameClosed
                }
                return nil, fmt.Errorf("read frame header: %w", err)
        }

        size := binary.LittleEndian.Uint32(header[:])

        if size == 0 {
                return nil, fmt.Errorf("native engine protocol: zero-length frame")
        }

        if size > MaxFrameBytes {
                return nil, ErrFrameTooLarge
        }

        buf := make([]byte, size)

        if _, err := io.ReadFull(r, buf); err != nil {
                return nil, ErrFrameTruncated
        }

        return buf, nil
}

// EncodeRequest marshals and frames a request.
func EncodeRequest(w io.Writer, req *Request) error {
        payload, err := json.Marshal(req)
        if err != nil {
                return fmt.Errorf("marshal request: %w", err)
        }

        return WriteFrame(w, payload)
}

// DecodeRequest parses and validates an inbound request (used by the fake
// host in tests; the C++ host performs the equivalent validation natively).
func DecodeRequest(payload []byte) (*Request, error) {
        var req Request

        if err := json.Unmarshal(payload, &req); err != nil {
                return nil, fmt.Errorf("malformed request: %w", err)
        }

        if req.Op == "" {
                return nil, fmt.Errorf("malformed request: missing op")
        }

        if !ValidOps[req.Op] {
                return nil, fmt.Errorf("unknown op %q", req.Op)
        }

        return &req, nil
}

// DecodeResponse parses an inbound response. Error responses to
// unparseable requests may carry id 0; request/response matching is the
// caller's job (the pending map never has id 0 registered).
func DecodeResponse(payload []byte) (*Response, error) {
        var resp Response

        if err := json.Unmarshal(payload, &resp); err != nil {
                return nil, fmt.Errorf("malformed response: %w", err)
        }

        return &resp, nil
}

// EncodeResponse frames a response.
func EncodeResponse(w io.Writer, resp *Response) error {
        payload, err := json.Marshal(resp)
        if err != nil {
                return fmt.Errorf("marshal response: %w", err)
        }

        return WriteFrame(w, payload)
}

// DecodeResult unmarshals a successful response's result into out.
func DecodeResult(resp *Response, out any) error {
        if resp == nil {
                return fmt.Errorf("nil response")
        }

        if !resp.OK {
                return fmt.Errorf("native engine error: %s", resp.Error)
        }

        if len(resp.Result) == 0 {
                return fmt.Errorf("native engine protocol: ok response without result")
        }

        if err := json.Unmarshal(resp.Result, out); err != nil {
                return fmt.Errorf("malformed result payload: %w", err)
        }

        return nil
}
