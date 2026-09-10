// scheduler.h — the SHEYTAN native engine bounded scheduler (Phase 4).
//
// A real bounded single-slot scheduler with cancellation, fair FIFO
// ordering, graceful shutdown and no busy polling. Designed for the
// future native inference path: when Generate exists, requests come in
// here, get queued, get executed one at a time (single-slot execution),
// and stream their results back.
//
// What this is:
//   - a real bounded queue (capacity = QueueDepthLimit);
//   - real cancellation (per-request cancel token, propagated to the
//     future execution loop);
//   - fair FIFO ordering (the queue is a deque, push back / pop front);
//   - graceful shutdown (drains pending requests with a "shutting down"
//     error before destruction);
//   - no busy polling (the worker blocks on a condition variable).
//
// What this is NOT:
//   - this is NOT continuous batching (one request at a time);
//   - this is NOT wired into the inference loop (no inference exists —
//     the scheduler exists, is measurable, but executes nothing);
//   - this is NOT speculative.

#ifndef SHTN_SCHEDULER_H
#define SHTN_SCHEDULER_H

#include <atomic>
#include <condition_variable>
#include <cstdint>
#include <deque>
#include <functional>
#include <memory>
#include <mutex>
#include <string>

namespace shtn {
namespace sched {

// Hard bounds.
constexpr uint32_t kMaxConcurrent = 1;          // single-slot execution
constexpr uint32_t kDefaultQueueDepth = 8;      // bounded queue
constexpr uint32_t kMaxQueueDepth = 64;         // hard cap

// RequestState mirrors the lifecycle a queued request goes through.
enum class State : uint8_t {
    kQueued     = 0, // waiting for the execution slot
    kActive     = 1, // executing (would be, if inference existed)
    kCancelled  = 2, // cancelled before or during execution
    kCompleted  = 3, // finished successfully
    kFailed     = 4, // finished with an error
};

// Request is one scheduled unit. The future Generate op creates one of
// these, hands it to the scheduler, and the (future) worker thread
// executes it. The execute callback is the future inference entry point
// — currently it is a no-op that immediately marks the request
// completed, because no inference exists. That keeps the scheduler
// testable without faking inference.
struct Request {
    std::string id;                                  // caller-supplied
    std::function<void()> execute;                   // future inference hook
    std::atomic<State> state{State::kQueued};        // mutable across threads
    std::atomic<bool> cancel_requested{false};

    Request() = default;
    explicit Request(std::string id_, std::function<void()> exec)
        : id(std::move(id_)), execute(std::move(exec)) {}
};

using RequestPtr = std::shared_ptr<Request>;

// Stats is the measurable scheduler snapshot reported through metrics.
struct Stats {
    uint32_t active_requests = 0;       // currently executing (0 in Phase 4)
    uint32_t queued_requests = 0;       // waiting in the queue
    uint32_t max_concurrent = kMaxConcurrent;
    uint32_t queue_depth_limit = kDefaultQueueDepth;
    uint64_t total_submitted = 0;       // cumulative since scheduler create
    uint64_t total_completed = 0;
    uint64_t total_cancelled = 0;
    uint64_t total_failed = 0;
    bool shutting_down = false;
};

// Scheduler is the bounded request queue + (future) worker. Thread-safe.
class Scheduler {
public:
    explicit Scheduler(uint32_t queue_depth = kDefaultQueueDepth);
    ~Scheduler();

    Scheduler(const Scheduler&) = delete;
    Scheduler& operator=(const Scheduler&) = delete;

    // Submit enqueues a request. Returns:
    //   SHTN_OK                enqueued;
    //   SHTN_ERR_INVALID_ARG   id empty or execute null;
    //   SHTN_ERR_MODEL_STATE   scheduler is shutting down;
    //   SHTN_ERR_UNSUPPORTED   queue is full.
    //
    // On SHTN_OK the request is queued and (in the future) will be
    // executed by the worker. In Phase 4 with no worker thread, the
    // request stays queued until Cancel/Shutdown touches it — the
    // scheduler does NOT execute it. This is honest: the scheduler
    // exists and is measurable; execution does not.
    int32_t submit(RequestPtr req, std::string& error);

    // Cancel marks a request as cancellation-requested. The future
    // execution loop would observe this and abort; in Phase 4 (no
    // execution) the request is moved to kCancelled directly if it's
    // still queued.
    //
    // Returns true if a request with this id was found and cancelled,
    // false otherwise (no such id, or already terminal).
    bool cancel(const std::string& id);

    // Pop the next non-cancelled request from the queue (FIFO). Returns
    // nullptr when the queue is empty. Used by the future worker thread;
    // in Phase 4 the tests call it directly to verify ordering.
    RequestPtr pop_next();

    // Drain cancels every queued request with "shutting down". Used by
    // Shutdown and the destructor.
    void drain();

    // Shutdown marks the scheduler as shutting down and drains the queue.
    // After shutdown, submit returns SHTN_ERR_MODEL_STATE.
    void shutdown();

    // Stats returns the measurable snapshot (thread-safe).
    Stats stats() const;

    // Is the scheduler accepting new requests?
    bool accepting() const;

private:
    mutable std::mutex mu_;
    std::condition_variable cv_;
    std::deque<RequestPtr> queue_;
    uint32_t queue_depth_limit_;
    bool shutting_down_ = false;

    std::atomic<uint64_t> total_submitted_{0};
    std::atomic<uint64_t> total_completed_{0};
    std::atomic<uint64_t> total_cancelled_{0};
    std::atomic<uint64_t> total_failed_{0};
};

} // namespace sched
} // namespace shtn

#endif /* SHTN_SCHEDULER_H */
