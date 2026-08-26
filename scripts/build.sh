#!/usr/bin/env bash
# Builds kbs's binaries for Windows and Linux. Works run from either OS
# (plain bash on Linux/CI, Git Bash on Windows) since it only ever
# cross-compiles via GOOS/GOARCH - no CGO, so no host toolchain needed
# either way.
#
# Usage:
#   scripts/build.sh                    # target+controller (windows+linux), tray (windows)
#   scripts/build.sh --suffix 2         # -> ea-share-target2.exe, ea-share-controller2.exe, ea-share-tray2.exe
#   scripts/build.sh --os windows       # windows only
#   scripts/build.sh --os linux         # linux only (no tray: Windows-only)
#   scripts/build.sh --skip-tray        # skip cmd/tray entirely
#
# --suffix is for testing a change without disrupting a deployment already
# in use elsewhere: it builds under a different binary name instead of
# overwriting bin/ea-share-target.exe etc. See ai-context.md.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

suffix=""
target_os="all"
skip_tray=false

while [[ $# -gt 0 ]]; do
	case "$1" in
	--suffix)
		suffix="$2"
		shift 2
		;;
	--os)
		target_os="$2"
		shift 2
		;;
	--skip-tray)
		skip_tray=true
		shift
		;;
	-h | --help)
		sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "build.sh: unknown argument: $1" >&2
		exit 2
		;;
	esac
done

case "$target_os" in
all | windows | linux) ;;
*)
	echo "build.sh: --os must be windows, linux, or all (got: $target_os)" >&2
	exit 2
	;;
esac

mkdir -p bin

build() {
	local goos="$1" name="$2" pkg="$3"
	shift 3
	local ext=""
	[[ "$goos" == "windows" ]] && ext=".exe"
	local out="bin/${name}${suffix}${ext}"
	echo "-> $out (GOOS=$goos)"
	GOOS="$goos" GOARCH=amd64 go build "$@" -o "$out" "$pkg"
}

if [[ "$target_os" == "all" || "$target_os" == "windows" ]]; then
	build windows ea-share-target ./cmd/target
	build windows ea-share-controller ./cmd/controller
	if [[ "$skip_tray" != true ]]; then
		# -H=windowsgui: no console flash when double-clicked or launched
		# by the shell (ea-share-tray.exe has no terminal UI of its own).
		build windows ea-share-tray ./cmd/tray -ldflags "-H=windowsgui"
	fi
fi

if [[ "$target_os" == "all" || "$target_os" == "linux" ]]; then
	build linux ea-share-target ./cmd/target
	build linux ea-share-controller ./cmd/controller
	# cmd/tray is Windows-only (fyne.io/systray backend + CREATE_NO_WINDOW
	# syscalls); nothing to build for Linux.
fi

echo "done."
