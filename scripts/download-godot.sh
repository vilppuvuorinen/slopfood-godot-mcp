#!/usr/bin/env bash
#
# download-godot.sh - download and verify the Linux x86_64 Godot builds listed
# in a manifest of "<minor> <version> <sha256>" lines.
#
# Usage:
#   scripts/download-godot.sh [manifest]
#
# The manifest defaults to godot.versions next to this script. Each version is
# fetched from the godotengine/godot-builds GitHub release, checked against the
# manifest sha256, and unpacked into .godot/<minor>/ so point releases of the
# same minor line share a directory and upgrade in place.
#
# Environment overrides:
#   GODOT_DIR   output directory (default: <repo>/.godot)
#   GODOT_REPO  GitHub "owner/repo" to download from
#               (default: godotengine/godot-builds)
#   GODOT_ARCH  Linux architecture (default: x86_64)
#   GODOT_FORCE set to 1 to redownload entries that are already unpacked

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/.." && pwd)"

manifest="${1:-${script_dir}/godot.versions}"
out_dir="${GODOT_DIR:-${repo_root}/.godot}"
godot_repo="${GODOT_REPO:-godotengine/godot-builds}"
arch="${GODOT_ARCH:-x86_64}"
force="${GODOT_FORCE:-0}"

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

info() {
	printf '==> %s\n' "$*" >&2
}

[ "$(uname -s)" = "Linux" ] || die "this script only targets Linux"

command -v unzip >/dev/null 2>&1 || die "need unzip"
if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
	die "need curl or wget"
fi
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
	die "need sha256sum or shasum"
fi

download() {
	local url="$1" dest="$2"
	if command -v curl >/dev/null 2>&1; then
		curl --fail --location --silent --show-error --output "$dest" "$url"
	else
		wget --quiet --output-document="$dest" "$url"
	fi
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

[ -f "$manifest" ] || die "manifest not found: $manifest"

mkdir -p "$out_dir"

while IFS= read -r line || [ -n "$line" ]; do
	# Strip comments and leading/trailing whitespace.
	line="${line%%#*}"
	read -r minor version expected _ <<<"$line"

	[ -n "${minor:-}" ] || continue
	[ -n "${version:-}" ] || die "missing version for minor '${minor}' in ${manifest}"
	[ -n "${expected:-}" ] || die "missing checksum for version '${version}' in ${manifest}"
	case "$minor" in
	[0-9]*.[0-9]*) ;;
	*) die "invalid minor version '${minor}' in ${manifest}" ;;
	esac
	case "$version" in
	"${minor}"|"${minor}".*|"${minor}"-*) ;;
	*) die "version '${version}' does not belong to minor '${minor}' in ${manifest}" ;;
	esac
	expected="$(printf '%s' "$expected" | tr '[:upper:]' '[:lower:]')"

	asset="Godot_v${version}_linux.${arch}.zip"
	binary="Godot_v${version}_linux.${arch}"
	dest_dir="${out_dir}/${minor}"
	url="https://github.com/${godot_repo}/releases/download/${version}/${asset}"

	if [ "$force" != "1" ] && [ -x "${dest_dir}/${binary}" ]; then
		info "already unpacked: ${version}"
		continue
	fi

	mkdir -p "$dest_dir"
	zip="${dest_dir}/${asset}.part"

	info "downloading ${asset} (${version})"
	if ! download "$url" "$zip"; then
		rm -f "$zip"
		die "download failed: $url"
	fi

	actual="$(sha256_of "$zip")"
	if [ "$actual" != "$expected" ]; then
		rm -f "$zip"
		die "checksum mismatch for ${version}: expected ${expected}, got ${actual}"
	fi
	info "checksum ok: ${version}"

	# Drop any older point release already sitting in the shared minor dir.
	find "$dest_dir" -maxdepth 1 -name 'Godot_v*_linux.'"${arch}" ! -name "$binary" -delete

	unzip -o -q "$zip" -d "$dest_dir"
	rm -f "$zip"

	[ -f "${dest_dir}/${binary}" ] || die "expected binary not found after unpack: ${dest_dir}/${binary}"
	chmod +x "${dest_dir}/${binary}"
	info "installed ${version} -> ${dest_dir}/${binary}"
done <"$manifest"

info "done; builds in ${out_dir}"
