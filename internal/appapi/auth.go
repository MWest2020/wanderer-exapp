// Package appapi implements the Nextcloud AppAPI ExApp contract for
// the Wanderer ExApp: the lifecycle endpoints AppAPI calls
// (/heartbeat, /init, /enabled) and the AppAPIAuth check on incoming
// requests from Nextcloud.
//
// Contract reference (verified 2026-06-15):
//   - GET  /heartbeat        — no auth, 200 when the app is up
//   - POST /init             — AppAPIAuth, 200 (optional init work)
//   - PUT  /enabled?enabled= — AppAPIAuth, 200, body "[]" on success
//   - AUTHORIZATION-APP-API  — base64("<userid>:<secret>"); the
//     secret must equal the APP_SECRET the deploy daemon injected.
//
// https://nextcloud.github.io/app_api/notes_for_developers/ExAppLifecycle.html
// https://nextcloud.github.io/app_api/tech_details/Authentication.html
//
// NOTE: this is the baseline shared-secret check. Newer AppAPI
// versions may add request signing; validate against the target
// AppAPI version before production. The live deploy was not smoke-
// tested in the authoring environment (no Docker) — see README.
package appapi

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

// authHeader is the header AppAPI sets on every authenticated request
// from Nextcloud to the ExApp.
const authHeader = "AUTHORIZATION-APP-API"

// requireAuth wraps next so it only runs when the incoming request
// carries a valid AUTHORIZATION-APP-API header whose secret matches
// the ExApp's own secret. Anything else gets 401.
func (h *Handlers) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.validAuth(r.Header.Get(authHeader)) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// validAuth reports whether raw is a base64-encoded "<userid>:<secret>"
// whose secret matches the configured ExApp secret. The comparison is
// constant-time. An empty configured secret never authenticates (a
// misconfigured ExApp must fail closed, not open).
func (h *Handlers) validAuth(raw string) bool {
	if raw == "" || h.Secret == "" {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return false
	}
	// "<userid>:<secret>" — the secret may itself contain ':', so
	// split only on the first separator.
	idx := strings.IndexByte(string(decoded), ':')
	if idx < 0 {
		return false
	}
	secret := string(decoded)[idx+1:]
	return subtle.ConstantTimeCompare([]byte(secret), []byte(h.Secret)) == 1
}
