#!/usr/bin/env bash
set -euo pipefail

find . -type f -name '*.excalidraw' | while IFS= read -r file; do
  dir=$(dirname "$file")
  base=$(basename  "$file" .excalidraw)
  out="$dir/$base.svg"
  echo "Exporting → $file  to  $out"
  excalidraw-brute-export-cli \
        -i "$file" \
        --background 0 \
        --embed-scene 1 \
        --dark-mode 0 \
        --scale 1 \
        --format svg \
        --quite \
        -o "$out"
done
