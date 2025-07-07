#!/usr/bin/env bash
set -euo pipefail

# This script recursively finds all .excalidraw files in the current directory
# and converts them to SVG format using the excalidraw-brute-export-cli tool.
# It requires Node.js and the Playwright library to be installed.
#
# Note: The playwright installation is a little heavy, so it might take some time. But it is necessary for the
#       excalidraw-brute-export-cli to work. This the only way to export excalidraw files to SVG, saving the fonts and
#       other assets in the SVG file, all other tools do not provide this feature.
npx playwright install
npx playwright install-deps

find . -type f -name '*.excalidraw' | while IFS= read -r file; do
  dir=$(dirname "$file")
  base=$(basename  "$file" .excalidraw)
  out="$dir/$base.svg"
  echo "Exporting → $file  to  $out"
  npx excalidraw-brute-export-cli \
        -i "$file" \
        --background 0 \
        --embed-scene 1 \
        --dark-mode 0 \
        --scale 1 \
        --format svg \
        --quiet \
        -o "$out"
done
