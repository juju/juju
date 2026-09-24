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

# Authored views that get NO synthesized variant. Each entry is a
# parked synthesis gap with its reason on record — the declared view
# remains the product surface (ADR-007: declared arrangement REQUIRED
# outranks synthesis MEDIUM).
SYNTH_SKIP: set[str] = {
    # "Relation attributes" (session 13): the pure-synthesis twin
    # reverses the FK reading (children left of parents) and routes
    # unit -> unit_boundary THROUGH relation_endpoint — synthesis does
    # not own ER forests yet. Parked with the synthesis-of-ER-forests
    # item; revisit when the solver grows an FK-direction plane.
    "Relation attributes",
    # "Machine attributes" (session 16, machine.md ADR-011 rollout):
    # same failure class. The twin layers the machine hub three ranks
    # from its source leaves (parent pair, status, instance chain) and
    # routes status -> machine through parent, plus a parent -> machine
    # graze on instance — caught by the corpus zero-crossing tests. The
    # declared view — the product surface — audits 0 crossings,
    # max-ratio 1.03. Same parked item as above.
    "Machine attributes",
    # "Application attributes" / "Unit attributes" (session 18,
    # application.md/unit.md ADR-011 rollout): same failure class as
    # Relation/Machine attributes — ER hub slices with satellite fans
    # (charm/status/config/endpoint west-east-south; the unit's
    # principal pair and shared net node). The declared views are the
    # product surface.
    "Application attributes",
    "Unit attributes",
    # "Charm attributes" (session 18, charm.md ADR-011 rollout): same
    # failure class — ER hub slice with satellite fans (metadata +
    # download west, relation + config east, actions south).
    "Charm attributes",
    # "Secret attributes" (session 18, secret.md ADR-011 rollout):
    # same failure class — ER hub slice with satellite fans (revisions
    # + content west, owner + consumer east, permissions south).
    "Secret attributes",
}

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
        # One variant PER DIAGRAM: banner chunking groups every diagram
        # under one banner into a single chunk, and renaming only the
        # first swept the rest through as same-name position-stripped
        # shadows (the shipped Tutorial: auth / Tutorial: provision &
        # deploy duplicates; measured 2026-09-23). Each diagram block is
        # cut out individually; the chunk banner is emitted once.
        banner = re.match(r"\s*// -{10,}[^\n]*(\n//[^\n]*)*", chunk)
        header = banner.group(0) + "\n" if banner else ""
        blocks = list(re.finditer(r'(?=diagram "[^"]+")', chunk))
        if not blocks:
            continue  # model body, banners without diagrams
        pieces: list[str] = [header, ""] if header else []
        for i, bm in enumerate(blocks):
            start = bm.start()
            end = blocks[i + 1].start() if i + 1 < len(blocks) else len(chunk)
            sub = chunk[start:end]
            m = re.match(r'diagram "([^"]+)"', sub)
            if not m or "positions {" not in sub:
                continue  # declared view (no positions to strip)
            name = m.group(1)
            # The ADR-007 refinement demo is hand-maintained; it never
            # gets a synthesized variant (the banner between the
            # authored "Juju enters" and the demo once failed to split,
            # sweeping both into one chunk: the demo shipped as a
            # stripped twin, two views with one name — measured,
            # 2026-09-21).
            if "(declared)" in name or name in SYNTH_SKIP:
                continue
            variant = strip_positions_blocks(sub)
            variant = variant.replace(f'diagram "{name}"',
                                      f'diagram "{name} (synthesized)"', 1)
            pieces.append(collapse_blank_runs(variant).rstrip())
            pieces.append("")
        if len(pieces) > (2 if header else 0):
            out.extend(pieces)
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
            print("juju.ggarch synthesized variants are stale — "
                  "regenerate with `make variants`", file=sys.stderr)
            return 1
        return 0
    FILE.write_text(result, encoding="utf-8")
    n = result[idx:].count("(synthesized)")
    print(f"regenerated {n} synthesized variants in {FILE}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
