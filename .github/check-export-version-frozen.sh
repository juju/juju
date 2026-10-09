#!/usr/bin/env bash

# Guard released export payload versions (see the domain/export/version.go
# godoc): a payload version is frozen once its release ships, and the
# generated surface stamped with it may never be regenerated in place.
#
# A version is frozen when a stable upstream release on its line reaches
# that version, even if its own-numbered tag was skipped. The frozen
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
if grep -qE '^[[:space:]]*(var[[:space:]]+)?controllerExportVersionStrings[[:space:]]*=' \
    "$VERSION_FILE" && [[ ${#controller_entries[@]} -eq 0 ]]; then
  echo "invalid controllerExportVersionStrings shape (0 entries); see $VERSION_FILE godoc"
  exit 1
fi
own=${entries[-1]}

# Query once so a failed upstream request cannot masquerade as an unreleased
# version, and all comparisons use the same release-tag snapshot.
if ! release_refs=$(git ls-remote --tags --refs "$UPSTREAM" 'refs/tags/v*'); then
  echo "could not query upstream release tags; refusing to skip the freeze check" >&2
  exit 1
fi

declare -A anchor_for=()
declare -A commit_for=()
# Cache the newest numeric release on a version line, or an empty string
# when it has no stable releases. Call directly so the cache survives.
latest_release() {
  local line=$1
  if [[ ! ${anchor_for[$line]+set} ]]; then
    if ! anchor_for[$line]=$(
      awk '{print $2}' <<<"$release_refs" |
        sed -nE "s|^refs/tags/v(${line//./\\.}\.[0-9]+)$|\\1|p" |
        sort -V | tail -1
    ); then
      echo "could not determine the latest release on line $line" >&2
      return 1
    fi
  fi
  return 0
}

# check_frozen <model|controller> <version> <own|non-own> <pathspec>...
# compares the stamped files at HEAD against the latest release on the
# version's line. Only a successful tag query can skip an unreleased version.
check_frozen() {
  local kind=$1 version=$2 ownership=$3
  shift 3
  local line=${version%.*} anchor
  if ! latest_release "$line"; then
    return 1
  fi
  anchor=${anchor_for[$line]}
  # A schema stamped with an older version cannot become mutable just
  # because that exact release tag is missing. Only future versions skip.
  if [[ -z $anchor ]]; then
    echo "$kind freeze check vacuous: $version is not released (no stable release on line $line)"
    return 0
  fi
  local first
  if ! first=$(printf '%s\n' "$version" "$anchor" | sort -V | head -1); then
    echo "could not compare payload and release versions on line $line" >&2
    return 1
  fi
  if [[ $first != "$version" ]]; then
    echo "$kind freeze check vacuous: $version is not released (latest release is v$anchor)"
    return 0
  fi
  if [[ ! ${commit_for[$line]+set} ]]; then
    if ! git fetch --quiet --depth=1 "$UPSTREAM" "refs/tags/v$anchor"; then
      echo "could not fetch release v$anchor" >&2
      return 1
    fi
    if ! commit_for[$line]=$(git rev-parse --verify 'FETCH_HEAD^{commit}'); then
      echo "could not resolve release v$anchor" >&2
      return 1
    fi
  fi
  # Every difference under a listed, released version is a violation,
  # including additions and deletions. Superseded versions are not checked
  # once their entries move. Disable rename detection to check both paths.
  local offenders
  if ! offenders=$(git diff --name-only --no-renames \
      "${commit_for[$line]}" HEAD -- "$@"); then
    echo "could not compare $kind payload $version with release v$anchor" >&2
    return 1
  fi
  if [[ -n $offenders ]]; then
    echo "*****"
    echo "$kind payload version $version is frozen by release v$anchor,"
    echo "but the generated surface below was changed in place instead of matching"
    echo "what release v$anchor shipped:"
    echo "$offenders"
    if [[ $ownership == own ]]; then
      local array=exportVersionStrings
      if [[ $kind == controller ]]; then
        array=controllerExportVersionStrings
      fi
      echo "Move the own entry of $array in $VERSION_FILE"
      echo "to the current dev version and run"
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
  echo "$kind frozen OK: $version matches release v$anchor"
}

fail=0
for v in "${entries[@]}"; do
  paths=(domain/export/types/v"${v//./_}")
  ownership=non-own
  if [[ $v == "$own" ]]; then
    ownership=own
    # The unversioned model-pass files carry the own version's schema.
    paths+=(
      domain/export/state/model/export.go
      domain/export/state/model/export_test.go
      domain/export/service/export.go
      domain/export/service/export_test.go
    )
  fi
  check_frozen model "$v" "$ownership" "${paths[@]}" || fail=1
done

controller_own=""
if [[ ${#controller_entries[@]} -gt 0 ]]; then
  controller_own=${controller_entries[-1]}
fi
for v in "${controller_entries[@]}"; do
  paths=(domain/export/types/controller/v"${v//./_}")
  ownership=non-own
  if [[ $v == "$controller_own" ]]; then
    ownership=own
    paths+=(
      domain/export/state/controller/export.go
      domain/export/state/controller/export_test.go
      domain/export/service/controller_export.go
      domain/export/service/controller_export_test.go
    )
  fi
  check_frozen controller "$v" "$ownership" "${paths[@]}" || fail=1
done

exit $fail
