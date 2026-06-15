# Wanderer ExApp image: the AppAPI shim + the pinned Wanderer core
# binary, in one container. The shim is the entrypoint; it starts
# `wanderer serve` on the loopback and proxies authenticated Nextcloud
# traffic to it.
#
# WANDERER_VERSION pins the downstream dependency on the core module.
# The sync workflow (.github/workflows/sync-wanderer.yml) bumps it to
# the latest core release.
ARG WANDERER_VERSION=v0.1.0

FROM golang:1.25-alpine AS build
RUN apk add --no-cache git curl
ENV CGO_ENABLED=0
ARG WANDERER_VERSION
# Pinned Wanderer core binary (pure-Go, modernc sqlite — no cgo).
# -ldflags injects the pinned version so `wanderer version` reports it
# (the core's Makefile sets it via ldflags, which `go install` skips).
RUN go install -ldflags="-s -w -X main.Version=${WANDERER_VERSION}" \
        github.com/MWest2020/wanderer/cmd/wanderer@${WANDERER_VERSION}
WORKDIR /src
COPY . .
RUN go build -trimpath -o /out/wanderer-exapp ./cmd/wanderer-exapp

# FRP client for HaRP deploy daemons: when HaRP injects HP_SHARED_KEY,
# start.sh runs frpc to tunnel the ExApp without exposing a port.
# Pinned + SHA256-verified per the HaRP integration guide.
ARG FRP_VERSION=0.61.1
ARG FRP_AMD64_SHA256=bff260b68ca7b1461182a46c4f34e9709ba32764eed30a15dd94ac97f50a2c40
ARG FRP_ARM64_SHA256=af6366f2b43920ebfe6235dba6060770399ed1fb18601e5818552bd46a7621f8
RUN set -ex; \
    ARCH=$(uname -m); \
    if [ "$ARCH" = "aarch64" ]; then FRP_ARCH="arm64"; FRP_SHA256="${FRP_ARM64_SHA256}"; \
    else FRP_ARCH="amd64"; FRP_SHA256="${FRP_AMD64_SHA256}"; fi; \
    curl -fsSL "https://github.com/fatedier/frp/releases/download/v${FRP_VERSION}/frp_${FRP_VERSION}_linux_${FRP_ARCH}.tar.gz" -o /tmp/frp.tar.gz; \
    [ "$(sha256sum /tmp/frp.tar.gz | cut -d' ' -f1)" = "$FRP_SHA256" ]; \
    tar -C /tmp -xzf /tmp/frp.tar.gz; \
    cp /tmp/frp_${FRP_VERSION}_linux_${FRP_ARCH}/frpc /usr/local/bin/frpc; \
    rm -rf /tmp/frp_${FRP_VERSION}_linux_${FRP_ARCH} /tmp/frp.tar.gz

FROM alpine:3.20
# bash for start.sh; curl for the FRP-aware healthcheck.
RUN apk add --no-cache bash curl && \
    adduser -D -u 10001 wanderer && mkdir -p /var/lib/wanderer && chown wanderer /var/lib/wanderer
COPY --from=build /go/bin/wanderer        /usr/local/bin/wanderer
COPY --from=build /out/wanderer-exapp     /usr/local/bin/wanderer-exapp
COPY --from=build /usr/local/bin/frpc     /usr/local/bin/frpc
COPY start.sh /start.sh
RUN chmod +x /start.sh
USER wanderer
ENV APP_HOST=0.0.0.0 \
    APP_PORT=9000 \
    WANDERER_ADDR=127.0.0.1:8080 \
    WANDERER_DB=/var/lib/wanderer/wanderer.db
EXPOSE 9000
# AppAPI also polls GET /heartbeat; this Docker-level check is the
# earlier startup gate. Under HaRP the shim listens on a unix socket
# (not TCP APP_PORT), so probe accordingly.
HEALTHCHECK --interval=30s --timeout=5s --start-period=40s \
    CMD if [ -n "$HP_SHARED_KEY" ]; then \
            curl -fsS --unix-socket /tmp/exapp.sock http://localhost/heartbeat || exit 1; \
        else \
            curl -fsS "http://127.0.0.1:${APP_PORT:-9000}/heartbeat" || exit 1; \
        fi
# start.sh starts frpc (HaRP mode) then execs the shim; in DSP/manual
# mode it is a transparent pass-through.
ENTRYPOINT ["/start.sh", "/usr/local/bin/wanderer-exapp"]
