// Command wanderer-exapp is the Nextcloud AppAPI shim for Wanderer.
//
// It runs as the container entrypoint: it starts the Wanderer binary
// on the container loopback, then serves the AppAPI lifecycle surface
// (/heartbeat, /init, /enabled) on APP_PORT and reverse-proxies every
// other authenticated request through to Wanderer. The Wanderer core
// stays untouched — this shim consumes the released binary (pinned in
// the Dockerfile), which is why it lives in a separate downstream
// repo.
package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/MWest2020/wanderer-exapp/internal/appapi"
)

// config is the runtime surface, sourced entirely from environment
// variables. The AppAPI deploy daemon injects APP_*; NEXTCLOUD_URL is
// the Nextcloud base; the WANDERER_* vars are this shim's own knobs
// for the colocated Wanderer process.
type config struct {
	appPort      string // APP_PORT — where the deploy daemon reaches us
	appHost      string // APP_HOST — bind address (usually 0.0.0.0)
	appSecret    string // APP_SECRET — shared secret for AppAPIAuth
	wandererAddr string // WANDERER_ADDR — loopback addr Wanderer listens on
	wandererBin  string // WANDERER_BIN — path to the wanderer binary ("" = don't spawn)
	wandererDB   string // WANDERER_DB — Wanderer's SQLite path
}

func loadConfig() config {
	return config{
		appPort:      envOr("APP_PORT", "9000"),
		appHost:      envOr("APP_HOST", "0.0.0.0"),
		appSecret:    os.Getenv("APP_SECRET"),
		wandererAddr: envOr("WANDERER_ADDR", "127.0.0.1:8080"),
		wandererBin:  envOr("WANDERER_BIN", "wanderer"),
		wandererDB:   envOr("WANDERER_DB", "/var/lib/wanderer/wanderer.db"),
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	cfg := loadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start the colocated Wanderer binary, unless WANDERER_BIN is
	// blanked (e.g. a compose that runs Wanderer in its own service).
	if cfg.wandererBin != "" {
		if err := startWanderer(ctx, cfg, logger, stop); err != nil {
			logger.Error("wanderer.start_failed", "err", err)
			os.Exit(1)
		}
	}

	backend, err := url.Parse("http://" + cfg.wandererAddr)
	if err != nil {
		logger.Error("config.bad_wanderer_addr", "addr", cfg.wandererAddr, "err", err)
		os.Exit(1)
	}
	proxy := httputil.NewSingleHostReverseProxy(backend)
	// Strip the AppAPI shared-secret header before forwarding to the
	// colocated Wanderer process: it has already served its purpose
	// at the gate, and Wanderer's request logging must never capture
	// it.
	baseDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		baseDirector(r)
		r.Header.Del("AUTHORIZATION-APP-API")
	}
	handlers := &appapi.Handlers{
		Secret:       cfg.appSecret,
		Proxy:        proxy,
		Logger:       logger,
		BackendProbe: func() bool { return dialable(cfg.wandererAddr) },
	}
	if cfg.appSecret == "" {
		// Not fatal — the ExApp may be registered before the deploy
		// daemon injects the secret — but every authed route will 401
		// until it is set, so make that visible in the logs.
		logger.Warn("appapi.no_secret", "msg", "APP_SECRET is empty; all authed routes will 401 until it is injected")
	}

	srv := &http.Server{
		Addr:              cfg.appHost + ":" + cfg.appPort,
		Handler:           handlers.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("exapp.start", "addr", srv.Addr, "wanderer", cfg.wandererAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("exapp.serve_error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("exapp.stopping")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// startWanderer launches `wanderer serve` on the loopback address the
// proxy targets. The process inherits the shim's context, so a
// shutdown signal tears both down together; on ctx cancel the process
// is sent SIGTERM (then SIGKILL after a grace period) so SQLite closes
// cleanly. An *unexpected* exit (ctx still live) calls stop() to bring
// the whole container down rather than serve a dead backend silently —
// the orchestrator then restarts it, turning "silently broken" into an
// observable crash-loop.
func startWanderer(ctx context.Context, cfg config, logger *slog.Logger, stop func()) error {
	cmd := exec.CommandContext(ctx, cfg.wandererBin, "serve",
		"-addr", cfg.wandererAddr,
		"-db", cfg.wandererDB,
		"-no-geoip",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 10 * time.Second
	if err := cmd.Start(); err != nil {
		return err
	}
	logger.Info("wanderer.started", "pid", cmd.Process.Pid, "addr", cfg.wandererAddr)
	go func() {
		err := cmd.Wait()
		if ctx.Err() != nil {
			return // expected: we're shutting down
		}
		logger.Error("wanderer.exited_unexpectedly", "err", err)
		stop()
	}()
	return nil
}

// dialable reports whether a TCP connection to addr succeeds within a
// short timeout — used by the heartbeat to reflect backend liveness.
func dialable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
