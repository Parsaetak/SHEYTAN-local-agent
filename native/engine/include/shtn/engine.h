// engine.h — the SHEYTAN Native Engine C ABI (v1.1.5Z Phase 2).
//
// This is the NARROW boundary the Go core sees. Only C types, only
// coarse operations — no C++ classes, templates or exceptions cross this
// line. The Go core does NOT link this ABI directly; it supervises the
// shtn-engine-host subprocess which wraps exactly these functions behind
// the length-prefixed JSON IPC protocol (see doc.go in
// internal/native/engine for the boundary rationale).
//
// Phase 1 surface (implemented, unchanged):
//   shtn_engine_create / shtn_engine_destroy   engine lifecycle
//   shtn_engine_health                          active health probe
//   shtn_engine_hardware_info                   detected hardware profile
//   shtn_engine_metrics                         measured metrics
//   shtn_abi_version                            ABI negotiation
//
// Phase 2 surface (implemented — native GGUF model loading):
//   shtn_engine_load_model                      validate + map + plan
//   shtn_engine_unload_model                    release everything
//   shtn_engine_model_info                      real metadata snapshot
//   shtn_engine_memory_plan                     load-time budget
//
// NOT implemented (later phases — the honest answer is an error code):
//   inference / token generation / GPU kernels / KV-cache allocation.
//
// Conventions:
//   - every call returns int32_t: 0 = success, negative = error code;
//   - structs are filled only with values this build can actually
//     detect, measure, read or derive;
//   - all calls are safe under concurrency per engine instance (internal
//     mutex); NULL arguments are rejected, never dereferenced.

#ifndef SHTN_ENGINE_H
#define SHTN_ENGINE_H

#include "types.h"
#include "version.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Error codes (stable ABI values; Phase 2 additions are appended —
 * existing values are never renumbered). */
enum {
    SHTN_OK = 0,
    SHTN_ERR_INVALID_ARG = -1,   /* NULL or otherwise invalid argument */
    SHTN_ERR_ABI = -2,           /* caller ABI does not match engine ABI */
    SHTN_ERR_UNSUPPORTED = -3,   /* operation reserved for a later phase */
    SHTN_ERR_INTERNAL = -4,      /* unexpected internal failure */
    SHTN_ERR_MODEL_FORMAT = -5,  /* malformed / unsupported GGUF file */
    SHTN_ERR_MODEL_STATE = -6,   /* model state rejects the operation */
    SHTN_ERR_NO_MODEL = -7       /* operation requires a loaded model */
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
 * engine reports healthy/ready. Destroy the handle with
 * shtn_engine_destroy. */
int32_t shtn_engine_create(const shtn_engine_options* opts, shtn_engine** out);

/* Destroy an engine instance. NULL is accepted and ignored. Any loaded
 * model (memory mapping, file handle) is released. */
void shtn_engine_destroy(shtn_engine* engine);

/* Fill out with the engine's current health. A created engine is healthy
 * until destroyed (a FAILED model load does not make the engine
 * unhealthy — the failure is reported on the model surface). The detail
 * string summarizes the model concern. */
int32_t shtn_engine_health(const shtn_engine* engine, shtn_health_status* out);

/* Fill out with the detected hardware profile of this machine (as seen
 * by the native binary). Only detectable values are filled; see
 * types.h. */
int32_t shtn_engine_hardware_info(shtn_engine* engine, shtn_hardware_info* out);

/* Fill out with measured engine metrics (state, uptime, own process RSS,
 * active request count). */
int32_t shtn_engine_metrics(shtn_engine* engine, shtn_metrics* out);

/* Load the GGUF model at path: validate the file (magic, version,
 * metadata, tensor table, bounds), map it read-only (memory-mapped, lazy
 * page-in — no eager copy of the tensor data), derive the metadata
 * snapshot and compute the memory plan. Replaces any previously loaded
 * model (replace semantics: unload, then load — all-or-nothing; a failed
 * load leaves NO model loaded and every resource released). Concurrent
 * load/unload calls are serialized; a load while another load is in
 * flight fails with SHTN_ERR_MODEL_STATE. */
int32_t shtn_engine_load_model(shtn_engine* engine, const char* path,
                               const shtn_model_load_options* opts);

/* Unload the current model and release all of its resources (mapping,
 * file handle, cached metadata). Idempotent: unloading an engine with no
 * model succeeds and leaves the state "unloaded". */
int32_t shtn_engine_unload_model(shtn_engine* engine);

/* Fill out with the model concern snapshot (state, metadata, plan
 * summary). Always succeeds for a valid engine; with no model attempted
 * the state is "unloaded" and every metadata field stays 0/empty. */
int32_t shtn_engine_model_info(const shtn_engine* engine, shtn_model_info* out);

/* Fill out with the memory plan of the CURRENT model (a failed or
 * finished load keeps the last computed plan for inspection; before any
 * load attempt every field is 0). Does not allocate. */
int32_t shtn_engine_memory_plan(const shtn_engine* engine, shtn_memory_plan* out);

/* Report the ABI version this engine was built with. */
uint32_t shtn_abi_version(void);

#ifdef __cplusplus
} /* extern "C" */
#endif

#endif /* SHTN_ENGINE_H */
