#!/usr/bin/env bash
set -euo pipefail

# This script recursively finds all .excalidraw files in the current directory
# and converts them to SVG format using the excalidraw-brute-export-cli tool.

find . -type f -name '*.excalidraw' | while IFS= read -r file; do
  #check if file has diff in git environment
  if git diff --quiet "$file"; then
    echo "No changes detected in $file, skipping export."
    continue
  fi

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
