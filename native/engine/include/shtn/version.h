// version.h — SHEYTAN Native Engine ABI and protocol versioning.
//
// The Go core (internal/native/engine) checks BOTH values in the
// handshake: a mismatch is a hard startup failure (fail closed) so a
// stale engine build can never serve a newer Go core, or vice versa.
//
// Bump SHTN_ABI_VERSION whenever the C ABI (engine.h / types.h) changes
// shape. Bump SHTN_PROTOCOL_VERSION whenever the wire protocol
// (framing / ops / payload shapes) changes.
//
// Phase 1 (v1.1.5Z): both were 1.
// Phase 2 (v1.1.5Z): both are 2 — added the model surface
// (load_model / unload_model / model_info) to the wire protocol and
// shtn_engine_load_model / shtn_engine_unload_model /
// shtn_engine_model_info / shtn_engine_memory_plan to the C ABI.
// Pre-existing ops and structs kept their shapes (additive change).

#ifndef SHTN_VERSION_H
#define SHTN_VERSION_H

#define SHTN_ABI_VERSION 2u
#define SHTN_PROTOCOL_VERSION 2

/* Engine identity reported by the ping op. */
#define SHTN_ENGINE_NAME "shtn-native-engine"

#endif /* SHTN_VERSION_H */
