#!/usr/bin/env python3
"""Generate juju4.ggarch (the auto-layout twin) from juju3.ggarch.

Single source of truth: the model and views live in juju3.ggarch; the
twin is DERIVED — every `positions` block stripped, everything else
(select curation, annotations, instances) kept verbatim. This ends the
twin-drift class: edits to juju3's model/views propagate by
regeneration instead of being hand-copied (three drifts bit in one
day: dropped except clauses, an unlabeled edge, stale fan spacing).

Everything in juju4.ggarch after the HAND-MAINTAINED marker survives
regeneration (the ADR-007 refinement demonstration view, which declares
what the twin's pure-synthesis charter omits).

--check: regenerate to stdout and diff against the file — CI mode
(exit 1 on drift).

Usage: python3 scripts/make-twin.py [--check]
"""

from __future__ import annotations

import sys
from pathlib import Path

DOCS = Path(__file__).resolve().parent.parent
SOURCE = DOCS / "juju3.ggarch"
TARGET = DOCS / "juju4.ggarch"

MARKER = "// ==== HAND-MAINTAINED below — edit freely; `make twin` regenerates everything above ===="

TWIN_HEADER = """// ==========================================================================
// juju4.ggarch -- the auto-layout twin. GENERATED from juju3.ggarch by
// scripts/make-twin.py -- do not edit the generated part by hand; edit
// juju3.ggarch and regenerate (make twin). Everything after the
// HAND-MAINTAINED marker below is hand-maintained and survives
// regeneration.
//
// Identical model and views to juju3.ggarch, with every positions
// block stripped: each diagram without a declared arrangement is
// solved by pure synthesis. Views with declared constraints run in
// ADR-007 refinement mode (synthesis fills the undeclared geometry).
// ==========================================================================

"""


def strip_positions_blocks(text: str) -> str:
    """Remove every `positions { ... }` block (brace-matched, so nested
    braces inside the block are honoured)."""
    out: list[str] = []
    depth = 0
    in_block = False
    for line in text.splitlines(keepends=True):
        stripped = line.strip()
        if not in_block and stripped == "positions {":
            in_block = True
            depth = 1
            continue
        if in_block:
            depth += line.count("{") - line.count("}")
            if depth <= 0:
                in_block = False
            continue
        out.append(line)
    return "".join(out)


def collapse_blank_runs(text: str) -> str:
    """Strip-block removal leaves double blank lines; collapse runs >1."""
    out: list[str] = []
    for line in text.splitlines(keepends=True):
        if line.strip() == "" and out and out[-1].strip() == "":
            continue
        out.append(line)
    return "".join(out)


def generate() -> str:
    source = SOURCE.read_text()
    # Drop juju3's header: generation starts at the model.
    body = source[source.index('model "Juju"'):]
    body = strip_positions_blocks(body)
    body = collapse_blank_runs(body)
    return TWIN_HEADER + body.rstrip() + "\n"


def hand_maintained(target_text: str) -> str:
    """Everything after the marker (empty when the marker is absent —
    first generation seeds it with the declared-view section header)."""
    if MARKER in target_text:
        return target_text[target_text.index(MARKER):]
    return (MARKER + "\n"
            + "// The ADR-007 refinement demonstration view lives here.\n"
            + "// (First generation seeded this marker; the declared view\n"
            + "// below was moved under it by that generation.)\n")


def main() -> int:
    generated = generate()
    current = TARGET.read_text() if TARGET.exists() else ""
    keep = hand_maintained(current)
    result = generated.rstrip() + "\n\n\n" + keep if keep.strip() != MARKER \
        else generated
    # Always carry the marker (a first-generation file gains it).
    if MARKER not in result:
        result = result.rstrip() + "\n\n\n" + MARKER + "\n"

    if "--check" in sys.argv:
        if result != current:
            print("juju4.ggarch is stale — regenerate with `make twin`",
                  file=sys.stderr)
            return 1
        return 0
    TARGET.write_text(result)
    print(f"regenerated {TARGET} from {SOURCE} "
          f"({len(keep.splitlines())} hand-maintained lines preserved)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
