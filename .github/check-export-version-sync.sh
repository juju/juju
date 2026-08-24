#!/usr/bin/env bash

# Every non-own exportVersionStrings entry must be the latest payload
# version of its minor line on the canonical source branch.

set -euo pipefail

VERSION_FILE=domain/export/version.go

parse_entries() {
  sed -n '/exportVersionStrings = \[\]string{/,/^}/p' | grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"' || true
}

mapfile -t entries < <(parse_entries <"$VERSION_FILE")
count=${#entries[@]}
if [[ $count -eq 0 ]]; then
  echo "invalid exportVersionStrings shape (0 entries); see $VERSION_FILE godoc"
  exit 1
fi
if [[ $count -eq 1 ]]; then
  echo "source-only branch (one export payload version); skipping source-branch sync check"
  exit 0
fi

own=${entries[-1]}
for ((i = 0; i < count - 1; i++)); do
  prev=${entries[i]}
  branch=${prev%.*}
  git fetch --depth=1 https://github.com/juju/juju.git "$branch"
  remote=$(git show "FETCH_HEAD:$VERSION_FILE" | parse_entries | grep -E "^${branch//./\\.}\." | sort -V | tail -1 || true)
  if [[ "$remote" != "$prev" ]]; then
    echo "the $branch source branch moved its export payload version to $remote. On this branch:"
    echo "(1) update the $branch entry of exportVersionStrings in $VERSION_FILE to $remote;"
    echo "(2) copy domain/export/types/v${remote//./_}/ from the $branch branch and delete the superseded types directory;"
    echo "(3) run \`go generate ./generate/export\` to regenerate the transform scaffolding;"
    echo "(4) implement any new Deltas methods in domain/modelimport/transformer/transforms/to_v${own//./_}/deltas.go;"
    echo "(5) run the modelimport and export tests."
    exit 1
  fi
  echo "export payload version sync OK: $branch source branch is at $prev"
done
