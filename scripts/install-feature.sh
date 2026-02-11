#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./scripts/install-feature.sh <feature-id> <target-repo-dir> [--dry-run]

Example:
  ./scripts/install-feature.sh codex-collab-orchestrator /path/to/repo
  ./scripts/install-feature.sh codex-collab-orchestrator /path/to/repo --dry-run
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
FEATURE_NAME="$(basename "${FEATURE_ID}")"

if [[ ! -d "${FEATURE_DIR}" ]]; then
  echo "feature not found: ${FEATURE_ID}" >&2
  echo "hint: ./scripts/list-features.sh" >&2
  exit 1
fi

if [[ ! -d "${COMPONENTS_DIR}" ]]; then
  echo "components directory missing: ${COMPONENTS_DIR}" >&2
  exit 1
fi

if [[ "${DRY_RUN}" == "--dry-run" ]]; then
  if [[ -d "${TARGET_DIR}" ]]; then
    TARGET_DIR="$(cd "${TARGET_DIR}" && pwd)"
  fi
else
  mkdir -p "${TARGET_DIR}"
  TARGET_DIR="$(cd "${TARGET_DIR}" && pwd)"
fi

CODEX_DIR="${TARGET_DIR}/.codex"
AGENTS_DIR="${TARGET_DIR}/.agents"
SKILLS_TARGET_DIR="${AGENTS_DIR}/skills"
AGENT_BUNDLE_DIR="${CODEX_DIR}/agents/${FEATURE_NAME}"
MCP_FEATURE_DIR="${CODEX_DIR}/mcp/features/${FEATURE_NAME}"
CONFIG_TOML_PATH="${CODEX_DIR}/config.toml"
AGENTS_MD_PATH="${TARGET_DIR}/AGENTS.md"

copy_directory() {
  local source_dir="$1"
  local destination_dir="$2"
  local label="$3"
  if [[ ! -d "${source_dir}" ]]; then
    return
  fi
  echo "installing ${label} -> ${destination_dir}"
  if [[ "${DRY_RUN}" == "--dry-run" ]]; then
    return
  fi
  mkdir -p "$(dirname "${destination_dir}")"
  chmod -R u+w "${destination_dir}" 2>/dev/null || true
  rm -rf "${destination_dir}"
  cp -R "${source_dir}" "${destination_dir}"
}

install_skills() {
  local source_root="${COMPONENTS_DIR}/skills"
  if [[ ! -d "${source_root}" ]]; then
    return
  fi

  local manifests
  manifests="$(find "${source_root}" -type f -name 'SKILL.md' | sort || true)"
  if [[ -z "${manifests}" ]]; then
    return
  fi

  echo "installing skills -> ${SKILLS_TARGET_DIR}"
  if [[ "${DRY_RUN}" != "--dry-run" ]]; then
    mkdir -p "${SKILLS_TARGET_DIR}"
  fi

  while IFS= read -r manifest_path; do
    [[ -n "${manifest_path}" ]] || continue
    local skill_dir
    local skill_name
    local destination_dir
    skill_dir="$(dirname "${manifest_path}")"
    skill_name="$(basename "${skill_dir}")"
    destination_dir="${SKILLS_TARGET_DIR}/${skill_name}"
    echo "  - ${skill_name} -> ${destination_dir}"
    if [[ "${DRY_RUN}" == "--dry-run" ]]; then
      continue
    fi
    chmod -R u+w "${destination_dir}" 2>/dev/null || true
    rm -rf "${destination_dir}"
    cp -R "${skill_dir}" "${destination_dir}"
  done <<< "${manifests}"
}

