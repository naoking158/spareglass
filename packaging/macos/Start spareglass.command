#!/bin/sh
# Double-click in Finder to start spareglass in a Terminal window.
cd "$(dirname "$0")" || exit 1

# A zip downloaded with a browser marks every extracted file as quarantined.
# macOS has already asked the user about this script, so clear the mark on the
# rest of the folder; otherwise the bundled ffmpeg is blocked when it starts.
xattr -dr com.apple.quarantine . 2>/dev/null

./spareglass "$@"
echo
echo "spareglass has stopped. You can close this window."
