package api

import "time"

// Close stops active agent runs and releases the resources owned by the
// shared runtime stack.
//
// The API server is the owner of the Stack instance created by New(), so
// shutting down the API must also shut down the runtime resources it owns.
func (s *Server) Close() {
	if s == nil {
		return
	}

	// Cancel every active HTTP-triggered run first.
	//
	// Do not hold runsMu while calling into the runtime; cancellation can
	// cause callbacks/goroutines to touch the run registry.
	s.runsMu.Lock()
	cancels := make([]func(), 0, len(s.runs))

	for _, rs := range s.runs {
		if rs != nil && rs.cancel != nil {
			cancels = append(cancels, rs.cancel)
		}
	}

	s.runsMu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}

	// The orchestrator itself needs no explicit abort: every run it
	// executes is bound to the request context canceled above, so
	// canceling the registered runs stops all orchestrator work.

	// Stop the engine event fan-out before tearing down the engine it
	// subscribes to.
	if s.engineStop != nil {
		select {
		case <-s.engineStop:
		default:
			close(s.engineStop)
		}
	}

	if s.engineDone != nil {
		select {
		case <-s.engineDone:
		case <-time.After(2 * time.Second):
		}
	}

	// Release the shared runtime resources.
	if s.stack != nil {
		s.stack.Close()
	}
}
