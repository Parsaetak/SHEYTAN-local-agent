// types.h — plain C data structures crossing the engine boundary.
//
// RULES (see ARCHITECTURE.md Part III):
//   - every value in these structs is DETECTED or MEASURED; unknowns are
//     zero/empty, never guessed;
//   - no vendor assumptions are hardcoded anywhere;
//   - fixed-size buffers keep the ABI stable across compilers; strings
//     are always NUL-terminated.

#ifndef SHTN_TYPES_H
#define SHTN_TYPES_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct shtn_health_status {
    int32_t healthy;        /* 0 or 1 */
    char state[32];         /* same vocabulary as the Go engine states */
    char detail[256];       /* human-readable note; empty when healthy */
} shtn_health_status;

typedef struct shtn_cpu_info {
    char name[128];         /* empty when not detected */
    int32_t physical_cores; /* 0 when not detected */
    int32_t logical_cores;  /* 0 when not detected */
    int64_t frequency_hz;   /* 0 when not detected */
} shtn_cpu_info;

typedef struct shtn_ram_info {
    uint64_t total_bytes;      /* 0 when not detected */
    uint64_t available_bytes;  /* 0 when not detected */
} shtn_ram_info;

typedef struct shtn_gpu_info {
    char vendor[64];
    char name[128];
    uint64_t vram_bytes;     /* 0 when not detected */
    int32_t shared_memory;   /* 1 = shared/unified memory; 0 when unknown */
    char driver_version[64];
} shtn_gpu_info;

/* NPU / AI accelerator device. The struct EXISTS so the hardware profile
 * can represent accelerators; Phase 1 detection fills none (count 0)
 * until a real probe is implemented. */
typedef struct shtn_accelerator_info {
    char vendor[64];
    char name[128];
    char kind[32];           /* "npu", "tpu", "dsp", "other" */
    uint64_t memory_bytes;
} shtn_accelerator_info;

typedef struct shtn_hardware_info {
    char architecture[16];   /* "x86_64", "aarch64", ... (compile-time) */
    shtn_cpu_info cpu;
    shtn_ram_info ram;
    shtn_gpu_info gpus[8];
    int32_t gpu_count;                 /* 0 in Phase 1 (no C++ GPU probe) */
    shtn_accelerator_info accelerators[4];
    int32_t accelerator_count;         /* 0 in Phase 1 */
    char detected_by[256];             /* semicolon-joined probe sources */
} shtn_hardware_info;

typedef struct shtn_metrics {
    char state[32];
    double uptime_seconds;     /* measured since engine create */
    uint64_t process_rss_bytes;/* measured engine process RSS */
    int32_t active_requests;   /* measured: 0 in Phase 1 (no generation) */
} shtn_metrics;

#ifdef __cplusplus
} /* extern "C" */
#endif

#endif /* SHTN_TYPES_H */
