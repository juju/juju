#!/usr/bin/env bash

# Regenerate the export surface and fail on any drift: either a model-DB
# schema change without moving exportVersionStrings to the current dev
# version (a released payload version is frozen), or regenerated output
# that was not committed.

set -euo pipefail

go generate ./generate/export

drift_paths=(domain/export)
if [[ -d domain/modelimport ]]; then
  drift_paths+=(domain/modelimport)
fi
if [[ -n $(git status --porcelain -- "${drift_paths[@]}") ]]; then
  git status --porcelain -- "${drift_paths[@]}"
  echo "*****"
  echo "The generated export surface is stale. Either a model-DB schema change was made"
  echo "without moving exportVersionStrings to the current dev version (a released"
  echo "payload version is frozen; see the domain/export/version.go godoc), or the"
  echo "regenerated output was not committed. Run \`go generate ./generate/export\`"
  echo "and commit the result."
  echo "*****"
  exit 1
fi
