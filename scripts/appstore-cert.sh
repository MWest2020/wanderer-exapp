#!/usr/bin/env bash
# appstore-cert.sh — generate the Nextcloud App Store signing
# certificate request for app-id "wanderer" (one-time).
#
# After running this, open a PR adding wanderer.csr to
#   https://github.com/nextcloud/app-certificate-requests
# (your GitHub profile must show your email). Once merged you receive
# wanderer.crt; drop it next to the key. Then put the *private key*
# (wanderer.key) into the wanderer-exapp repo secret APPSTORE_CERT_KEY
# and your apps.nextcloud.com API token into APPSTORE_TOKEN so the
# appstore-release workflow can sign + publish.
#
# The key is a credential — keep wanderer.key out of git.
set -euo pipefail

APP_ID="wanderer"
DIR="${1:-$HOME/.nextcloud/certificates}"
mkdir -p "${DIR}"

if [ -f "${DIR}/${APP_ID}.key" ]; then
  echo "refusing to overwrite existing ${DIR}/${APP_ID}.key" >&2
  exit 1
fi

openssl req -nodes -newkey rsa:4096 \
  -keyout "${DIR}/${APP_ID}.key" \
  -out "${DIR}/${APP_ID}.csr" \
  -subj "/CN=${APP_ID}"

echo
echo "Generated:"
echo "  ${DIR}/${APP_ID}.key   (PRIVATE — never commit; becomes secret APPSTORE_CERT_KEY)"
echo "  ${DIR}/${APP_ID}.csr   (submit this as a PR to nextcloud/app-certificate-requests)"
echo
echo "CSR contents to paste into the PR:"
echo "----------------------------------"
cat "${DIR}/${APP_ID}.csr"
