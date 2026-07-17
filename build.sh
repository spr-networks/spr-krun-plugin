#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

set -a
# shellcheck disable=SC1091
. ./reproducible.env
set +a

ARGS=()
while IFS='=' read -r key value; do
    case "$key" in ''|\#*) continue;; esac
    ARGS+=(--build-arg "${key}=${value}")
done < reproducible.env

docker buildx build \
    --load \
    --tag "${SPR_KRUN_PLUGIN_IMAGE:-ghcr.io/spr-networks/spr-krun-plugin:latest}" \
    "${ARGS[@]}" \
    .
