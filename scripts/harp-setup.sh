#!/usr/bin/env bash
# harp-setup.sh — stand up the Wanderer ExApp on a homelab Nextcloud
# via the HaRP deploy daemon (the recommended production path).
#
# Run this ON the Docker host that also runs Nextcloud (or that the
# Nextcloud can reach). It:
#   1. generates a HaRP shared key (or uses $HP_SHARED_KEY),
#   2. starts the HaRP proxy container,
#   3. prints the exact AppAPI "Register Daemon" values for you to
#      enter once in the Nextcloud admin UI (the daemon registration
#      is a one-time admin action whose occ flags vary by version, so
#      the UI is the reliable path),
#   4. registers the Wanderer ExApp against that daemon via occ.
#
# IMPORTANT (Proxmox/LXC users): run the Docker engine on a VM or a
# privileged/nesting-enabled LXC. In an unprivileged LXC, daemon-
# spawned bridge containers fail to start (the kernel blocks the
# net.ipv4.ip_unprivileged_port_start sysctl write). A normal Docker
# host is unaffected.
#
# Requires: docker, and either NC_CONTAINER (a Nextcloud Docker
# container name, occ run via `docker exec`) or OCC set to how you
# invoke occ.
set -euo pipefail

# --- config (override via environment) ------------------------------
NC_URL="${NC_URL:?set NC_URL, e.g. https://cloud.example.nl}"
HOST_IP="${HOST_IP:?set HOST_IP — the Docker host IP the daemon/FRP bind to}"
NC_CONTAINER="${NC_CONTAINER:-nextcloud}"          # Nextcloud container name for occ
OCC="${OCC:-docker exec -u www-data ${NC_CONTAINER} php occ}"
HP_SHARED_KEY="${HP_SHARED_KEY:-$(openssl rand -hex 24)}"
HARP_IMAGE="${HARP_IMAGE:-ghcr.io/nextcloud/nextcloud-appapi-harp:release}"
EXAPP_VERSION="${EXAPP_VERSION:-latest}"            # ghcr image tag of the ExApp
INFO_XML="${INFO_XML:-https://raw.githubusercontent.com/MWest2020/wanderer-exapp/main/appinfo/info.xml}"
DAEMON="${DAEMON:-appapi-harp}"
CERTS_DIR="${CERTS_DIR:-$(pwd)/harp-certs}"

echo "==> HaRP shared key: ${HP_SHARED_KEY}"
echo "    (save this — you need it in the admin UI below)"

# --- 1. start the HaRP proxy container ------------------------------
mkdir -p "${CERTS_DIR}"
if docker ps -a --format '{{.Names}}' | grep -qx "${DAEMON}"; then
  echo "==> HaRP container '${DAEMON}' already exists; leaving it as-is."
else
  echo "==> starting HaRP proxy container..."
  docker run -d \
    -e HP_SHARED_KEY="${HP_SHARED_KEY}" \
    -e NC_INSTANCE_URL="${NC_URL}" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "${CERTS_DIR}":/certs \
    --name "${DAEMON}" -h "${DAEMON}" \
    --restart unless-stopped \
    -p 8780:8780 -p 8782:8782 \
    "${HARP_IMAGE}"
fi

# --- 2. ensure AppAPI is enabled ------------------------------------
echo "==> ensuring the AppAPI app is enabled..."
${OCC} app:enable app_api >/dev/null 2>&1 || true

# --- 3. register the deploy daemon (one-time, via the admin UI) -----
cat <<EOF

==> Register the HaRP deploy daemon ONCE in the Nextcloud admin UI:
    Settings -> Administration -> AppAPI -> "Register Daemon"

    Daemon Configuration template : HaRP Proxy (HOST)
    Surname / Name                : ${DAEMON}
    Display name                  : ${DAEMON}
    Deployment method             : docker-install
    HaRP host                     : ${HOST_IP}:8780
    HaRP shared key               : ${HP_SHARED_KEY}
    Nextcloud URL                 : ${NC_URL}
    FRP server address            : ${HOST_IP}:8782
    Docker network                : bridge

    Then click "Check connection". It must succeed before continuing.

EOF
read -r -p "Press ENTER once the daemon is registered and the connection check passed... " _

# --- 4. register the Wanderer ExApp against the daemon --------------
echo "==> registering the Wanderer ExApp (AppAPI/HaRP pulls the ghcr image, tunnels, runs the lifecycle)..."
${OCC} app_api:app:register wanderer "${DAEMON}" \
  --info-xml "${INFO_XML}" \
  --wait-finish

echo "==> done. Current ExApps:"
${OCC} app_api:app:list
echo "    Expect: wanderer (Wanderer): <version> [enabled]"
