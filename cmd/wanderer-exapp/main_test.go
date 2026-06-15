package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
)

// serveOK serves a trivial 200 handler over ln until the test ends.
func serveOK(t *testing.T, ln net.Listener) {
	t.Helper()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

func TestListen_HarpModeUsesUnixSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "exapp.sock")
	cfg := config{harpKey: "shared-key", harpSocket: sock}

	ln, err := listen(cfg, slog.Default())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if ln.Addr().Network() != "unix" {
		t.Fatalf("network = %q, want unix", ln.Addr().Network())
	}
	serveOK(t, ln)

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
	}}
	resp, err := client.Get("http://unix/heartbeat")
	if err != nil {
		t.Fatalf("get over unix socket: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestListen_HarpModeRemovesStaleSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "exapp.sock")
	cfg := config{harpKey: "k", harpSocket: sock}
	// First bind, then close — leaves the socket file behind.
	ln1, err := listen(cfg, slog.Default())
	if err != nil {
		t.Fatalf("first listen: %v", err)
	}
	_ = ln1.Close()
	// Second listen must succeed by removing the stale socket file.
	ln2, err := listen(cfg, slog.Default())
	if err != nil {
		t.Fatalf("second listen (stale socket): %v", err)
	}
	_ = ln2.Close()
}

func TestListen_DefaultModeUsesTCP(t *testing.T) {
	cfg := config{appHost: "127.0.0.1", appPort: "0"} // :0 picks a free port
	ln, err := listen(cfg, slog.Default())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	if ln.Addr().Network() != "tcp" {
		t.Fatalf("network = %q, want tcp", ln.Addr().Network())
	}
	serveOK(t, ln)

	resp, err := http.Get("http://" + ln.Addr().String() + "/heartbeat")
	if err != nil {
		t.Fatalf("get over tcp: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
