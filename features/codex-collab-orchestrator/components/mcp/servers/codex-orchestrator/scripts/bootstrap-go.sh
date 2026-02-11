#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

GO_VERSION="${GO_VERSION:-1.24.0}"

detect_os() {
  local os_name
  os_name="$(uname -s)"
  case "${os_name}" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *)
      echo "unsupported OS: ${os_name} (expected darwin or linux)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  local arch_name
  arch_name="$(uname -m)"
  case "${arch_name}" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      echo "unsupported architecture: ${arch_name} (expected amd64 or arm64)" >&2
      exit 1
      ;;
  esac
}

sha256_of_file() {
  local target_file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${target_file}" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "${target_file}" | awk '{print $1}'
    return
  fi
  echo "sha256 command not found (sha256sum or shasum required)" >&2
  exit 1
}

OS="$(detect_os)"
ARCH="$(detect_arch)"
ARCHIVE_NAME="go${GO_VERSION}.${OS}-${ARCH}.tar.gz"
ARCHIVE_URL="https://dl.google.com/go/${ARCHIVE_NAME}"
CHECKSUM_URL="${ARCHIVE_URL}.sha256"

TOOLCHAIN_ROOT="${PROJECT_ROOT}/.toolchains/go/${GO_VERSION}/${OS}-${ARCH}"
GO_BIN="${TOOLCHAIN_ROOT}/go/bin/go"

if [[ -x "${GO_BIN}" ]] && "${GO_BIN}" version 2>/dev/null | grep -q "go${GO_VERSION}"; then
  echo "${GO_BIN}"
  exit 0
fi

DOWNLOAD_DIR="${PROJECT_ROOT}/.toolchains/cache"
TMP_DIR="${PROJECT_ROOT}/.toolchains/.tmp"
mkdir -p "${DOWNLOAD_DIR}" "${TMP_DIR}"

ARCHIVE_PATH="${DOWNLOAD_DIR}/${ARCHIVE_NAME}"
if [[ ! -f "${ARCHIVE_PATH}" ]]; then
  curl -fsSL "${ARCHIVE_URL}" -o "${ARCHIVE_PATH}"
fi

EXPECTED_CHECKSUM="$(curl -fsSL "${CHECKSUM_URL}" | tr -d '[:space:]')"
ACTUAL_CHECKSUM="$(sha256_of_file "${ARCHIVE_PATH}")"

if [[ -z "${EXPECTED_CHECKSUM}" ]]; then
  echo "failed to fetch checksum from ${CHECKSUM_URL}" >&2
  exit 1
fi

if [[ "${EXPECTED_CHECKSUM}" != "${ACTUAL_CHECKSUM}" ]]; then
  echo "checksum mismatch for ${ARCHIVE_NAME}" >&2
  echo "expected: ${EXPECTED_CHECKSUM}" >&2
  echo "actual:   ${ACTUAL_CHECKSUM}" >&2
  exit 1
fi

EXTRACT_DIR="${TMP_DIR}/go-${GO_VERSION}-${OS}-${ARCH}"
rm -rf "${EXTRACT_DIR}" "${TOOLCHAIN_ROOT}"
mkdir -p "${EXTRACT_DIR}" "${TOOLCHAIN_ROOT}"

tar -xzf "${ARCHIVE_PATH}" -C "${EXTRACT_DIR}"
mv "${EXTRACT_DIR}/go" "${TOOLCHAIN_ROOT}/go"
rm -rf "${EXTRACT_DIR}"

echo "${GO_BIN}"
