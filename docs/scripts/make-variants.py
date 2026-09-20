#!/usr/bin/env python3
"""Generate the synthesized view variants inside juju.ggarch.

The authored views (with positions) and their pure-synthesis variants
(same select and annotations, positions stripped, name suffixed
"(synthesized)") live in ONE file: the model is shared, so the
declared-vs-synthesized comparison is structural (one model, two
views) and cannot drift the way the old file twins did.
Everything below the HAND-MAINTAINED marker is GENERATED: one
"(synthesized)" variant per authored diagram. The refinement demo
("Intro: Juju enters (declared)") is hand-maintained above the marker.

--check: regenerate and compare — CI mode (exit 1 on drift).

Usage: python3 scripts/make-variants.py [--check]
"""

from __future__ import annotations

import sys
from pathlib import Path

DOCS = Path(__file__).resolve().parent.parent
FILE = DOCS / "juju.ggarch"

MARKER = "// ==== HAND-MAINTAINED below — edit freely; `make variants` regenerates everything above ===="

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
    """The synthesized variants: one per authored diagram, positions
    stripped, name suffixed "(synthesized)"."""
    source = FILE.read_text(encoding="utf-8")
    authored = source[:source.index(MARKER)] if MARKER in source else source
    out: list[str] = [MARKER, "",
                      "// ==== GENERATED BELOW — synthesized variants of the",
                      "// ==== authored views above. Regenerate with `make variants`.===",
                      ""]
    # split authored diagrams on their banner comments
    import re
    chunks = re.split(r"(?=^// -{10,})", authored, flags=re.M)
    for chunk in chunks:
        m = re.search(r'diagram "([^"]+)"', chunk)
        if not m or "positions {" not in chunk:
            continue  # model body, banners without diagrams, declared view
        name = m.group(1)
        variant = strip_positions_blocks(chunk)
        variant = variant.replace(f'diagram "{name}"',
                                  f'diagram "{name} (synthesized)"', 1)
        out.append(collapse_blank_runs(variant).rstrip())
        out.append("")
    return "\n".join(out) + "\n"


def hand_maintained(target_text: str) -> str:
    """Everything after the marker (empty when the marker is absent)."""
    if MARKER in target_text:
        return target_text[target_text.index(MARKER):]
    return MARKER + "\n"


def main() -> int:
    generated = generate()
    current = FILE.read_text(encoding="utf-8")
    idx = current.index(MARKER)
    result = current[:idx] + generated

    if "--check" in sys.argv:
        if result != current:
            print("juju3.ggarch synthesized variants are stale — "
                  "regenerate with `make variants`", file=sys.stderr)
            return 1
        return 0
    FILE.write_text(result, encoding="utf-8")
    n = result[idx:].count("(synthesized)")
    print(f"regenerated {n} synthesized variants in {FILE}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
