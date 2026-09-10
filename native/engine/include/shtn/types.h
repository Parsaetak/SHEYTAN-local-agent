// types.h — plain C data structures crossing the engine boundary.
//
// RULES (see ARCHITECTURE.md Part III):
//   - every value in these structs is DETECTED, MEASURED, READ or
//     DERIVED; unknowns are zero/empty, never guessed;
//   - no vendor assumptions are hardcoded anywhere;
//   - fixed-size buffers keep the ABI stable across compilers; strings
//     are always NUL-terminated.
//
// Phase 2 additions (ABI v2): shtn_model_load_options, shtn_model_info
// and shtn_memory_plan — the model-loading surface. Phase 1 structs are
// UNCHANGED.

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
 * can represent accelerators; detection fills none (count 0) until a
 * real probe is implemented. */
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
    int32_t gpu_count;                 /* 0 (no C++ GPU probe) */
    shtn_accelerator_info accelerators[4];
    int32_t accelerator_count;         /* 0 */
    char detected_by[256];             /* semicolon-joined probe sources */
} shtn_hardware_info;

typedef struct shtn_metrics {
    char state[32];
    double uptime_seconds;     /* measured since engine create */
    uint64_t process_rss_bytes;/* measured engine process RSS */
    int32_t active_requests;   /* measured: 0 (no generation yet) */
} shtn_metrics;

/* --- Phase 2: model loading surface -------------------------------------- */

/* Model lifecycle states (reported by shtn_model_info.state). These are
 * MODEL states — deliberately a small, dedicated vocabulary; the ENGINE
 * states (idle/starting/ready/...) keep living in the shared llm.State*
 * set and are never mixed with these. */
#define SHTN_MODEL_STATE_UNLOADED "unloaded"
#define SHTN_MODEL_STATE_LOADING  "loading"
#define SHTN_MODEL_STATE_LOADED   "loaded"
#define SHTN_MODEL_STATE_FAILED   "failed"

/* shtn_model_load_options configures one load attempt.
 * context_length: 0 = plan with the model's own trained context length;
 * a non-zero value overrides it for the KV-cache / workspace estimates
 * (planning only — Phase 2 allocates no context buffers). */
typedef struct shtn_model_load_options {
    uint32_t context_length; /* 0 = use the model's trained context */
    uint32_t reserved;       /* must be 0 */
} shtn_model_load_options;

/* shtn_model_info is a snapshot of the engine's model concern. Fields
 * are filled ONLY with values actually read from the GGUF header or
 * derived from the tensor table; unknowns stay 0/empty. `state` uses the
 * SHTN_MODEL_STATE_* vocabulary; `error` carries the last load failure
 * detail (empty when the model did not fail). */
typedef struct shtn_model_info {
    char path[1024];
    char architecture[64];    /* general.architecture */
    char name[256];           /* general.name */
    char quantization[64];    /* human name for general.file_type */
    char state[16];           /* SHTN_MODEL_STATE_* */
    char error[256];          /* last load failure detail; empty otherwise */
    uint64_t file_size_bytes;      /* measured on-disk size */
    uint64_t parameter_count;      /* derived from tensor table, or read */
    uint64_t context_length;       /* <arch>.context_length */
    uint64_t vocabulary_size;      /* <arch>.vocab_size / tokenizer array */
    uint64_t embedding_length;     /* <arch>.embedding_length */
    uint64_t layer_count;          /* <arch>.block_count */
    uint64_t kv_cache_bytes;       /* planned estimate (f16 K+V) */
    uint64_t workspace_bytes;      /* planned estimate (logits row) */
    uint64_t total_plan_bytes;     /* full memory-plan total */
    uint32_t tensor_count;         /* tensor table entries */
    uint32_t gguf_version;         /* 2 or 3 */
    uint32_t general_file_type;    /* raw general.file_type (when present) */
    int32_t  has_file_type;        /* 1 = general.file_type was present */
} shtn_model_info;

/* shtn_memory_plan is the load-time budget the engine computes from the
 * GGUF header + tensor table. NOTHING is allocated to produce this plan
 * — every number is arithmetic on parsed metadata. Bytes semantics:
 *   model_file_bytes       measured on-disk size;
 *   mapped_bytes           bytes mapped read-only (== file size);
 *   weights_bytes          tensor-data span (file_size - data start);
 *   workspace_bytes        estimated temporary compute buffers;
 *   kv_cache_bytes         estimated f16 K+V cache at the planned context;
 *   runtime_overhead_bytes fixed engine bookkeeping allowance;
 *   total_bytes            overflow-checked sum of the above;
 *   available_ram_bytes    detected available RAM (0 = unknown);
 *   fits_in_ram            1 yes / 0 no / -1 unknown. */
typedef struct shtn_memory_plan {
    uint64_t model_file_bytes;
    uint64_t mapped_bytes;
    uint64_t weights_bytes;
    uint64_t workspace_bytes;
    uint64_t kv_cache_bytes;
    uint64_t runtime_overhead_bytes;
    uint64_t total_bytes;
    uint64_t available_ram_bytes;
    int32_t  fits_in_ram;
    int32_t  reserved;       /* always 0 */
} shtn_memory_plan;

#ifdef __cplusplus
} /* extern "C" */
#endif

#endif /* SHTN_TYPES_H */
