package appapi

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func authValue(user, secret string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + secret))
}

func newHandlers(secret string) *Handlers {
	return &Handlers{
		Secret: secret,
		Proxy: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot) // sentinel: request reached the backend
		}),
	}
}

func TestHeartbeat_NoAuthRequired(t *testing.T) {
	srv := httptest.NewServer(newHandlers("s3cret").Router())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/heartbeat")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status = %d, want 200 (no auth)", resp.StatusCode)
	}
}

func TestHeartbeat_ReflectsBackendProbe(t *testing.T) {
	h := newHandlers("s3cret")
	h.BackendProbe = func() bool { return false } // backend down
	srv := httptest.NewServer(h.Router())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/heartbeat")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("heartbeat with dead backend = %d, want 503", resp.StatusCode)
	}
}

func TestEnabled_RequiresValidSecret(t *testing.T) {
	srv := httptest.NewServer(newHandlers("s3cret").Router())
	defer srv.Close()

	// No auth header → 401.
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/enabled?enabled=1", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth status = %d, want 401", resp.StatusCode)
	}

	// Wrong secret → 401.
	req, _ = http.NewRequest(http.MethodPut, srv.URL+"/enabled?enabled=1", nil)
	req.Header.Set(authHeader, authValue("admin", "wrong"))
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong-secret status = %d, want 401", resp.StatusCode)
	}

	// Correct secret → 200 with the empty error array.
	req, _ = http.NewRequest(http.MethodPut, srv.URL+"/enabled?enabled=1", nil)
	req.Header.Set(authHeader, authValue("admin", "s3cret"))
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "[]" {
		t.Errorf("enabled body = %q, want []", string(body))
	}
}

func TestProxy_AuthedRequestReachesBackend(t *testing.T) {
	srv := httptest.NewServer(newHandlers("s3cret").Router())
	defer srv.Close()

	// Unauthenticated proxy attempt → 401, never reaches backend.
	resp, _ := http.Get(srv.URL + "/ui/")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth proxy status = %d, want 401", resp.StatusCode)
	}

	// Authenticated → reaches the backend sentinel (418).
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/ui/", nil)
	req.Header.Set(authHeader, authValue("admin", "s3cret"))
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("authed proxy status = %d, want 418 (backend sentinel)", resp.StatusCode)
	}
}

func TestValidAuth_EmptySecretFailsClosed(t *testing.T) {
	h := &Handlers{Secret: ""}
	if h.validAuth(authValue("admin", "")) {
		t.Error("an unconfigured (empty) secret must never authenticate")
	}
}

func TestValidAuth_SecretWithColon(t *testing.T) {
	// The secret may contain ':'; only the first separator splits.
	h := &Handlers{Secret: "ab:cd:ef"}
	if !h.validAuth(authValue("admin", "ab:cd:ef")) {
		t.Error("secret containing ':' should validate")
	}
}
