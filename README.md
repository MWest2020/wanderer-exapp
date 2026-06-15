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

## Local validation (run this — it was NOT smoke-tested here)

The authoring environment had no Docker, so the **Go shim is unit-
tested (`go test ./...`) but the live AppAPI deploy against a real
Nextcloud is unverified**. To validate:

```sh
docker compose -f deploy/docker-compose.dev.yml up -d --build
# finish the Nextcloud wizard at http://localhost:8080 (admin/admin),
# ensure the AppAPI app is enabled, then register a manual deploy
# daemon and this ExApp via `occ app_api:daemon:register` +
# `occ app_api:app:register` (see the AppAPI docs).
```

`appinfo/info.xml` is a **template** — validate its schema against your
target AppAPI version before publishing (the manifest format drifted
across AppAPI 3.0+).

## Status

Skeleton / spike. The Go shim builds, vets, and passes unit tests. The
container build, ExApp manifest, and AppAPI deploy need a live
Nextcloud to validate end-to-end.

## Licence

EUPL-1.2 — same as the Wanderer core.
