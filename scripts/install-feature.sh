#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./scripts/install-feature.sh <feature-id> <target-dir> [--dry-run]

Example:
  ./scripts/install-feature.sh codex-collab-orchestrator /path/to/workspace
  ./scripts/install-feature.sh codex-collab-orchestrator /path/to/workspace --dry-run
EOF
}

if [[ $# -lt 2 || $# -gt 3 ]]; then
  usage
  exit 1
fi

FEATURE_ID="$1"
TARGET_DIR="$2"
DRY_RUN="${3:-}"

if [[ -n "${DRY_RUN}" && "${DRY_RUN}" != "--dry-run" ]]; then
  echo "unknown option: ${DRY_RUN}" >&2
  usage
  exit 1
fi

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
FEATURE_DIR="${REPO_ROOT}/features/${FEATURE_ID}"
COMPONENTS_DIR="${FEATURE_DIR}/components"

if [[ ! -d "${FEATURE_DIR}" ]]; then
  echo "feature not found: ${FEATURE_ID}" >&2
  echo "hint: ./scripts/list-features.sh" >&2
  exit 1
fi

if [[ ! -d "${COMPONENTS_DIR}" ]]; then
  echo "components directory missing: ${COMPONENTS_DIR}" >&2
  exit 1
fi

TARGET_DIR="$(cd "${TARGET_DIR}" 2>/dev/null && pwd || true)"
if [[ -z "${TARGET_DIR}" ]]; then
  if [[ "${DRY_RUN}" == "--dry-run" ]]; then
    TARGET_DIR="$2"
  else
    mkdir -p "$2"
    TARGET_DIR="$(cd "$2" && pwd)"
  fi
fi

copy_component() {
  local component_name="$1"
  local source_dir="${COMPONENTS_DIR}/${component_name}"
  local destination_dir="${TARGET_DIR}/${component_name}"

  if [[ ! -d "${source_dir}" ]]; then
    return
  fi

  echo "installing ${component_name} -> ${destination_dir}"
  if [[ "${DRY_RUN}" == "--dry-run" ]]; then
    return
  fi

  mkdir -p "${destination_dir}"
  cp -R "${source_dir}/." "${destination_dir}/"
}

echo "feature: ${FEATURE_ID}"
echo "source:  ${FEATURE_DIR}"
echo "target:  ${TARGET_DIR}"

copy_component "agents"
copy_component "skills"
copy_component "mcp"

echo "done"

