#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
FEATURES_DIR="${REPO_ROOT}/features"

if [[ ! -d "${FEATURES_DIR}" ]]; then
  echo "features directory not found: ${FEATURES_DIR}" >&2
  exit 1
fi

for feature_dir in "${FEATURES_DIR}"/*; do
  [[ -d "${feature_dir}" ]] || continue
  feature_id="$(basename "${feature_dir}")"
  manifest_path="${feature_dir}/feature.yaml"

  if [[ -f "${manifest_path}" ]]; then
    summary="$(awk -F': ' '/^summary:/{print $2; exit}' "${manifest_path}")"
  else
    summary="(no manifest summary)"
  fi

  echo "- ${feature_id}: ${summary}"
done

