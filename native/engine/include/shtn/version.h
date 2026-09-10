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
// Phase 1 (v1.1.5Z): both are 1.

#ifndef SHTN_VERSION_H
#define SHTN_VERSION_H

#define SHTN_ABI_VERSION 1u
#define SHTN_PROTOCOL_VERSION 1

/* Engine identity reported by the ping op. */
#define SHTN_ENGINE_NAME "shtn-native-engine"

#endif /* SHTN_VERSION_H */
