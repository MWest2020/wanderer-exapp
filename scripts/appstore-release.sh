#!/usr/bin/env bash
# appstore-release.sh — package, sign, and publish a Wanderer ExApp
# release to the Nextcloud App Store.
#
# An ExApp's store archive is just the app metadata: a top-level
# `wanderer/appinfo/info.xml` (+ optional screenshots). The actual
# code is the Docker image referenced inside info.xml's
# <external-app><docker-install>, which the App Store does not host.
#
# Inputs (env):
#   APPSTORE_CERT_KEY  PEM private key for app-id wanderer (the secret)
#   APPSTORE_TOKEN     apps.nextcloud.com API token
#   DOWNLOAD_URL       public URL where the built tarball is hosted
#                      (e.g. the GitHub release asset)
#   NIGHTLY            "true" | "false" (default false)
# Produces ./wanderer.tar.gz (the archive to upload to DOWNLOAD_URL).
set -euo pipefail

: "${DOWNLOAD_URL:?set DOWNLOAD_URL (public URL of the uploaded tarball)}"
: "${APPSTORE_CERT_KEY:?set APPSTORE_CERT_KEY (signing private key)}"
: "${APPSTORE_TOKEN:?set APPSTORE_TOKEN (apps.nextcloud.com API token)}"
NIGHTLY="${NIGHTLY:-false}"

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# 1. assemble the archive: wanderer/appinfo/info.xml (+ screenshots)
mkdir -p "$work/wanderer/appinfo"
cp "$root/appinfo/info.xml" "$work/wanderer/appinfo/info.xml"
[ -d "$root/screenshots" ] && cp -r "$root/screenshots" "$work/wanderer/screenshots"
tar -C "$work" -czf "$root/wanderer.tar.gz" wanderer
echo "==> built $root/wanderer.tar.gz"

# 2. sign the archive with the app private key
keyfile="$work/wanderer.key"
printf '%s' "$APPSTORE_CERT_KEY" > "$keyfile"
signature="$(openssl dgst -sha512 -sign "$keyfile" "$root/wanderer.tar.gz" | openssl base64 -A)"

# 3. publish the release (the tarball must already be reachable at DOWNLOAD_URL)
echo "==> POST /api/v1/apps/releases (download=$DOWNLOAD_URL nightly=$NIGHTLY)"
http_code="$(curl -sS -o /tmp/appstore-resp -w '%{http_code}' \
  -X POST "https://apps.nextcloud.com/api/v1/apps/releases" \
  -H "Authorization: Token ${APPSTORE_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"download\":\"${DOWNLOAD_URL}\",\"signature\":\"${signature}\",\"nightly\":${NIGHTLY}}")"
echo "HTTP ${http_code}"; cat /tmp/appstore-resp; echo
case "$http_code" in
  200|201) echo "==> published." ;;
  *) echo "==> appstore upload failed (HTTP $http_code)" >&2; exit 1 ;;
esac
