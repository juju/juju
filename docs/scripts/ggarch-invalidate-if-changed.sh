#!/usr/bin/env bash
# Force a full re-parse when ggarch's own source OR the .ggarch models
# changed.
#
# The SVG cache keys fold in a hash of ggarch's source, so a renderer
# edit invalidates every cached diagram — but incremental Sphinx skips
# unchanged .md sources, meaning the ggarch directives would never
# re-run to notice. Removing the doctrees forces a full re-parse; the
# directives re-execute, miss their (changed) cache keys, and
# re-render. Used as a `make run --pre-build` hook.
#
# 2026-09-20: the same trap applies to the MODELS — a .ggarch edit
# (new view, changed constraint, added label) leaves diagrams*.md
# untouched, so the cached doctree kept serving the pre-edit page with
# pre-edit image hashes (measured: the refinement demo view was absent
# from the served diagrams4 after the file changed). The models are
# now hashed alongside the renderer source.
set -eu

GGARCH_SRC="${1:-$HOME/git/ggarch/src}"
STAMP=".sphinx/ggarch-src.stamp"

SRC_HASH=$(find "$GGARCH_SRC" -name '*.py' -type f -print0 2>/dev/null \
    | sort -z | xargs -0 md5sum | md5sum | cut -d' ' -f1)
MODEL_HASH=$(find . -maxdepth 1 -name '*.ggarch' -type f -print0 2>/dev/null \
    | sort -z | xargs -0 md5sum 2>/dev/null | md5sum | cut -d' ' -f1)
HASH="$SRC_HASH $MODEL_HASH"

if [ -f "$STAMP" ] && [ "$(cat "$STAMP")" = "$HASH" ]; then
    exit 0
fi

echo "ggarch source or models changed — invalidating doctrees (full re-render)"
rm -rf .sphinx/.doctrees
mkdir -p .sphinx
echo "$HASH" > "$STAMP"
