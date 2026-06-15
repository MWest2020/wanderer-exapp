# wanderer-exapp

Nextcloud **ExApp** packaging for [Wanderer](https://github.com/MWest2020/wanderer)
— it ships Wanderer as a marketplace-installable Nextcloud External
App, deployed and managed as a container by Nextcloud's
[AppAPI](https://nextcloud.github.io/app_api/).

This repo is **downstream** of the core: the Wanderer core (pure Go,
all logic) stays untouched in `MWest2020/wanderer`. This repo adds
only the AppAPI surface — a thin Go shim plus container/manifest
packaging — and tracks the core via a pinned version that a CI
workflow bumps on each core release.

## Why a separate repo

The core is pure Go with its API under `internal/` (module-private, so
it cannot be imported by another module). Packaging it for the
Nextcloud marketplace pulls in container + manifest concerns that have
no place in the core. Keeping them apart means `go test ./...` in the
core never needs Docker or any Nextcloud tooling — the constraint the
marketplace OpenSpec change set out.

## How it works

```
Nextcloud  ──AppAPI──►  wanderer-exapp container
                         ├─ shim (this repo): /heartbeat /init /enabled
                         └─ wanderer core binary on 127.0.0.1:8080
                            (pinned via go install, started by the shim)
```

The shim (`cmd/wanderer-exapp`):
- serves the AppAPI lifecycle endpoints (`GET /heartbeat` unauthenticated,
  `POST /init`, `PUT /enabled` — both AppAPIAuth);
- validates the `AUTHORIZATION-APP-API` shared secret (`internal/appapi`);
- reverse-proxies every other authenticated request to the colocated
  Wanderer binary.

Config comes from the env the AppAPI deploy daemon injects
(`APP_ID`, `APP_SECRET`, `APP_HOST`, `APP_PORT`, `NEXTCLOUD_URL`, …)
plus a few shim knobs for the colocated core process:

| env | default | meaning |
|-----|---------|---------|
| `WANDERER_ADDR` | `127.0.0.1:8080` | loopback address Wanderer listens on / the proxy targets |
| `WANDERER_DB` | `/var/lib/wanderer/wanderer.db` | Wanderer's SQLite path |
| `WANDERER_BIN` | `wanderer` | core binary to spawn; set empty to **not** spawn it (e.g. a compose that runs Wanderer as its own service) |

`/heartbeat` reflects backend liveness: it returns `503` until the
colocated Wanderer is reachable (covering the startup race) and if it
later dies. An unexpected Wanderer exit brings the container down so
the orchestrator restarts it rather than serving a dead backend.

## Staying current with the core

`.github/workflows/sync-wanderer.yml` rebuilds the image pinned to a
specific core version. Wire the core repo to dispatch on release:

```sh
gh api repos/MWest2020/wanderer-exapp/dispatches \
  -f event_type=wanderer-release \
  -f 'client_payload[version]=<tag>'
```

Until the core tags releases, the image pins `WANDERER_VERSION=main`.
Tagging the core (e.g. `v0.1.0`) gives reproducible pins — a core-side
decision, not done here.

## Local validation

The container was **smoke-tested end-to-end against Nextcloud 30.0.17
+ AppAPI 4.0.6** (2026-06-15): the image builds, the shim starts the
core, AppAPI authenticated against it, called `/init`, and the ExApp
registered as `enabled`. The reproduction (using AppAPI's
`manual-install` daemon, which registers an already-running container
— the path that works without the deploy daemon spawning containers
itself):

```sh
# 1. build + run the ExApp container (host network avoids the per-netns
#    sysctl write that unprivileged/nested container hosts block):
docker build -t wanderer-exapp .
docker run -d --name wexapp --network host -e APP_SECRET=secret -e APP_PORT=9000 wanderer-exapp

# 2. a Nextcloud with AppAPI (default since NC 30.0.1), then:
occ app_api:daemon:register manual Manual manual-install http 127.0.0.1 http://127.0.0.1 --set-default
occ app_api:app:register wanderer manual --wait-finish --json-info \
  '{"appid":"wanderer","name":"Wanderer","daemon_config_name":"manual","version":"0.1.0","secret":"secret","host":"127.0.0.1","port":9000,"scopes":[],"system_app":false}'
occ app_api:app:list   # → wanderer (Wanderer): 0.1.0 [enabled]
```

`deploy/docker-compose.dev.yml` is the compose harness for the same
flow. Note: in the validation above the runtime contract was exercised
via `--json-info`; `appinfo/info.xml` is the **App-Store packaging
manifest** and is still a template — validate its schema against your
target AppAPI version before publishing (the format drifted across
AppAPI 3.0+).

## Status

Working spike, validated against a live Nextcloud 30.0.17 / AppAPI
4.0.6 (build → run → AppAPI register → enabled). The Go shim builds,
vets, and passes unit tests. Remaining before "production": tag core
releases + wire the release→dispatch sync, validate the `info.xml`
App-Store manifest, and decide HaRP vs docker-socket-proxy deploy for
non-manual installs.

## Licence

EUPL-1.2 — same as the Wanderer core.