write_managed_block() {
  local file_path="$1"
  local block_tag="$2"
  local block_body="$3"

  local start_marker end_marker temp_path
  start_marker="# BEGIN ${block_tag}"
  end_marker="# END ${block_tag}"
  temp_path="${file_path}.tmp.$$"

  if [[ "${DRY_RUN}" == "--dry-run" ]]; then
    echo "[dry-run] update managed block ${block_tag} in ${file_path}"
    return
  fi

  mkdir -p "$(dirname "${file_path}")"
  touch "${file_path}"

  awk -v start="${start_marker}" -v end="${end_marker}" '
    BEGIN {skip=0}
    {
      if ($0 == start) {skip=1; next}
      if (skip && $0 == end) {skip=0; next}
      if (!skip) print
    }
  ' "${file_path}" > "${temp_path}"

  {
    cat "${temp_path}"
    if [[ -s "${temp_path}" ]]; then
      printf "\n"
    fi
    printf "%s\n" "${start_marker}"
    printf "%s\n" "${block_body}"
    printf "%s\n" "${end_marker}"
  } > "${file_path}"

  rm -f "${temp_path}"
}

configure_mcp_servers() {
  local server_source_root="${COMPONENTS_DIR}/mcp/servers"
  if [[ ! -d "${server_source_root}" ]]; then
    return
  fi

  local block_lines=""
  local has_entry="false"

  for source_server_dir in "${server_source_root}"/*; do
    [[ -d "${source_server_dir}" ]] || continue
    local server_name
    local server_key
    local go_wrapper
    local cmd_dir
    local installed_server_dir
    server_name="$(basename "${source_server_dir}")"
    server_key="${FEATURE_NAME}_${server_name}"
    server_key="${server_key//-/_}"
    installed_server_dir="${MCP_FEATURE_DIR}/servers/${server_name}"
    go_wrapper="${installed_server_dir}/scripts/go.sh"
    cmd_dir="${source_server_dir}/cmd/${server_name}"

    if [[ ! -x "${source_server_dir}/scripts/go.sh" || ! -d "${cmd_dir}" ]]; then
      continue
    fi

    has_entry="true"
    block_lines+=$'\n'
    block_lines+="[mcp_servers.${server_key}]"$'\n'
    block_lines+="command = \"${go_wrapper}\""$'\n'
    block_lines+="args = [\"-C\", \"${installed_server_dir}\", \"run\", \"./cmd/${server_name}\", \"--mode\", \"serve\", \"--repo\", \"${TARGET_DIR}\"]"$'\n'
    block_lines+="startup_timeout_sec = 120"$'\n'
  done

  if [[ "${has_entry}" != "true" ]]; then
    return
  fi

  write_managed_block "${CONFIG_TOML_PATH}" "codex-feature:${FEATURE_NAME}:mcp" "${block_lines}"
}

configure_agents_file() {
  local block_body
  block_body=$(cat <<EOF
<!-- Codex feature: ${FEATURE_NAME} -->
- Feature bundle installed: \`${FEATURE_NAME}\`
- Skill directory: \`.agents/skills\`
- Agent templates: \`.codex/agents/${FEATURE_NAME}\`
- MCP config is managed in \`.codex/config.toml\` via block \`codex-feature:${FEATURE_NAME}:mcp\`
EOF
)

  write_managed_block "${AGENTS_MD_PATH}" "codex-feature:${FEATURE_NAME}:agents" "${block_body}"
}

echo "feature: ${FEATURE_ID}"
echo "source:  ${FEATURE_DIR}"
echo "target:  ${TARGET_DIR}"
echo "codex:   ${CODEX_DIR}"
echo "agents:  ${AGENTS_DIR}"

copy_directory "${COMPONENTS_DIR}/agents" "${AGENT_BUNDLE_DIR}" "agent templates"
copy_directory "${COMPONENTS_DIR}/mcp" "${MCP_FEATURE_DIR}" "mcp assets"
install_skills
configure_mcp_servers
configure_agents_file

echo "done"
echo "next:"
echo "- restart Codex session so new AGENTS/skills/MCP config are loaded."
echo "- optional verify: codex mcp list (from target repo)"
