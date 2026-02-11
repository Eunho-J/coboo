#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
GO_BIN="$("${SCRIPT_DIR}/bootstrap-go.sh")"
GOROOT="$(cd -- "$(dirname -- "${GO_BIN}")/.." && pwd)"

exec "${GOROOT}/bin/gofmt" "$@"

