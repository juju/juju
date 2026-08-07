---
myst:
  html_meta:
    description: "Juju documentation diagram tooling: how to choose and author text-based, version-controlled diagrams (Mermaid) for explanation and reference docs."
---

(diagram-tooling)=
# Diagram tooling

Juju documentation uses **text-based, version-controlled diagrams** so they can be reviewed and diffed in pull requests, regenerated reliably, and edited by both people and LLM tooling.

## Tool choice

Two tools are used, depending on the diagram:

| Tool | Format | Used for | Rendering |
|---|---|---|---|
| [Mermaid](https://mermaid.js.org/) | Inline fenced block (`{mermaid}`) in Markdown | Component, topology, and sequence diagrams in narrative docs | `sphinxcontrib-mermaid` at build time |
| [Excalidraw](https://excalidraw.com/) | `.excalidraw` JSON source file | Densely detailed reference diagrams where fine layout control matters | `scripts/convert-excalidraw-to-svg-recursively.sh` |

**Mermaid is the default.** Inline Mermaid blocks live next to the prose that describes them, are fully diffable, and need no separate build step beyond the Sphinx extension. It is the right choice for the question-answer style architecture and explanation diagrams.

Prefer **Excalidraw** only when a diagram needs precise, pixel-level layout that flowchart and sequence diagram notation cannot express cleanly. Store the `.excalidraw` JSON source and commit the generated `.svg` alongside it, mirroring the existing `relation-databags.*` and `relation-taxonomy.*` examples in `reference/`.

## Rendering integration

For Mermaid:

- Add `sphinxcontrib-mermaid` to `docs/requirements.txt`.
- Register the extension in `docs/conf.py` (`extensions`). The extension emits the diagram client-side using the Mermaid JavaScript runtime, so no local diagram renderer is required and charts are searchable text.

For Excalidraw:

- Commit both the `.excalidraw` source and the generated `.svg` (and `.dark.svg`) output.
- Regenerate with `make convert-excalidraw-to-svg` inside `docs/`.
- Reference the `.svg` in the page with a `{figure}` directive so readers can download a copy; the dark variant is used automatically in dark mode.

## Style guidelines

Apply these rules to every diagram:

- **One idea per diagram.** If a diagram becomes crowded, split it and use progressive disclosure: open a section with a simple overview diagram, then expand into focused sub-diagrams.
- **Keep labels short and concrete.** Use real names where possible (`unit agent`, `controller`, `containeragent`) rather than generic boxes. Avoid jargon that is not defined in the surrounding text.
- **Clarify direction.** Show the direction of data or control flow with labeled edges (`Juju API`, `hook commands`, `Pebble API`). Use solid lines for the primary path and dashed lines for the control/API path where helpful.
- **Match the surrounding terminology.** Use the same names for components as the reference pages they point to (e.g. `unit agent`, `containeragent`, {ref}`Pebble <pebble>`).
- **Prefer flowchart `LR` (left-to-right) for topology** and `sequenceDiagram` for interactions. Keep nested `subgraph`s shallow -- at most two levels -- so the diagram renders legibly.
- **Provide an alt-text caption.** Add an italic caption under each diagram describing what it shows; this aids accessibility and search.
- **Reuse across docs, don't duplicate.** If the same diagram would serve two pages, put it in one place and cross-reference it from the others.

## Acceptable and unacceptable patterns

- **Acceptable:** inline `{mermaid}` blocks; `.excalidraw` + committed `.svg`/`.dark.svg` pairs.
- **Unacceptable:** committing binary diagram images (`.png`, `.jpeg`) generated from a tool whose editable source is not in the repository -- the image cannot be reviewed or regenerated.
