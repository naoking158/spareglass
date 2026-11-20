#!/bin/sh
# Downloads the Virtual Display Driver installer that ships in the Windows
# release archives (github.com/VirtualDrivers/Virtual-Display-Driver, MIT),
# pinned by hash, plus its license.
#
# Usage: packaging/windows/fetch-vdd.sh OUT_DIR   (needs gh)
# Writes OUT_DIR/VirtualDisplayDriver-setup.exe and OUT_DIR/VirtualDisplayDriver-LICENSE.txt.
set -eu

VDD_TAG=25.5.2
VDD_ASSET=Virtual.Display.Driver-v25.05.03-setup-x64.exe
VDD_SHA256=ca10b85babecfb636c85b3f04d2306968d4f940dd3dd35767f866207bfba846e
LICENSE_SHA256=c2285c867ddc9b88f43b4f8fa2c77a9eec66207166c1217b37353a2ccd62ac03

sha256() { if command -v sha256sum >/dev/null; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }

out=$(mkdir -p "$1" && cd "$1" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

gh release download "$VDD_TAG" --repo VirtualDrivers/Virtual-Display-Driver -p "$VDD_ASSET" -D "$work"
gh api "repos/VirtualDrivers/Virtual-Display-Driver/contents/LICENSE?ref=$VDD_TAG" -q .content | base64 -d >"$work/LICENSE"
(cd "$work" && printf '%s  %s\n%s  %s\n' "$VDD_SHA256" "$VDD_ASSET" "$LICENSE_SHA256" LICENSE | sha256 -c -)
cp "$work/$VDD_ASSET" "$out/VirtualDisplayDriver-setup.exe"
{
	cat "$work/LICENSE"
	printf '\nVirtual Display Driver %s: https://github.com/VirtualDrivers/Virtual-Display-Driver/releases/tag/%s\n' "$VDD_TAG" "$VDD_TAG"
} >"$out/VirtualDisplayDriver-LICENSE.txt"
