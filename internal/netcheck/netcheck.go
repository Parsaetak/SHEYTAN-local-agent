// Package netcheck detects whether the machine has a working internet
// connection so the agent can degrade gracefully offline: web tools fail
// fast with a friendly message, remote LLM endpoints skip their retry
// ladder, and the LLM itself is told which tools are unavailable.
//
// v1.0.3 rewrite: the old single-strategy probe (a raw TCP dial to
// 1.1.1.1/8.8.8.8) reported OFFLINE forever on machines whose network only
// allows traffic through a proxy or filtered gateway — the UI pill never
// flipped back to ONLINE after reconnecting. The probe now tries THREE
// independent strategies and reports online as soon as ANY succeeds:
//
//  1. TCP dial to well-known anycast IPs (fast, no DNS)
//  2. HTTP HEAD to connectivity-check endpoints (Microsoft NCSI / Google
//     204 — flows through system/env proxies where configured)
//  3. DNS resolution of a public hostname through the OS resolver
//
// The result is cached with a short TTL so callers can ask IsOffline() on
// every tool invocation without measurable cost.
package netcheck

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// ttl is how long a probe result is trusted. Kept short (15s) so a
// reconnect is picked up quickly; the UI watcher also re-probes on its own
// cadence and calls Force() when it wants an immediate answer.
const ttl = 15 * time.Second

// probeTimeout bounds a single connectivity check.
const probeTimeout = 2500 * time.Millisecond

// targets are reliable anycast endpoints (Cloudflare + Google DNS). A TCP
// dial to either proves real outbound connectivity — no DNS involved, so a
// broken resolver does not produce false negatives.
var targets = []string{"1.1.1.1:443", "8.8.8.8:53", "1.0.0.1:443"}

// httpTargets are connectivity-check URLs (Windows NCSI + Google captive
// portal). They are fetched with HEAD through the default transport, which
// honors HTTP(S)_PROXY environment variables and the platform proxy where
// Go supports it — this is the strategy that fixes the stuck-OFFLINE bug on
// proxied machines.
var httpTargets = []string{
	"http://www.msftconnecttest.com/connecttest.txt",
	"https://www.gstatic.com/generate_204",
}

// dnsHosts are hostnames resolved through the OS resolver as the third
// strategy (works when raw IP dials are filtered but DNS + normal traffic
// flow).
var dnsHosts = []string{"github.com", "cloudflare.com"}

var (
	mu      sync.Mutex
	lastAt  time.Time
	online  bool
	known   bool // has any probe completed yet?
	probeFn func() bool
)

// SetProbe replaces the connectivity probe (tests only). The cached
// answer is dropped so the next Online() call uses the new probe.
func SetProbe(fn func() bool) {
	mu.Lock()
	defer mu.Unlock()
	probeFn = fn
	known = false
	lastAt = time.Time{}
}

// Force re-probes now and returns the fresh result.
func Force() bool {
	mu.Lock()
	probe := probeFn
	mu.Unlock()

	var ok bool
	if probe != nil {
		ok = probe()
	} else {
		ok = probeAll()
	}

	mu.Lock()
	online, known, lastAt = ok, true, time.Now()
	mu.Unlock()
	return ok
}

// IsOffline reports whether the machine is (very probably) offline.
// The first call probes; subsequent calls within the TTL reuse the cached
// answer.
func IsOffline() bool { return !Online() }

// Online reports whether the internet appears reachable (cached).
func Online() bool {
	mu.Lock()
	fresh := known && time.Since(lastAt) < ttl
	if fresh {
		ok := online
		mu.Unlock()
		return ok
	}
	probe := probeFn
	mu.Unlock()

	var ok bool
	if probe != nil {
		ok = probe()
	} else {
		ok = probeAll()
	}

	mu.Lock()
	online, known, lastAt = ok, true, time.Now()
	mu.Unlock()
	return ok
}

// Note returns a system-prompt environment note describing the current
// connectivity, or "" when online. The orchestrator prepends it so the LLM
// does not waste turns calling web tools that cannot work.
func Note() string {
	if Online() {
		return ""
	}
	return "ENVIRONMENT NOTE: the machine is currently OFFLINE — there is no internet connection. " +
		"The webSearch and browser tools cannot reach the web; do not call them for online information. " +
		"All local capabilities remain fully available: files, shell, codeExec, git, dataAnalysis (CSV/JSON + charts), and memory. " +
		"Answer from your own knowledge and local files, and tell the user when something would have required the web."
}

// State returns a human label for the UI: "online" | "offline".
func State() string {
	if Online() {
		return "online"
	}
	return "offline"
}

// probeAll runs every strategy until one reports online.
func probeAll() bool {
	if dialAny() {
		return true
	}
	if httpAny() {
		return true
	}
	return dnsAny()
}

// dialAny tries each target once, in order, with a short timeout.
func dialAny() bool {
	for _, addr := range targets {
		conn, err := net.DialTimeout("tcp", addr, probeTimeout)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

// httpAny HEADs the connectivity-check endpoints. Any response (even a
// non-2xx) proves outbound HTTP works; only transport errors count as
// offline.
func httpAny() bool {
	client := &http.Client{
		Timeout: probeTimeout,
		// Never follow redirects into a captive portal rabbit hole — a
		// redirect is still proof of connectivity.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	for _, url := range httpTargets {
		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := client.Do(req)
		cancel()
		if err == nil {
			_ = resp.Body.Close()
			return true
		}
	}
	return false
}

// dnsAny resolves a public hostname through the OS resolver.
func dnsAny() bool {
	resolver := &net.Resolver{}
	for _, host := range dnsHosts {
		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		_, err := resolver.LookupHost(ctx, host)
		cancel()
		if err == nil {
			return true
		}
	}
	return false
}
