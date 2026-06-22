#!/usr/bin/env bash
#
# PaqetPremium — one-line bootstrap.
# Downloads the full installer/manager from the repo and runs it.
#
#   curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/paqetpremium/master/scripts/install-linux.sh | sudo bash
#
# Any arguments are forwarded to install-premium.sh, e.g.:
#   ... | sudo bash -s -- server
#   ... | sudo bash -s -- update          # rebuild + restart (prefers source when available)
#
set -Eeuo pipefail

REPO_RAW="${REPO_RAW:-https://raw.githubusercontent.com/iPmartNetwork/paqetpremium/master}"

if [[ ${EUID} -ne 0 ]]; then
  echo "[x] Please run as root (sudo)." >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "[x] curl is required to bootstrap. Install it and retry." >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

echo "[i] Fetching installer from ${REPO_RAW}/install-premium.sh ..."
curl -fsSL "${REPO_RAW}/install-premium.sh" -o "$tmp"
chmod +x "$tmp"
exec bash "$tmp" "$@"
