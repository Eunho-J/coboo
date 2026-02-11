#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

GO_BIN="$("${SCRIPT_DIR}/bootstrap-go.sh")"
GOROOT="$(cd -- "$(dirname -- "${GO_BIN}")/.." && pwd)"

export GOROOT
export GOCACHE="${GOCACHE:-${PROJECT_ROOT}/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-${PROJECT_ROOT}/.cache/go-mod}"
export PATH="${GOROOT}/bin:${PATH}"

# Keep Go module cache writable so feature directories can be removed cleanly.
if [[ " ${GOFLAGS:-} " != *" -modcacherw "* ]]; then
  export GOFLAGS="${GOFLAGS:+${GOFLAGS} }-modcacherw"
fi

mkdir -p "${GOCACHE}" "${GOMODCACHE}"

exec "${GO_BIN}" "$@"
