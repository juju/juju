#!/usr/bin/env bash
set -euo pipefail

# This script recursively finds all .excalidraw files in the current directory
# and converts them to SVG format using the excalidraw-brute-export-cli tool.

# Find all .excalidraw filenames in the current directory that are modified in the git diff and drop the .excalidraw
# extension
excalidraw_files_without_extension=$(git diff --name-only --relative HEAD -- '*.excalidraw' | sed 's/\.excalidraw$//')

# Find all original .svg filenames in the current directory that are modified in the git diff and drop the .svg
# extension
svg_files_without_extension=$(git diff --name-status --relative HEAD -- '*.svg' | awk '{print $2}' | sed 's/\.svg$//')

# Unite list of excalidraw_files_without_extension and svg_files_without_extension excluding duplicates
files_without_extension=$(echo -e "$excalidraw_files_without_extension\n$svg_files_without_extension" | sort -u)

# If no files are found, exit the script
if [ -z "$files_without_extension" ]; then
  echo "No .excalidraw files to convert, exiting."
  exit 0
fi

# Check if excalidraw-brute-export-cli is available, install dependencies if needed
if ! npx --no-install excalidraw-brute-export-cli --help &>/dev/null; then
  echo "excalidraw-brute-export-cli not found, installing dependencies..."
  npx playwright install-deps
  echo "Dependencies installed successfully."
fi

# Explicitly install Playwright browsers, to avoid issues with missing browsers in cache
echo "Installing Playwright browsers..."
npx playwright install

echo "Starting conversion of .excalidraw files to .svg..."
echo $files_without_extension | tr ' ' '\n' | while IFS= read -r file; do
  # Check if the excalidraw file exists
  if [[ ! -f "$file.excalidraw" ]]; then
    echo "File $file.excalidraw does not exist, skipping."
    continue
  fi

  # Generate the normal SVG file from the excalidraw file
  echo "Exporting → $file.excalidraw  to  $file.svg"
  npx excalidraw-brute-export-cli \
        -i "$file.excalidraw" \
        --background 0 \
        --embed-scene 1 \
        --dark-mode 0 \
        --scale 1 \
        --format svg \
        --quiet \
        -o "$file.svg"
  # Generate the dark mode SVG file from the excalidraw file
  echo "Exporting → $file.excalidraw  to  $file.dark.svg"
  npx excalidraw-brute-export-cli \
        -i "$file.excalidraw" \
        --background 0 \
        --embed-scene 1 \
        --dark-mode 1 \
        --scale 1 \
        --format svg \
        --quiet \
        -o "$file.dark.svg"
done
