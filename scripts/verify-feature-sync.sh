#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  ./scripts/verify-feature-sync.sh <feature-id> <target-repo-dir>

Example:
  ./scripts/verify-feature-sync.sh codex-collab-orchestrator /path/to/target-repo
USAGE
}

if [[ $# -ne 2 ]]; then
  usage
  exit 1
fi

FEATURE_ID="$1"
TARGET_DIR="$2"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
FEATURE_DIR="${REPO_ROOT}/features/${FEATURE_ID}"
COMPONENTS_DIR="${FEATURE_DIR}/components"
FEATURE_NAME="$(basename "${FEATURE_ID}")"

if [[ ! -d "${FEATURE_DIR}" ]]; then
  echo "feature not found: ${FEATURE_ID}" >&2
  exit 1
fi
if [[ ! -d "${TARGET_DIR}" ]]; then
  echo "target repo not found: ${TARGET_DIR}" >&2
  exit 1
fi

TARGET_DIR="$(cd "${TARGET_DIR}" && pwd)"
CODEX_DIR="${TARGET_DIR}/.codex"
AGENTS_DIR="${TARGET_DIR}/.agents"

SOURCE_AGENTS_DIR="${COMPONENTS_DIR}/agents"
SOURCE_MCP_DIR="${COMPONENTS_DIR}/mcp"
SOURCE_SKILLS_DIR="${COMPONENTS_DIR}/skills"
TARGET_AGENT_BUNDLE_DIR="${CODEX_DIR}/agents/${FEATURE_NAME}"
TARGET_MCP_DIR="${CODEX_DIR}/mcp/features/${FEATURE_NAME}"
TARGET_SKILLS_DIR="${AGENTS_DIR}/skills"
MANIFEST_PATH="${CODEX_DIR}/features/${FEATURE_NAME}/install-manifest.json"

status=0

compare_dirs() {
  local source_dir="$1"
  local target_dir="$2"
  local label="$3"

  if [[ ! -d "${source_dir}" ]]; then
    echo "[warn] source missing for ${label}: ${source_dir}"
    return
  fi
  if [[ ! -d "${target_dir}" ]]; then
    echo "[fail] target missing for ${label}: ${target_dir}"
    status=1
    return
  fi

  local diff_output
  diff_output="$(diff -qr \
    --exclude '.cache' \
    --exclude '.toolchains' \
    "${source_dir}" "${target_dir}" || true)"
  if [[ -n "${diff_output}" ]]; then
    echo "[fail] mismatch detected for ${label}"
    echo "${diff_output}"
    status=1
  else
    echo "[ok] ${label} is synced"
  fi
}

compare_dirs "${SOURCE_AGENTS_DIR}" "${TARGET_AGENT_BUNDLE_DIR}" "agent templates"
compare_dirs "${SOURCE_MCP_DIR}" "${TARGET_MCP_DIR}" "mcp assets"

if [[ -d "${SOURCE_SKILLS_DIR}" ]]; then
  while IFS= read -r manifest_path; do
    [[ -n "${manifest_path}" ]] || continue
    skill_dir="$(dirname "${manifest_path}")"
    skill_name="$(basename "${skill_dir}")"
    compare_dirs "${skill_dir}" "${TARGET_SKILLS_DIR}/${skill_name}" "skill:${skill_name}"
  done < <(find "${SOURCE_SKILLS_DIR}" -type f -name 'SKILL.md' | sort || true)
fi

if [[ -f "${MANIFEST_PATH}" ]]; then
  echo "[ok] install manifest exists: ${MANIFEST_PATH}"
else
  echo "[fail] install manifest missing: ${MANIFEST_PATH}"
  status=1
fi

if [[ ${status} -ne 0 ]]; then
  echo "result: NOT SYNCED"
  exit 1
fi

echo "result: SYNCED"
