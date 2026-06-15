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
RUN apk add --no-cache git
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

FROM alpine:3.20
RUN adduser -D -u 10001 wanderer && mkdir -p /var/lib/wanderer && chown wanderer /var/lib/wanderer
COPY --from=build /go/bin/wanderer        /usr/local/bin/wanderer
COPY --from=build /out/wanderer-exapp     /usr/local/bin/wanderer-exapp
USER wanderer
ENV APP_HOST=0.0.0.0 \
    APP_PORT=9000 \
    WANDERER_ADDR=127.0.0.1:8080 \
    WANDERER_DB=/var/lib/wanderer/wanderer.db
EXPOSE 9000
# AppAPI also polls GET /heartbeat; this Docker-level check is the
# earlier startup gate.
HEALTHCHECK --interval=30s --timeout=5s --start-period=40s \
    CMD wget -qO- "http://127.0.0.1:${APP_PORT:-9000}/heartbeat" || exit 1
ENTRYPOINT ["/usr/local/bin/wanderer-exapp"]
