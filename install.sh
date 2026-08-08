#!/bin/sh

set -eu

OWNER="nazar256"
REPO="datadog-axi"
PROJECT_NAME="datadog-axi"
BINARY_NAME="datadog-axi"
BASE_URL_OVERRIDE="${DATADOG_AXI_INSTALL_BASE_URL:-}"

usage() {
  cat <<'EOF'
Install datadog-axi from the latest GitHub release.

Usage:
  install.sh [--version <tag>] [--install-dir <dir>]

Options:
  --version <tag>      Install a specific compatible release tag (for example vX.Y.Z)
  --install-dir <dir>  Install into a specific directory
  -h, --help           Show this help message
EOF
}

log() {
  printf '%s\n' "$*" >&2
}

fail() {
  log "Error: $*"
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

detect_os() {
  case "$(uname -s)" in
    Linux) printf 'linux' ;;
    Darwin) printf 'darwin' ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
}

resolve_latest_tag() {
  header_file="$1"
  curl -fsSLI "https://github.com/${OWNER}/${REPO}/releases/latest" >"${header_file}"
  location_line=$(grep -i '^location:' "${header_file}" | tail -n 1 || true)
  [ -n "${location_line}" ] || fail "could not resolve latest release"
  tag=$(printf '%s' "${location_line}" | tr -d '\r' | sed -nE 's#.*releases/tag/([^[:space:]]+).*#\1#p')
  [ -n "${tag}" ] || fail "could not parse latest release tag"
  printf '%s' "${tag}"
}

is_writable_dir() {
  dir="$1"
  [ -d "${dir}" ] || return 1
  [ -w "${dir}" ] || return 1
}

dir_in_path() {
  dir="$1"
  case ":${PATH}:" in
    *:"${dir}":*) return 0 ;;
    *) return 1 ;;
  esac
}

choose_install_dir() {
  if [ -n "${INSTALL_DIR}" ]; then
    mkdir -p "${INSTALL_DIR}"
    is_writable_dir "${INSTALL_DIR}" || fail "install dir is not writable: ${INSTALL_DIR}"
    printf '%s' "${INSTALL_DIR}"
    return
  fi

  old_ifs=${IFS}
  IFS=:
  for candidate in ${PATH}; do
    [ -n "${candidate}" ] || continue
    if is_writable_dir "${candidate}"; then
      IFS=${old_ifs}
      printf '%s' "${candidate}"
      return
    fi
  done
  IFS=${old_ifs}

  for fallback in "${HOME}/.local/bin" "${HOME}/bin"; do
    mkdir -p "${fallback}"
    if is_writable_dir "${fallback}"; then
      printf '%s' "${fallback}"
      return
    fi
  done

  fail "could not find a writable install directory"
}

verify_checksum() {
  archive_path="$1"
  checksum_file="$2"
  archive_name=$(basename "${archive_path}")

  if command -v sha256sum >/dev/null 2>&1; then
    expected=$(grep "  ${archive_name}" "${checksum_file}" | awk '{print $1}')
    actual=$(sha256sum "${archive_path}" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    expected=$(grep "  ${archive_name}" "${checksum_file}" | awk '{print $1}')
    actual=$(shasum -a 256 "${archive_path}" | awk '{print $1}')
  else
    fail "need sha256sum or shasum to verify release checksum"
  fi

  [ -n "${expected}" ] || fail "checksum entry not found for ${archive_name}"
  [ "${expected}" = "${actual}" ] || fail "checksum verification failed for ${archive_name}"
}

VERSION=""
INSTALL_DIR=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || fail "missing value for --version"
      VERSION="$2"
      shift 2
      ;;
    --install-dir)
      [ "$#" -ge 2 ] || fail "missing value for --install-dir"
      INSTALL_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

require_cmd curl
require_cmd tar
require_cmd mktemp
require_cmd install

tmpdir=$(mktemp -d)
trap 'rm -rf "${tmpdir}"' EXIT INT TERM HUP

if [ -z "${VERSION}" ]; then
  VERSION=$(resolve_latest_tag "${tmpdir}/latest.headers")
fi

OS=$(detect_os)
ARCH=$(detect_arch)
ARCHIVE_NAME="${PROJECT_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
CHECKSUM_NAME="${PROJECT_NAME}_${VERSION}_checksums.txt"
BASE_URL="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}"
if [ -n "${BASE_URL_OVERRIDE}" ]; then
  BASE_URL="${BASE_URL_OVERRIDE}"
fi
ARCHIVE_PATH="${tmpdir}/${ARCHIVE_NAME}"
CHECKSUM_PATH="${tmpdir}/${CHECKSUM_NAME}"
TARGET_DIR=$(choose_install_dir)

log "Installing ${BINARY_NAME} ${VERSION} for ${OS}/${ARCH}"
log "Downloading release assets from ${BASE_URL}"
curl -fsSL "${BASE_URL}/${ARCHIVE_NAME}" -o "${ARCHIVE_PATH}"
curl -fsSL "${BASE_URL}/${CHECKSUM_NAME}" -o "${CHECKSUM_PATH}"

verify_checksum "${ARCHIVE_PATH}" "${CHECKSUM_PATH}"

tar -xzf "${ARCHIVE_PATH}" -C "${tmpdir}"
[ -f "${tmpdir}/${BINARY_NAME}" ] || fail "archive did not contain ${BINARY_NAME}"

install -m 0755 "${tmpdir}/${BINARY_NAME}" "${TARGET_DIR}/${BINARY_NAME}"

log "Installed to ${TARGET_DIR}/${BINARY_NAME}"
if dir_in_path "${TARGET_DIR}"; then
  "${TARGET_DIR}/${BINARY_NAME}" version
else
  log "${TARGET_DIR} is not currently in PATH"
  log "Add it with: export PATH=\"${TARGET_DIR}:\$PATH\""
  "${TARGET_DIR}/${BINARY_NAME}" version
fi
