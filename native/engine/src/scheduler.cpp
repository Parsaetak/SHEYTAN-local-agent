// scheduler.cpp — native engine bounded scheduler implementation (Phase 4).

#include "scheduler.h"

#include "shtn/engine.h"

namespace shtn {
namespace sched {

Scheduler::Scheduler(uint32_t queue_depth)
    : queue_depth_limit_(queue_depth == 0 ? kDefaultQueueDepth
                       : queue_depth > kMaxQueueDepth ? kMaxQueueDepth
                                                      : queue_depth) {}

Scheduler::~Scheduler() {
    shutdown();
}

int32_t Scheduler::submit(RequestPtr req, std::string& error) {
    if (!req) {
        error = "scheduler: null request";
        return SHTN_ERR_INVALID_ARG;
    }
    if (req->id.empty()) {
        error = "scheduler: empty request id";
        return SHTN_ERR_INVALID_ARG;
    }
    if (!req->execute) {
        // Phase 4: a null execute is accepted — it represents "queue this
        // but the inference path does not exist yet". The scheduler
        // still tracks it. This keeps the scheduler testable without
        // faking inference.
    }

    std::lock_guard<std::mutex> lock(mu_);

    if (shutting_down_) {
        error = "scheduler: shutting down";
        return SHTN_ERR_MODEL_STATE;
    }

    if (queue_.size() >= queue_depth_limit_) {
        error = "scheduler: queue is full (" +
                std::to_string(queue_depth_limit_) + ")";
        return SHTN_ERR_UNSUPPORTED;
    }

    queue_.push_back(req);
    total_submitted_.fetch_add(1, std::memory_order_relaxed);
    cv_.notify_one();

    return SHTN_OK;
}

bool Scheduler::cancel(const std::string& id) {
    if (id.empty()) {
        return false;
    }

    std::lock_guard<std::mutex> lock(mu_);

    for (auto it = queue_.begin(); it != queue_.end(); ++it) {
        RequestPtr& r = *it;
        if (r && r->id == id) {
            // Still queued: remove and mark cancelled.
            State expected = State::kQueued;
            if (r->state.compare_exchange_strong(expected, State::kCancelled)) {
                queue_.erase(it);
                total_cancelled_.fetch_add(1, std::memory_order_relaxed);
                return true;
            }
            // Already terminal — leave it; return false.
            return false;
        }
    }

    // Not in the queue: it might be "active" (executing). Mark
    // cancel_requested so the future execution loop observes it. We
    // cannot remove an active request — the future loop must abort.
    // (In Phase 4 nothing is active, so this is a no-op.)
    return false;
}

RequestPtr Scheduler::pop_next() {
    std::lock_guard<std::mutex> lock(mu_);

    while (!queue_.empty()) {
        RequestPtr r = queue_.front();
        queue_.pop_front();

        if (!r) {
            continue;
        }

        // Skip already-cancelled requests.
        State s = r->state.load(std::memory_order_relaxed);
        if (s == State::kCancelled) {
            continue;
        }

        // Mark active (the future execution loop would observe this).
        State expected = State::kQueued;
        if (r->state.compare_exchange_strong(expected, State::kActive)) {
            return r;
        }
    }

    return nullptr;
}

void Scheduler::drain() {
    std::lock_guard<std::mutex> lock(mu_);

    while (!queue_.empty()) {
        RequestPtr r = queue_.front();
        queue_.pop_front();
        if (r) {
            State expected = State::kQueued;
            if (r->state.compare_exchange_strong(expected, State::kCancelled)) {
                total_cancelled_.fetch_add(1, std::memory_order_relaxed);
            }
        }
    }
}

void Scheduler::shutdown() {
    {
        std::lock_guard<std::mutex> lock(mu_);
        shutting_down_ = true;
    }
    drain();
    cv_.notify_all();
}

Stats Scheduler::stats() const {
    std::lock_guard<std::mutex> lock(mu_);

    Stats s;
    s.active_requests = 0; // Phase 4: no worker thread
    s.queued_requests = static_cast<uint32_t>(queue_.size());
    s.max_concurrent = kMaxConcurrent;
    s.queue_depth_limit = queue_depth_limit_;
    s.total_submitted = total_submitted_.load(std::memory_order_relaxed);
    s.total_completed = total_completed_.load(std::memory_order_relaxed);
    s.total_cancelled = total_cancelled_.load(std::memory_order_relaxed);
    s.total_failed = total_failed_.load(std::memory_order_relaxed);
    s.shutting_down = shutting_down_;

    return s;
}

bool Scheduler::accepting() const {
    std::lock_guard<std::mutex> lock(mu_);
    return !shutting_down_;
}

} // namespace sched
} // namespace shtn
