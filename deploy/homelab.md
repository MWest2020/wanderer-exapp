# Homelab: run Wanderer on your own Nextcloud (HaRP)

Hands-on guide to deploy the Wanderer ExApp on a self-hosted Nextcloud
via HaRP — the production path — and validate the full tunnel. For the
conceptual comparison of daemon modes, see [DEPLOY.md](DEPLOY.md).

## Prerequisites

- **Nextcloud 32+** (HaRP requires it; AppAPI ships by default).
- A **Docker host** that can spawn bridge containers.
  - ⚠️ **Proxmox / LXC:** run Docker on a **VM** or a **privileged LXC
    with `nesting=1` (+ `keyctl=1`)**. In an *unprivileged* LXC,
    AppAPI's daemon-spawned containers fail to start — the kernel
    blocks the `net.ipv4.ip_unprivileged_port_start` sysctl write that
    Docker sets on every bridge container. A normal Docker host / VM is
    unaffected. (This is the one wall a sandboxed CI cannot cross.)
- `openssl` (the setup script generates the shared key).

## 1. Deploy + register (scripted)

From this repo on the Docker host:

```sh
NC_URL="https://cloud.example.nl" \
HOST_IP="192.0.2.10" \
NC_CONTAINER="nextcloud" \
bash scripts/harp-setup.sh
```

The script ([`scripts/harp-setup.sh`](../scripts/harp-setup.sh)):
1. generates a HaRP shared key,
2. starts the `appapi-harp` proxy container (ports 8780/8782, Docker
   socket mounted),
3. prints the exact **Register Daemon** values to enter once in
   *Settings → Administration → AppAPI* (the daemon registration is a
   one-time admin action; its `occ` flags vary by version, so the UI is
   the reliable path), then waits,
4. registers the Wanderer ExApp via `occ app_api:app:register` — AppAPI
   pulls `ghcr.io/mwest2020/wanderer-exapp:latest`, HaRP tunnels to it,
   and the lifecycle (`/heartbeat`, `/init`, `/enabled`) runs over the
   FRP tunnel.

## 2. Verify

```sh
docker exec -u www-data nextcloud php occ app_api:app:list
# → wanderer (Wanderer): 0.1.0 [enabled]
```

Then open Nextcloud and confirm Wanderer's UI is reachable through it.
If `app:list` shows `enabled` and the daemon's "Test deploy" passes,
the full HaRP tunnel works end-to-end.

## Troubleshooting

- **Daemon "Check connection" fails:** the `HOST_IP` must be reachable
  from the Nextcloud container, and ports 8780/8782 open on the host.
- **ExApp stuck "deploying" / container won't start:** almost always
  the unprivileged-LXC sysctl wall above — move Docker to a VM /
  privileged LXC.
- **`enabled` but UI unreachable:** a broken FRP tunnel is invisible to
  the container's local healthcheck (see DEPLOY.md); check the daemon
  status + the `appapi-harp` container logs.

## 3. Publish to the App Store (optional, after homelab works)

1. `bash scripts/appstore-cert.sh` → submit the CSR to
   [nextcloud/app-certificate-requests](https://github.com/nextcloud/app-certificate-requests),
   receive `wanderer.crt`.
2. Add repo secrets `APPSTORE_CERT_KEY` (the private key) and
   `APPSTORE_TOKEN` (your apps.nextcloud.com token).
3. Replace the placeholder `<screenshot>` in `appinfo/info.xml` with a
   real image under `screenshots/`.
4. Publish a GitHub release — the
   [`appstore-release`](../.github/workflows/appstore-release.yml)
   workflow signs the metadata archive and uploads it to the store.
