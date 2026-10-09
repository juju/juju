#!/usr/bin/env bash

# Regenerate the export surface and fail on any drift: the committed
# generated output must match what `go generate ./generate/export`
# produces. Released payload versions are frozen by the sibling check,
# check-export-version-frozen.sh.

set -euo pipefail

go generate ./generate/export

drift_paths=(domain/export)
if [[ -d domain/modelimport ]]; then
  drift_paths+=(domain/modelimport)
fi
if [[ -n $(git status --porcelain -- "${drift_paths[@]}") ]]; then
  git status --porcelain -- "${drift_paths[@]}"
  echo "*****"
  echo "The generated export surface is stale: the committed output does not"
  echo "match regeneration. Run \`go generate ./generate/export\` and commit"
  echo "the result."
  echo "*****"
  exit 1
fi
