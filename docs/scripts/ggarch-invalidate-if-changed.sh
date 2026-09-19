#!/usr/bin/env bash
# Force a full re-parse when ggarch's own source changed.
#
# The SVG cache keys fold in a hash of ggarch's source, so a renderer
# edit invalidates every cached diagram — but incremental Sphinx skips
# unchanged .md sources, meaning the ggarch directives would never
# re-run to notice. Removing the doctrees forces a full re-parse; the
# directives re-execute, miss their (changed) cache keys, and
# re-render. Used as a `make run --pre-build` hook.
set -eu

GGARCH_SRC="${1:-$HOME/git/ggarch/src}"
STAMP=".sphinx/ggarch-src.stamp"

HASH=$(find "$GGARCH_SRC" -name '*.py' -type f -print0 2>/dev/null \
    | sort -z | xargs -0 md5sum | md5sum | cut -d' ' -f1)

if [ -f "$STAMP" ] && [ "$(cat "$STAMP")" = "$HASH" ]; then
    exit 0
fi

echo "ggarch source changed — invalidating doctrees (full re-render)"
rm -rf .sphinx/.doctrees
mkdir -p .sphinx
echo "$HASH" > "$STAMP"
