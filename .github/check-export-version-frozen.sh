#!/usr/bin/env bash

# Guard released export payload versions (see the domain/export/version.go
# godoc): a payload version is frozen once its release ships, and the
# generated surface stamped with it may never be regenerated in place.
#
# A version is released when upstream tag v<version> exists. The frozen
# content is anchored to the latest release on that version's line: a
# release line cannot ship a schema change without moving its entry
# first, so the latest line release carries, byte for byte, the format
# every later commit must reproduce. This catches both failure shapes:
#   - the own entry regenerated in place after its release shipped;
#   - a vendored copy diverging from what the source branch shipped.
# A schema change that leaves the generated surface untouched (e.g. a
# new table excluded from the export) passes: it needs no payload
# version move. Transforms and registered steps are excluded because
# they legitimately change when a non-own entry moves (re-vendoring);
# the sync check guards those.

set -euo pipefail

VERSION_FILE=domain/export/version.go
UPSTREAM=https://github.com/juju/juju.git

parse_array() {
  sed -n "/^var $1 = \[\]string{/,/^}/p" "$VERSION_FILE" |
    grep -oE '"[0-9]+\.[0-9]+\.[0-9]+"' | tr -d '"' || true
}

mapfile -t entries < <(parse_array exportVersionStrings)
mapfile -t controller_entries < <(parse_array controllerExportVersionStrings)
if [[ ${#entries[@]} -eq 0 ]]; then
  echo "invalid exportVersionStrings shape (0 entries); see $VERSION_FILE godoc"
  exit 1
fi
own=${entries[-1]}

released() {
  git ls-remote --exit-code --tags "$UPSTREAM" "refs/tags/v$1" >/dev/null 2>&1
}

declare -A anchor_for=()
# latest_release prints the newest numeric tag on a version line, or
# nothing when the line has not shipped yet.
latest_release() {
  local line=$1
  if [[ ! ${anchor_for[$line]+set} ]]; then
    anchor_for[$line]=$(
      git ls-remote --tags "$UPSTREAM" "refs/tags/v$line.*" 2>/dev/null |
        awk '{print $2}' | grep -E "^refs/tags/v${line}\.[0-9]+$" |
        sed -E 's|refs/tags/v||' | sort -V | tail -1 || true
    )
  fi
  printf '%s' "${anchor_for[$line]}"
}

# check_frozen <version> <own|non-own> <pathspec>... compares the
# stamped files at HEAD against the latest release of the version's
# line. Unreleased versions and lines without releases pass vacuously.
check_frozen() {
  local version=$1 ownership=$2
  shift 2
  if ! released "$version"; then
    echo "freeze check vacuous: $version is not released (no tag v$version)"
    return 0
  fi
  local line=${version%.*} anchor
  anchor=$(latest_release "$line")
  if [[ -z $anchor ]]; then
    echo "freeze check vacuous: line $line has no released version to anchor to"
    return 0
  fi
  git fetch --quiet --depth=1 "$UPSTREAM" "refs/tags/v$anchor"
  # --diff-filter=MR: only content the release actually shipped. Files
  # added or deleted against the anchor (path relocations, superseded
  # entries) are the version-list and sync checks' business.
  local offenders
  offenders=$(git diff --name-only --diff-filter=MR FETCH_HEAD HEAD -- "$@")
  if [[ -n $offenders ]]; then
    echo "*****"
    echo "payload version $version is released (tag v$version exists) and frozen,"
    echo "but the generated surface below was changed in place instead of matching"
    echo "what release v$anchor shipped:"
    echo "$offenders"
    if [[ $ownership == own ]]; then
      echo "Move the own entry of exportVersionStrings (or controllerExportVersion-"
      echo "Strings) in $VERSION_FILE to the current dev version and run"
      echo "\`go generate ./generate/export\`; never regenerate a released payload"
      echo "version in place. See the $VERSION_FILE godoc."
    else
      echo "The vendored types for $version must match what the $line branch"
      echo "shipped verbatim. Copy the files above from the $line branch; if that"
      echo "branch changed them in place, fix the $line branch first."
    fi
    echo "*****"
    return 1
  fi
  echo "frozen OK: $version matches release v$anchor"
}

fail=0
for v in "${entries[@]}"; do
  paths=(domain/export/types/v"${v//./_}")
  if [[ $v == "$own" ]]; then
    # The unversioned model-pass files carry the own version's schema.
    paths+=(
      domain/export/state/model
      domain/export/service/export.go
      domain/export/service/export_test.go
    )
  fi
  check_frozen "$v" "$( [[ $v == "$own" ]] && echo own || echo non-own )" \
    "${paths[@]}" || fail=1
done

controller_own=""
if [[ ${#controller_entries[@]} -gt 0 ]]; then
  controller_own=${controller_entries[-1]}
fi
for v in "${controller_entries[@]}"; do
  paths=(domain/export/types/controller/v"${v//./_}")
  if [[ $v == "$controller_own" ]]; then
    paths+=(
      domain/export/state/controller
      domain/export/service/controller_export.go
      domain/export/service/controller_export_test.go
    )
  fi
  check_frozen "$v" "$( [[ $v == "$controller_own" ]] && echo own || echo non-own )" \
    "${paths[@]}" || fail=1
done

exit $fail
