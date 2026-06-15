# Deploying the Wanderer ExApp

AppAPI deploys an ExApp through a **deploy daemon**. Three modes, and
when to use each.

## HaRP — recommended for production (Nextcloud 32+)

HaRP (HTTP/AppAPI Reverse Proxy, FRP-based) is the AppAPI project's
**recommended** daemon. AppAPI talks to the ExApp through an FRP
tunnel, so the ExApp container needs **no exposed port** and need not
be directly reachable from Nextcloud — the daemon brokers the
connection. Best security posture (nothing to firewall), and it is the
direction AppAPI is investing in. Requires **Nextcloud 32+**.

**This image supports HaRP.** When the HaRP daemon injects
`HP_SHARED_KEY`, the entrypoint (`start.sh`) starts an `frpc` client
(SHA256-pinned in the image) that tunnels to a unix socket, and the
shim listens on `/tmp/exapp.sock` (mode `0600`) instead of TCP — see
`cmd/wanderer-exapp/listen()`. No exposed port. Without `HP_SHARED_KEY`
the same image runs in plain TCP mode (DSP / docker-socket-proxy /
manual-install), so one image covers all daemons.

Validated: the app-side HaRP path (start.sh → frpc launch → unix-socket
serving of `/heartbeat` + the authed proxy) was smoke-tested. The
frps↔frpc tunnel itself needs a running HaRP server + NC 32 and was not
exercised here. **Operational note:** with `loginFailExit = false`
(upstream HaRP default), a broken frps tunnel does not crash the
container, and the *local* Docker HEALTHCHECK probes the socket
directly — so a dead tunnel is invisible to it. AppAPI's own heartbeat
poll (through frps) is the real liveness signal; watch the daemon
status in Nextcloud, not just container health.

Use HaRP when Wanderer ships to real customers via the App Store.

## Docker socket proxy — simpler, more exposure

The older daemon: AppAPI reaches the ExApp over a port on a Docker
network, brokered by a socket-proxy container that holds the Docker
socket. Works, but the ExApp port is reachable on the network and the
socket-proxy is extra surface. Fine for a self-managed single-host
install where HaRP is overkill.

## manual-install — development / this repo's validation

AppAPI does **not** start the container; you run it yourself and
register the already-running instance. This is what the README's
local-validation flow uses, and how this ExApp was smoke-tested
against NC 30.0.17 (the deploy daemon never needs to spawn a
container — which also sidesteps nested/unprivileged container hosts
that block the per-netns sysctl writes a normal `docker run` makes).

Use manual-install for dev loops and CI, not production.

## Recommendation

- **Dev / CI:** manual-install (validated here).
- **Production / App Store:** HaRP.
- Docker socket proxy only if a customer explicitly cannot run HaRP.

The image and `appinfo/info.xml` are daemon-agnostic — the same
container works under all three; only the daemon registration differs.

Refs:
- https://nextcloud.github.io/app_api/tech_details/Deployment.html
- https://docs.nextcloud.com/server/stable/admin_manual/exapps_management/AppAPIAndExternalApps.html
