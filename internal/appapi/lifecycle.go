package appapi

import (
	"log/slog"
	"net/http"
)

// Handlers serves the AppAPI lifecycle endpoints and proxies every
// other path to the Wanderer binary running beside it in the
// container. Secret is the APP_SECRET injected by the deploy daemon;
// Proxy is the reverse proxy to the local Wanderer HTTP API.
type Handlers struct {
	Secret string
	Proxy  http.Handler
	Logger *slog.Logger
	// BackendProbe reports whether the colocated Wanderer process is
	// reachable. /heartbeat returns 503 when it is not, so AppAPI's
	// liveness gate reflects the truth instead of always-200 — this
	// also covers the startup race (Wanderer not yet listening).
	// Nil means "assume healthy" (used in unit tests).
	BackendProbe func() bool
}

// Router builds the ExApp HTTP surface:
//
//	GET  /heartbeat  — liveness, no auth
//	POST /init       — post-enable init hook, auth
//	PUT  /enabled    — enable/disable, auth
//	/*               — everything else, auth, proxied to Wanderer
func (h *Handlers) Router() http.Handler {
	if h.Logger == nil {
		h.Logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/heartbeat", h.heartbeat)
	mux.HandleFunc("/init", h.requireAuth(h.initHandler))
	mux.HandleFunc("/enabled", h.requireAuth(h.enabled))
	mux.HandleFunc("/", h.requireAuth(h.proxy))
	return mux
}

// heartbeat answers AppAPI's periodic liveness probe. No auth: the
// probe runs before the secret handshake is meaningful.
func (h *Handlers) heartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.BackendProbe != nil && !h.BackendProbe() {
		writeJSON(w, http.StatusServiceUnavailable, `{"status":"backend_unavailable"}`)
		return
	}
	writeJSON(w, http.StatusOK, `{"status":"ok"}`)
}

// initHandler runs one-time setup after the ExApp is enabled. Wanderer
// needs no model downloads or data seeding, so this is a no-op 200 —
// but the route must exist (a 404 here makes AppAPI think init failed
// rather than "nothing to do").
func (h *Handlers) initHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.Logger.Info("appapi.init")
	w.WriteHeader(http.StatusOK)
}

// enabled is called with ?enabled=1 when the admin enables the ExApp
// and ?enabled=0 when they disable it. Wanderer registers no extra
// Nextcloud UI surfaces yet, so there is nothing to (un)register;
// success is the empty error array AppAPI expects.
func (h *Handlers) enabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.Logger.Info("appapi.enabled", "enabled", r.URL.Query().Get("enabled"))
	// An empty JSON array means "no errors" to AppAPI.
	writeJSON(w, http.StatusOK, `[]`)
}

// proxy forwards an authenticated Nextcloud request to the Wanderer
// binary running on the container's loopback.
func (h *Handlers) proxy(w http.ResponseWriter, r *http.Request) {
	if h.Proxy == nil {
		http.Error(w, "wanderer backend not configured", http.StatusBadGateway)
		return
	}
	h.Proxy.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
