// engine.h — the SHEYTAN Native Engine C ABI (v1.1.5Z Phase 1).
//
// This is the NARROW boundary the Go core sees. Only C types, only
// coarse operations — no C++ classes, templates or exceptions cross this
// line. The Go core does NOT link this ABI directly; it supervises the
// shtn-engine-host subprocess which wraps exactly these functions behind
// the length-prefixed JSON IPC protocol (see doc.go in
// internal/native/engine for the boundary rationale).
//
// Phase 1 surface (implemented):
//   shtn_engine_create / shtn_engine_destroy   engine lifecycle
//   shtn_engine_health                          active health probe
//   shtn_engine_hardware_info                   detected hardware profile
//   shtn_engine_metrics                         measured metrics
//   shtn_abi_version                            ABI negotiation
//
// Future phases will extend this surface (model load/unload, generate,
// stream, cancel) WITHOUT breaking the version negotiation.
//
// Conventions:
//   - every call returns int32_t: 0 = success, negative = error code;
//   - structs are filled only with values this build can actually
//     detect or measure;
//   - all calls are safe under concurrency per engine instance (internal
//     mutex); NULL arguments are rejected, never dereferenced.

#ifndef SHTN_ENGINE_H
#define SHTN_ENGINE_H

#include "types.h"
#include "version.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Error codes (stable ABI values). */
enum {
    SHTN_OK = 0,
    SHTN_ERR_INVALID_ARG = -1,   /* NULL or otherwise invalid argument */
    SHTN_ERR_ABI = -2,           /* caller ABI does not match engine ABI */
    SHTN_ERR_UNSUPPORTED = -3,   /* operation reserved for a later phase */
    SHTN_ERR_INTERNAL = -4       /* unexpected internal failure */
};

/* Opaque engine handle. */
typedef struct shtn_engine shtn_engine;

/* shtn_engine_options configures engine creation. The caller MUST set
 * abi_version = SHTN_ABI_VERSION; a mismatch fails with SHTN_ERR_ABI so
 * incompatible builds fail closed at creation, not mid-flight. */
typedef struct shtn_engine_options {
    uint32_t abi_version;
    uint32_t reserved; /* must be 0 */
} shtn_engine_options;

/* Create an engine instance. On success *out holds the handle and the
 * engine reports healthy/ready (Phase 1 has no model loading step).
 * Destroy the handle with shtn_engine_destroy. */
int32_t shtn_engine_create(const shtn_engine_options* opts, shtn_engine** out);

/* Destroy an engine instance. NULL is accepted and ignored. */
void shtn_engine_destroy(shtn_engine* engine);

/* Fill out with the engine's current health. A created engine is healthy
 * until destroyed; state uses the shared engine vocabulary ("ready" in
 * Phase 1). */
int32_t shtn_engine_health(const shtn_engine* engine, shtn_health_status* out);

/* Fill out with the detected hardware profile of this machine (as seen
 * by the native binary). Only detectable values are filled; see
 * types.h. */
int32_t shtn_engine_hardware_info(shtn_engine* engine, shtn_hardware_info* out);

/* Fill out with measured engine metrics (state, uptime, own process RSS,
 * active request count). */
int32_t shtn_engine_metrics(shtn_engine* engine, shtn_metrics* out);

/* Report the ABI version this engine was built with. */
uint32_t shtn_abi_version(void);

#ifdef __cplusplus
} /* extern "C" */
#endif

#endif /* SHTN_ENGINE_H */
